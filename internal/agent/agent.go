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
	"bledom-controller/internal/mqtt"
	"bledom-controller/internal/scheduler"
	"bledom-controller/internal/server"
	"github.com/robfig/cron/v3"
)

type Agent struct {
	ctx           context.Context
	cancel        context.CancelFunc
	bleController *ble.Controller
	luaEngine     *lua.Engine
	scheduler     *scheduler.Scheduler
	server        *server.Server
	mqttClient    *mqtt.Client
	config        *config.Config
	wg            sync.WaitGroup

	lastBleStatus  bool
	lastRsssi      int16
	statusMutex    sync.RWMutex
	runningPattern string
	agentMutex     sync.Mutex
}

func NewAgent(cfg *config.Config) (*Agent, error) {
	ctx, cancel := context.WithCancel(context.Background())

	a := &Agent{
		ctx:           ctx,
		cancel:        cancel,
		lastBleStatus: false,
		lastRsssi:     0,
		config:        cfg,
	}

	// Налаштування Bluetooth
	bleScanTimeout, _ := time.ParseDuration(cfg.BLE.ScanTimeout)
	bleConnectTimeout, _ := time.ParseDuration(cfg.BLE.ConnectTimeout)
	bleHeartbeatInterval, _ := time.ParseDuration(cfg.BLE.HeartbeatInterval)
	bleRetryDelay, _ := time.ParseDuration(cfg.BLE.RetryDelay)

	a.bleController = ble.NewController(
		ctx,
		cfg.BLE.DeviceNames,
		bleScanTimeout,
		bleConnectTimeout,
		bleHeartbeatInterval,
		bleRetryDelay,
		cfg.BLE.RateLimit,
		cfg.BLE.RateBurst,
	)

	a.luaEngine = lua.NewEngine(a.bleController, cfg.PatternsDir)

	// Create Server
	a.server = server.NewServer(
		a.luaEngine,
		a.getLastBleStatus,
		a.getLastRsssi,
		a.getSchedules,
		a.getRunningPattern,
		a.bleController.GetState,
		cfg.Server.Port,
		cfg.Server.StaticFilesDir,
		cfg.Server.AllowedOrigins,
	)

	// Create MQTT Client
	a.mqttClient = mqtt.NewClient(cfg, a.bleController, a.luaEngine, a.server.Hub)

	// Create Scheduler
	a.scheduler = scheduler.NewScheduler(a.luaEngine, a.bleController, a.handlePatternStatusChange, cfg.SchedulesFile)

	// Create CommandHandler
	commandHandler := NewCommandHandler(a.bleController, a.luaEngine, a.scheduler, a.mqttClient, a.handlePatternStatusChange)

	// Connect Handler to Server
	a.server.SetHandler(commandHandler)

	return a, nil
}

func (a *Agent) getRunningPattern() string {
	a.agentMutex.Lock()
	defer a.agentMutex.Unlock()
	return a.runningPattern
}

// handlePatternStatusChange викликається при зміні статусу скрипта.
func (a *Agent) handlePatternStatusChange(runningPattern string) {
	a.agentMutex.Lock()
	a.runningPattern = runningPattern
	a.agentMutex.Unlock()

	// 1. Broadcast Pattern Name
	payload := map[string]string{"running": runningPattern}
	if a.server != nil && a.server.Hub != nil {
		a.server.Hub.Broadcast(server.NewMessage("pattern_status", payload))
	}

	// 2. Broadcast MQTT Pattern Name
	if a.mqttClient != nil {
		state := runningPattern
		if state == "" {
			state = "IDLE"
		}
		a.mqttClient.Publish("pattern/state", state, true)
	}

	// 3. Sync Final State (Якщо патерн завершився)
	if runningPattern == "" {
		log.Println("Pattern finished. Syncing final state.")
		a.syncState()
	}
}

// syncState зчитує актуальний стан з BLE контролера і розсилає його в WS і MQTT
func (a *Agent) syncState() {
	state := a.bleController.GetState()

	// 1. Sync WebSocket (UI)
	if a.server != nil && a.server.Hub != nil {
		hex := fmt.Sprintf("#%02X%02X%02X", state.R, state.G, state.B)
		a.server.Hub.Broadcast(server.NewMessage("device_state", map[string]interface{}{
			"isOn":       state.IsOn,
			"r":          state.R,
			"g":          state.G,
			"b":          state.B,
			"hex":        hex,
			"brightness": state.Brightness,
			"speed":      state.Speed,
		}))
	}

	// 2. Sync MQTT (Home Assistant)
	if a.mqttClient != nil {
		powerPayload := "OFF"
		if state.IsOn {
			powerPayload = "ON"
		}
		a.mqttClient.Publish("power/state", powerPayload, true)

		a.mqttClient.Publish("brightness/state", state.Brightness, true)

		rgbPayload := fmt.Sprintf("%d,%d,%d", state.R, state.G, state.B)
		a.mqttClient.Publish("color/state", rgbPayload, true)
	}
}

func (a *Agent) getLastBleStatus() bool {
	a.statusMutex.RLock()
	defer a.statusMutex.RUnlock()
	return a.lastBleStatus
}

func (a *Agent) getLastRsssi() int16 {
	a.statusMutex.RLock()
	defer a.statusMutex.RUnlock()
	return a.lastRsssi
}

func (a *Agent) getSchedules() map[cron.EntryID]scheduler.ScheduleEntry {
	return a.scheduler.GetAll()
}

func (a *Agent) Run() {
	bleStatusCallback := func(connected bool, rssi int16) {
		wasConnected := a.getLastBleStatus()

		a.statusMutex.Lock()
		a.lastBleStatus = connected
		a.lastRsssi = rssi
		a.statusMutex.Unlock()

		msg := server.NewMessage("ble_status", map[string]interface{}{
			"connected": connected,
			"rssi":      rssi,
		})
		a.server.Hub.Broadcast(msg)

		if a.mqttClient != nil {
			statusStr := "disconnected"
			if connected {
				statusStr = "connected"
			}
			a.mqttClient.Publish("connection", statusStr, true)
			if connected {
				a.mqttClient.Publish("rssi", rssi, false)
			}
		}

		if !wasConnected && connected {
			log.Println("Device connected, checking for a pattern to resume.")
			a.agentMutex.Lock()
			patternToResume := a.runningPattern
			a.agentMutex.Unlock()

			if patternToResume != "" {
				log.Printf("Resuming pattern: %s", patternToResume)
				go a.luaEngine.RunPattern(patternToResume, a.handlePatternStatusChange)
			}
		}
	}

	if a.mqttClient != nil {
		go func() {
			// Connect() тепер використовує SetConnectRetry=true.
			// Якщо брокер офлайн, Connect() не поверне помилку відразу, а буде намагатися
			// підключитися у фоні. Помилка повернеться лише при некоректній конфігурації.
			if err := a.mqttClient.Connect(); err != nil {
				log.Printf("[Agent] MQTT Setup Error: %v", err)
			}
		}()
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.bleController.Run(a.ctx, bleStatusCallback)
	}()

	a.scheduler.Start()

	log.Printf("Agent running on http://localhost:%s", a.config.Server.Port)
	if err := a.server.ListenAndServe(); err != nil {
		log.Printf("Server error: %v", err)
	}
}

func (a *Agent) Shutdown() {
	a.scheduler.Stop()
	_ = a.server.Shutdown(context.Background())
	if a.mqttClient != nil {
		a.mqttClient.Disconnect()
	}
	a.cancel()
	a.wg.Wait()
}
