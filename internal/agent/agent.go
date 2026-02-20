package agent

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/config"
	"bledom-controller/internal/core"
	"bledom-controller/internal/lua"
	"bledom-controller/internal/mqtt"
	"bledom-controller/internal/scheduler"
	"bledom-controller/internal/server"
)

type Agent struct {
	ctx    context.Context
	cancel context.CancelFunc
	config *config.Config
	wg     sync.WaitGroup

	state          *core.State
	eventBus       *core.EventBus
	commandChannel core.CommandChannel

	bleController *ble.Controller
	luaEngine     *lua.Engine
	scheduler     *scheduler.Scheduler
	server        *server.Server
	mqttClient    *mqtt.Client
}

func NewAgent(cfg *config.Config) (*Agent, error) {
	ctx, cancel := context.WithCancel(context.Background())

	a := &Agent{
		ctx:            ctx,
		cancel:         cancel,
		config:         cfg,
		state:          core.NewState(),
		eventBus:       core.NewEventBus(),
		commandChannel: make(core.CommandChannel, 20),
	}

	// Налаштування Bluetooth
	bleScanTimeout, _ := time.ParseDuration(cfg.BLE.ScanTimeout)
	bleConnectTimeout, _ := time.ParseDuration(cfg.BLE.ConnectTimeout)
	bleHeartbeatInterval, _ := time.ParseDuration(cfg.BLE.HeartbeatInterval)
	bleRetryDelay, _ := time.ParseDuration(cfg.BLE.RetryDelay)

	a.bleController = ble.NewController(
		ctx,
		a.eventBus,
		cfg.BLE.DeviceNames,
		bleScanTimeout,
		bleConnectTimeout,
		bleHeartbeatInterval,
		bleRetryDelay,
		cfg.BLE.RateLimit,
		cfg.BLE.RateBurst,
	)

	a.luaEngine = lua.NewEngine(a.bleController, cfg.PatternsDir, a.eventBus)

	// Create Scheduler (before server so we can pass it in)
	a.scheduler = scheduler.NewScheduler(a.commandChannel, cfg.SchedulesFile)

	// Create Server
	a.server = server.NewServer(
		a.luaEngine,
		a.eventBus,
		a.state,
		a.scheduler,
		a.commandChannel,
		cfg.Server.Port,
		cfg.Server.WebFilesDir,
		cfg.Server.AllowedOrigins,
	)

	// Create MQTT Client (optional)
	a.mqttClient = mqtt.NewClient(cfg, a.eventBus, a.state, a.commandChannel, a.luaEngine.GetPatternList)

	return a, nil
}

