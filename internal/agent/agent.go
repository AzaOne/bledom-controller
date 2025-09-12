package agent

import (
	"context"
	"log"
	"sync"

	"bledom-controller/internal/ble"
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

	// wg (WaitGroup) for waiting on background goroutines to finish.
	wg sync.WaitGroup

	// State for safely sharing BLE status
	lastBleStatus bool
	lastRssi      int16
	statusMutex   sync.RWMutex
	runningPattern string
	agentMutex     sync.Mutex
}

// NewAgent creates and initializes a new agent.
func NewAgent() (*Agent, error) {
	ctx, cancel := context.WithCancel(context.Background())

	a := &Agent{
		ctx:           ctx,
		cancel:        cancel,
		lastBleStatus: false, // Assume disconnected at start
		lastRssi:      0,
	}

	a.bleController = ble.NewController()
	a.luaEngine = lua.NewEngine(a.bleController)

	patternStatusCallback := func(runningPattern string) {
		a.agentMutex.Lock()
		a.runningPattern = runningPattern
		a.agentMutex.Unlock()

		payload := map[string]string{"running": runningPattern}
		a.server.Hub.Broadcast(server.NewMessage("pattern_status", payload))
	}

	a.scheduler = scheduler.NewScheduler(a.luaEngine, a.bleController, patternStatusCallback)

	// The command handler will now also use this generic callback.
	commandHandler := NewCommandHandler(a.bleController, a.luaEngine, a.scheduler)
	// Pass the getLastRssi function to the server.
	a.server = server.NewServer(commandHandler, a.getLastBleStatus, a.getLastRssi, a.getSchedules)

	return a, nil
}

// getLastBleStatus is a thread-safe way to get the current connection status.
func (a *Agent) getLastBleStatus() bool {
	a.statusMutex.RLock()
	defer a.statusMutex.RUnlock()
	return a.lastBleStatus
}

// getLastRssi is a thread-safe way to get the last known RSSI.
func (a *Agent) getLastRssi() int16 {
	a.statusMutex.RLock()
	defer a.statusMutex.RUnlock()
	return a.lastRssi
}

// getSchedules is a thread-safe way to get the current schedules.
func (a *Agent) getSchedules() map[cron.EntryID]scheduler.ScheduleEntry {
	return a.scheduler.GetAll()
}

// Run starts all the agent's services.
func (a *Agent) Run() {
	// Define the callback for BLE status changes.
	bleStatusCallback := func(connected bool, rssi int16) {
		wasConnected := a.getLastBleStatus() // Get status *before* updating

		a.statusMutex.Lock()
		a.lastBleStatus = connected
		a.lastRssi = rssi
		a.statusMutex.Unlock()

		msg := server.NewMessage("ble_status", map[string]interface{}{
			"connected": connected,
			"rssi":      rssi,
		})
		a.server.Hub.Broadcast(msg)

		// Check if the status changed from disconnected to connected.
		if !wasConnected && connected {
			log.Println("Device connected, checking for a pattern to resume.")

			a.agentMutex.Lock()
			patternToResume := a.runningPattern
			a.agentMutex.Unlock()

			if patternToResume != "" {
				log.Printf("Resuming pattern: %s", patternToResume)

				// Create the callback that the Lua engine will use to report its status.
				resumeCallback := func(runningPattern string) {
					a.agentMutex.Lock()
					a.runningPattern = runningPattern
					a.agentMutex.Unlock()
					payload := map[string]string{"running": runningPattern}
					a.server.Hub.Broadcast(server.NewMessage("pattern_status", payload))
				}
				// Run the pattern in a new goroutine.
				go a.luaEngine.RunPattern(patternToResume, resumeCallback)
			}
		}
	}

	// Run the BLE controller in a goroutine managed by the WaitGroup
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.bleController.Run(a.ctx, bleStatusCallback)
	}()

	a.scheduler.Start()

	log.Println("Agent is running. Starting web server on http://localhost:8080")
	if err := a.server.ListenAndServe(); err != nil {
		log.Printf("Server error: %v", err)
	}
}

// Shutdown gracefully stops the agent's services.
func (a *Agent) Shutdown() {
	log.Println("Stopping scheduler and server...")
	a.scheduler.Stop()
	// Use a new background context for server shutdown to ensure it completes.
	if err := a.server.Shutdown(context.Background()); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Signaling background services to stop...")
	a.cancel() // Signal goroutines using a.ctx to stop

	log.Println("Waiting for background services to finish...")
	a.wg.Wait() // Wait for the BLE controller goroutine to finish completely
	log.Println("All background services finished.")
}
