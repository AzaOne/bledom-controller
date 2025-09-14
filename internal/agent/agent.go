package agent

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"

    "bledom-controller/internal/ble"
    "bledom-controller/internal/config"
    "bledom-controller/internal/lua"
    "bledom-controller/internal/scheduler"
    "bledom-controller/internal/server"
    "github.com/robfig/cron/v3"
)

// Agent is the core application struct holding all services.
type Agent struct {
    ctx           context.Context
    cancel        context.CancelFunc
    bleController *ble.Controller
    luaEngine     *lua.Engine
    scheduler     *scheduler.Scheduler
    server        *server.Server
    config        *config.Config
    wg            sync.WaitGroup // for waiting on background goroutines to finish.
    // State for safely sharing BLE status
    lastBleStatus bool
    lastRsssi      int16
    statusMutex    sync.RWMutex
    runningPattern string
    agentMutex     sync.Mutex
}

// NewAgent creates and initializes a new agent.
func NewAgent(cfg *config.Config) (*Agent, error) {
    ctx, cancel := context.WithCancel(context.Background())

    a := &Agent{
        ctx:            ctx,
        cancel:         cancel,
        lastBleStatus: false, // Assume disconnected at start
        lastRsssi:      0,
        config:         cfg,
    }

    // Parse BLE durations
    bleScanTimeout, err := time.ParseDuration(cfg.BLEScanTimeout)
    if err != nil {
        return nil, fmt.Errorf("invalid BLEScanTimeout: %w", err)
    }
    bleConnectTimeout, err := time.ParseDuration(cfg.BLEConnectTimeout)
    if err != nil {
        return nil, fmt.Errorf("invalid BLEConnectTimeout: %w", err)
    }
    bleHeartbeatInterval, err := time.ParseDuration(cfg.BLEHeartbeatInterval)
    if err != nil {
        return nil, fmt.Errorf("invalid BLEHeartbeatInterval: %w", err)
    }
    bleRetryDelay, err := time.ParseDuration(cfg.BLERetryDelay)
    if err != nil {
        return nil, fmt.Errorf("invalid BLERetryDelay: %w", err)
    }

    // Pass relevant config fields to BLE controller, including the new rate limit parameters
    a.bleController = ble.NewController(
        ctx, // Pass context here for the command writer goroutine
        cfg.DeviceNames,
        bleScanTimeout,
        bleConnectTimeout,
        bleHeartbeatInterval,
        bleRetryDelay,
        cfg.BLECommandRateLimitRate,  // New parameter
        cfg.BLECommandRateLimitBurst, // New parameter
    )

    a.luaEngine = lua.NewEngine(a.bleController, cfg.PatternsDir)

    patternStatusCallback := func(runningPattern string) {
        a.agentMutex.Lock()
        a.runningPattern = runningPattern
        a.agentMutex.Unlock()

        payload := map[string]string{"running": runningPattern}
        // Ensure a.server is initialized before broadcasting
        if a.server != nil && a.server.Hub != nil {
            a.server.Hub.Broadcast(server.NewMessage("pattern_status", payload))
        } else {
            log.Printf("Warning: Cannot broadcast pattern_status, server not ready. Pattern: %s", runningPattern)
        }
    }

    a.scheduler = scheduler.NewScheduler(a.luaEngine, a.bleController, patternStatusCallback, cfg.SchedulesFile)

    commandHandler := NewCommandHandler(a.bleController, a.luaEngine, a.scheduler)

    a.server = server.NewServer(
        commandHandler,
        a.luaEngine,
        a.getLastBleStatus,
        a.getLastRsssi,
        a.getSchedules,
        cfg.ServerPort,
        cfg.StaticFilesDir,
        cfg.AllowedOrigins,
    )

    return a, nil
}

// getLastBleStatus is a thread-safe way to get the current connection status.
func (a *Agent) getLastBleStatus() bool {
    a.statusMutex.RLock()
    defer a.statusMutex.RUnlock()
    return a.lastBleStatus
}

// getLastRssi is a thread-safe way to get the last known RSSI.
func (a *Agent) getLastRsssi() int16 {
    a.statusMutex.RLock()
    defer a.statusMutex.RUnlock()
    return a.lastRsssi
}

// getSchedules is a thread-safe way to get the current schedules.
func (a *Agent) getSchedules() map[cron.EntryID]scheduler.ScheduleEntry {
    return a.scheduler.GetAll()
}

// Run starts all the agent's services.
func (a *Agent) Run() {
    bleStatusCallback := func(connected bool, rssi int16) {
        wasConnected := a.getLastBleStatus() // Get status *before* updating

        a.statusMutex.Lock()
        a.lastBleStatus = connected
        a.lastRsssi = rssi
        a.statusMutex.Unlock()

        msg := server.NewMessage("ble_status", map[string]interface{}{
            "connected": connected,
            "rssi":      rssi,
        })
        a.server.Hub.Broadcast(msg)

        if !wasConnected && connected {
            log.Println("Device connected, checking for a pattern to resume.")

            a.agentMutex.Lock()
            patternToResume := a.runningPattern
            a.agentMutex.Unlock()

            if patternToResume != "" {
                log.Printf("Resuming pattern: %s", patternToResume)

                resumeCallback := func(runningPattern string) {
                    a.agentMutex.Lock()
                    a.runningPattern = runningPattern
                    a.agentMutex.Unlock()
                    payload := map[string]string{"running": runningPattern}
                    a.server.Hub.Broadcast(server.NewMessage("pattern_status", payload))
                }
                go a.luaEngine.RunPattern(patternToResume, resumeCallback)
            }
        }
    }

    a.wg.Add(1)
    go func() {
        defer a.wg.Done()
        a.bleController.Run(a.ctx, bleStatusCallback)
    }()

    a.scheduler.Start()

    log.Printf("Agent is running. Starting web server on http://localhost:%s", a.config.ServerPort)
    if err := a.server.ListenAndServe(); err != nil {
        log.Printf("Server error: %v", err)
    }
}

// Shutdown gracefully stops the agent's services.
func (a *Agent) Shutdown() {
    log.Println("Stopping scheduler and server...")
    a.scheduler.Stop()
    if err := a.server.Shutdown(context.Background()); err != nil {
        log.Printf("Server shutdown error: %v", err)
    }

    log.Println("Signaling background services to stop...")
    a.cancel() // This will signal all goroutines created with a.ctx

    log.Println("Waiting for background services to finish...")
    a.wg.Wait()
    log.Println("All background services finished.")
}