// Run starts the agent orchestration loop.
func (a *Agent) Run() {
	// Hook up event subscriptions to maintain the central state and handle resync logic
	go a.listenEvents()

	if a.mqttClient != nil {
		go func() {
			if err := a.mqttClient.Connect(); err != nil {
				log.Printf("[Agent] MQTT Setup Error: %v", err)
			}
		}()
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.bleController.Run(a.ctx)
	}()

	a.scheduler.Start()

	log.Printf("Agent running on http://localhost:%s", a.config.Server.Port)
	go func() {
		if err := a.server.ListenAndServe(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Orchestrator Central Command Loop
	log.Println("Agent orchestrator ready.")
	for {
		select {
		case <-a.ctx.Done():
			log.Println("Agent orchestrator shutting down...")
			return
		case cmd := <-a.commandChannel:
			a.handleCommand(cmd)
		}
	}
}

func (a *Agent) listenEvents() {
	sub := a.eventBus.Subscribe(core.DeviceConnectedEvent, core.PatternChangedEvent)

	for {
		select {
		case <-a.ctx.Done():
			return
		case event := <-sub:
			switch event.Type {
			case core.DeviceConnectedEvent:
				// Update State and Resume Pattern if needed
				if payload, ok := event.Payload.(map[string]interface{}); ok {
					if connected, ok := payload["connected"].(bool); ok {
						if rssi, ok := payload["rssi"].(int16); ok {
							wasConnected := a.state.Clone().IsConnected
							a.state.SetConnection(connected, rssi)

							if !wasConnected && connected {
								log.Println("Device connected, checking for a pattern to resume.")
								patternToResume := a.state.Clone().RunningPattern

								if patternToResume != "" {
									log.Printf("Resuming pattern: %s", patternToResume)
									a.luaEngine.RunPattern(patternToResume)
								}
							}
						}
					}
				}
			case core.PatternChangedEvent:
				if payload, ok := event.Payload.(map[string]interface{}); ok {
					if pattern, ok := payload["running"].(string); ok {
						a.state.SetRunningPattern(pattern)

						if pattern == "" {
							log.Println("Pattern finished. Syncing final state.")
							a.syncState()
						}
					}
				}
			}
		}
	}
}

func (a *Agent) handleCommand(cmd core.Command) {
	log.Printf("[Agent] Handling command: %s with payload: %v", cmd.Type, cmd.Payload)

	currentState := a.state.Clone()

	switch cmd.Type {
	case core.CmdSetPower:
		isOn := false
		if b, ok := cmd.Payload["isOn"].(bool); ok {
			isOn = b
		}

		if currentState.Power == isOn {
			log.Printf("[Agent] Power already %v, skipping pattern stop.", isOn)
		} else {
			log.Printf("[Agent] Power changing to %v, stopping pattern.", isOn)
			a.luaEngine.StopCurrentPattern()
		}

		a.state.SetPower(isOn)
		a.bleController.SetPower(isOn)
		a.eventBus.Publish(core.Event{Type: core.PowerChangedEvent, Payload: map[string]interface{}{"isOn": isOn}})

	case core.CmdSetColor:
		r := 0
		g := 0
		b := 0
		if vr, ok := cmd.Payload["r"].(float64); ok {
			r = int(vr)
		}
		if vg, ok := cmd.Payload["g"].(float64); ok {
			g = int(vg)
		}
		if vb, ok := cmd.Payload["b"].(float64); ok {
			b = int(vb)
		}

		if currentState.ColorR == r && currentState.ColorG == g && currentState.ColorB == b {
			log.Printf("[Agent] Color already #%02X%02X%02X, skipping pattern stop.", r, g, b)
		} else {
			log.Printf("[Agent] Color changing to #%02X%02X%02X, stopping pattern.", r, g, b)
			a.luaEngine.StopCurrentPattern()
		}

		a.state.SetColor(r, g, b)
		a.bleController.SetColor(r, g, b)

		hex := fmt.Sprintf("#%02X%02X%02X", r, g, b)
		a.eventBus.Publish(core.Event{
			Type: core.ColorChangedEvent,
			Payload: map[string]interface{}{
				"r": r, "g": g, "b": b, "hex": hex,
			},
		})

	case core.CmdSetBrightness:
		val := 100
		if v, ok := cmd.Payload["value"].(float64); ok {
			val = int(v)
		}
		a.state.SetBrightness(val)
		a.bleController.SetBrightness(val)
		a.eventBus.Publish(core.Event{Type: core.StateChangedEvent, Payload: map[string]interface{}{"brightness": val}}) // Partial state change can be published this way too or just resync all

	case core.CmdSetSpeed:
		val := 50
		if v, ok := cmd.Payload["value"].(float64); ok {
			val = int(v)
		}
		a.state.SetSpeed(val)
		a.bleController.SetSpeed(val)
		a.eventBus.Publish(core.Event{Type: core.StateChangedEvent, Payload: map[string]interface{}{"speed": val}})

	case core.CmdSetHardwarePattern:
		id := 0
		if v, ok := cmd.Payload["id"].(float64); ok {
			id = int(v)
		}
		if currentState.RunningPattern != "" {
			log.Printf("[Agent] Hardware pattern requested while Lua pattern '%s' is running. Stopping Lua pattern.", currentState.RunningPattern)
		}
		a.luaEngine.StopCurrentPattern()
		a.bleController.SetHardwarePattern(id)

	case core.CmdSyncTime:
		a.bleController.SyncTime()

	case core.CmdSetRgbOrder:
		v1, v2, v3 := 0, 0, 0
		if v, ok := cmd.Payload["v1"].(float64); ok {
			v1 = int(v)
		}
		if v, ok := cmd.Payload["v2"].(float64); ok {
			v2 = int(v)
		}
		if v, ok := cmd.Payload["v3"].(float64); ok {
			v3 = int(v)
		}
		a.bleController.SetRgbOrder(v1, v2, v3)

	case core.CmdSetSchedule:
		hour, minute, second := 0, 0, 0
		var weekdays byte = 0
		isOn, isSet := false, false
		if v, ok := cmd.Payload["hour"].(float64); ok {
			hour = int(v)
		}
		if v, ok := cmd.Payload["minute"].(float64); ok {
			minute = int(v)
		}
		if v, ok := cmd.Payload["second"].(float64); ok {
			second = int(v)
		}
		if v, ok := cmd.Payload["weekdays"].(float64); ok {
			weekdays = byte(v)
		}
		if v, ok := cmd.Payload["isOn"].(bool); ok {
			isOn = v
		}
		if v, ok := cmd.Payload["isSet"].(bool); ok {
			isSet = v
		}
		a.bleController.SetSchedule(hour, minute, second, weekdays, isOn, isSet)

	case core.CmdRunPattern:
		name := ""
		if v, ok := cmd.Payload["name"].(string); ok {
			name = v
		}
		a.luaEngine.RunPattern(name)

	case core.CmdStopPattern:
		a.luaEngine.StopCurrentPattern()

	case core.CmdAddSchedule:
		spec, command := "", ""
		if v, ok := cmd.Payload["spec"].(string); ok {
			spec = v
		}
		if v, ok := cmd.Payload["command"].(string); ok {
			command = v
		}
		a.scheduler.Add(spec, command)

		// Send an update event
		if a.server != nil && a.server.Hub != nil {
			a.server.Hub.Broadcast(server.NewMessage("schedule_list", a.scheduler.GetAll()))
		}

	case core.CmdRemoveSchedule:
		if idStr, ok := cmd.Payload["id"].(string); ok {
			if id, err := strconv.Atoi(idStr); err == nil {
				a.scheduler.Remove(id)
				if a.server != nil && a.server.Hub != nil {
					a.server.Hub.Broadcast(server.NewMessage("schedule_list", a.scheduler.GetAll()))
				}
			}
		}

	case core.CmdGetPatternCode:
		if name, ok := cmd.Payload["name"].(string); ok {
			if content, err := a.luaEngine.GetPatternCode(name); err == nil {
				if a.server != nil && a.server.Hub != nil {
					a.server.Hub.Broadcast(server.NewMessage("pattern_code", map[string]string{"name": name, "code": content}))
				}
			} else {
				log.Printf("Error getting pattern code: %v", err)
			}
		}

	case core.CmdSavePatternCode:
		name, nameOk := cmd.Payload["name"].(string)
		code, codeOk := cmd.Payload["code"].(string)
		if nameOk && codeOk {
			if err := a.luaEngine.SavePatternCode(name, code); err != nil {
				log.Printf("Error saving pattern: %v", err)
			} else {
				patterns, _ := a.luaEngine.GetPatternList()
				if a.server != nil && a.server.Hub != nil {
					a.server.Hub.Broadcast(server.NewMessage("pattern_list", patterns))
				}
			}
		}

	case core.CmdDeletePattern:
		if name, ok := cmd.Payload["name"].(string); ok {
			if err := a.luaEngine.DeletePattern(name); err != nil {
				log.Printf("Error deleting pattern '%s': %v", name, err)
			} else {
				patterns, _ := a.luaEngine.GetPatternList()
				if a.server != nil && a.server.Hub != nil {
					a.server.Hub.Broadcast(server.NewMessage("pattern_list", patterns))
				}
			}
		}

	default:
		log.Printf("Unknown command type: %s", cmd.Type)
	}
}

// syncState зчитує актуальний стан з BLE контролера і розсилає його
func (a *Agent) syncState() {
	bs := a.bleController.GetState()

	// Sync internal State
	a.state.SetPower(bs.IsOn)
	a.state.SetColor(bs.R, bs.G, bs.B)
	a.state.SetBrightness(bs.Brightness)
	a.state.SetSpeed(bs.Speed)

	// Publish global sync event
	hex := fmt.Sprintf("#%02X%02X%02X", bs.R, bs.G, bs.B)
	a.eventBus.Publish(core.Event{
		Type: core.StateChangedEvent,
		Payload: map[string]interface{}{
			"isOn":       bs.IsOn,
			"r":          bs.R,
			"g":          bs.G,
			"b":          bs.B,
			"hex":        hex,
			"brightness": bs.Brightness,
			"speed":      bs.Speed,
		},
	})
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
