package agent

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strconv"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/lua"
	"bledom-controller/internal/scheduler"
	"bledom-controller/internal/server"
)

// CommandHandler handles incoming WebSocket commands.
type CommandHandler struct {
	bleController *ble.Controller
	luaEngine     *lua.Engine
	scheduler     *scheduler.Scheduler
}

// NewCommandHandler creates a new command handler.
func NewCommandHandler(bc *ble.Controller, le *lua.Engine, s *scheduler.Scheduler) *CommandHandler {
	return &CommandHandler{
		bleController: bc,
		luaEngine:     le,
		scheduler:     s,
	}
}

// Handle routes the command to the appropriate service.
func (h *CommandHandler) Handle(msg server.Message, hub *server.Hub) {
	var cmd server.Command
	if err := json.Unmarshal(msg.Raw, &cmd); err != nil {
		log.Printf("Error unmarshalling command: %v", err)
		return
	}

	log.Printf("Executing command: %s", cmd.Type)

	switch cmd.Type {

	case "setPower":
		h.luaEngine.StopCurrentPattern() // Stop any running script
		h.bleController.SetPower(cmd.Payload["isOn"].(bool))

	case "setColor":
		h.luaEngine.StopCurrentPattern() // Stop any running script
		r := int(cmd.Payload["r"].(float64))
		g := int(cmd.Payload["g"].(float64))
		b := int(cmd.Payload["b"].(float64))
		h.bleController.SetColor(r, g, b)

	case "setBrightness":
		// NOTE: We intentionally do NOT stop the pattern here,
		// allowing brightness adjustments during an animation.
		h.bleController.SetBrightness(int(cmd.Payload["value"].(float64)))

	case "setSpeed":
		// NOTE: We intentionally do NOT stop the pattern here,
		// allowing speed adjustments during an animation.
		h.bleController.SetSpeed(int(cmd.Payload["value"].(float64)))

	case "setHardwarePattern":
		h.luaEngine.StopCurrentPattern() // Stop any running script
		h.bleController.SetHardwarePattern(int(cmd.Payload["id"].(float64)))

	case "syncTime":
		h.bleController.SyncTime()

	case "setRgbOrder":
		v1 := int(cmd.Payload["v1"].(float64))
		v2 := int(cmd.Payload["v2"].(float64))
		v3 := int(cmd.Payload["v3"].(float64))
		h.bleController.SetRgbOrder(v1, v2, v3)

	case "setSchedule":
		hour := int(cmd.Payload["hour"].(float64))
		minute := int(cmd.Payload["minute"].(float64))
		second := int(cmd.Payload["second"].(float64))
		weekdays := byte(cmd.Payload["weekdays"].(float64))
		isOn := cmd.Payload["isOn"].(bool)
		isSet := cmd.Payload["isSet"].(bool)
		h.bleController.SetSchedule(hour, minute, second, weekdays, isOn, isSet)

	case "runPattern":
		name := cmd.Payload["name"].(string)
		luaStatusCallback := func(runningPattern string) {
			payload := map[string]string{"running": runningPattern}
			hub.Broadcast(server.NewMessage("pattern_status", payload))
		}
		go h.luaEngine.RunPattern(name, luaStatusCallback)

	case "stopPattern":
		h.luaEngine.StopCurrentPattern()
		hub.Broadcast(server.NewMessage("pattern_status", map[string]string{"running": ""}))

	case "addSchedule":
		spec := cmd.Payload["spec"].(string)
		command := cmd.Payload["command"].(string)
		h.scheduler.Add(spec, command)
		hub.Broadcast(server.NewMessage("schedule_list", h.scheduler.GetAll()))

	case "removeSchedule":
		idStr, ok := cmd.Payload["id"].(string)
		if !ok {
			log.Printf("Invalid 'id' for removeSchedule: not a string")
			return
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			log.Printf("Invalid 'id' for removeSchedule: %v", err)
			return
		}
		h.scheduler.Remove(id)
		hub.Broadcast(server.NewMessage("schedule_list", h.scheduler.GetAll()))
		
	case "getPatternCode":
		if name, ok := cmd.Payload["name"].(string); ok {
			content, err := lua.GetPatternCode(name)
			if err != nil {
				log.Printf("Error getting pattern code: %v", err)
				return
			}
			hub.Broadcast(server.NewMessage("pattern_code", map[string]string{"name": filepath.Base(name), "code": content}))
		}

	case "savePatternCode":
		name, nameOk := cmd.Payload["name"].(string)
		code, codeOk := cmd.Payload["code"].(string)
		if nameOk && codeOk {
			if err := lua.SavePatternCode(name, code); err != nil {
				log.Printf("Error saving pattern: %v", err)
				return
			}
			patterns, _ := lua.GetPatternList()
			hub.Broadcast(server.NewMessage("pattern_list", patterns))
		}

	case "deletePattern":
		if name, ok := cmd.Payload["name"].(string); ok {
			if err := lua.DeletePattern(name); err != nil {
				log.Printf("Error deleting pattern: %v", err)
				return
			}
			patterns, _ := lua.GetPatternList()
			hub.Broadcast(server.NewMessage("pattern_list", patterns))
		}

	default:
		log.Printf("Unknown command type: %s", cmd.Type)
	}
}
