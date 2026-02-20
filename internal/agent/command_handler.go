package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/lua"
	"bledom-controller/internal/mqtt"
	"bledom-controller/internal/scheduler"
	"bledom-controller/internal/server"
)

type CommandHandler struct {
	bleController  *ble.Controller
	luaEngine      *lua.Engine
	scheduler      *scheduler.Scheduler
	mqttClient     *mqtt.Client
	onStatusChange func(string)
}

func NewCommandHandler(bc *ble.Controller, le *lua.Engine, s *scheduler.Scheduler, mc *mqtt.Client, onStatusChange func(string)) *CommandHandler {
	return &CommandHandler{
		bleController:  bc,
		luaEngine:      le,
		scheduler:      s,
		mqttClient:     mc,
		onStatusChange: onStatusChange,
	}
}

func (h *CommandHandler) Handle(msg server.Message, hub *server.Hub) {
	var cmd server.Command
	if err := json.Unmarshal(msg.Raw, &cmd); err != nil {
		log.Printf("Error unmarshalling command: %v", err)
		return
	}

	switch cmd.Type {

	case "setPower":
		h.luaEngine.StopCurrentPattern()
		isOn := cmd.Payload["isOn"].(bool)
		h.bleController.SetPower(isOn)

		if h.mqttClient != nil {
			state := "OFF"
			if isOn {
				state = "ON"
			}
			h.mqttClient.Publish("power/state", state, true)
		}
		hub.Broadcast(server.NewMessage("power_update", map[string]bool{"isOn": isOn}))

	case "setColor":
		h.luaEngine.StopCurrentPattern()
		r := int(cmd.Payload["r"].(float64))
		g := int(cmd.Payload["g"].(float64))
		b := int(cmd.Payload["b"].(float64))
		h.bleController.SetColor(r, g, b)

		hex := fmt.Sprintf("#%02X%02X%02X", r, g, b)
		rgbString := fmt.Sprintf("%d,%d,%d", r, g, b)

		if h.mqttClient != nil {
			h.mqttClient.Publish("color/state", rgbString, true)
		}
		hub.Broadcast(server.NewMessage("color_update", map[string]interface{}{
			"r": r, "g": g, "b": b, "hex": hex,
		}))

	case "setBrightness":
		val := int(cmd.Payload["value"].(float64))
		h.bleController.SetBrightness(val)

		if h.mqttClient != nil {
			h.mqttClient.Publish("brightness/state", val, true)
		}
		hub.Broadcast(server.NewMessage("brightness_update", map[string]int{"value": val}))

	case "setSpeed":
		h.bleController.SetSpeed(int(cmd.Payload["value"].(float64)))

	case "setHardwarePattern":
		h.luaEngine.StopCurrentPattern()
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
		go h.luaEngine.RunPattern(name, h.onStatusChange)

	case "stopPattern":
		h.luaEngine.StopCurrentPattern()

	case "addSchedule":
		spec := cmd.Payload["spec"].(string)
		command := cmd.Payload["command"].(string)
		h.scheduler.Add(spec, command)
		hub.Broadcast(server.NewMessage("schedule_list", h.scheduler.GetAll()))

	case "removeSchedule":
		idStr, ok := cmd.Payload["id"].(string)
		if !ok {
			return
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return
		}
		h.scheduler.Remove(id)
		hub.Broadcast(server.NewMessage("schedule_list", h.scheduler.GetAll()))

	case "getPatternCode":
		if name, ok := cmd.Payload["name"].(string); ok {
			content, err := h.luaEngine.GetPatternCode(name)
			if err != nil {
				log.Printf("Error getting pattern code: %v", err)
				return
			}
			hub.Broadcast(server.NewMessage("pattern_code", map[string]string{"name": name, "code": content}))
		}

	case "savePatternCode":
		name, nameOk := cmd.Payload["name"].(string)
		code, codeOk := cmd.Payload["code"].(string)
		if nameOk && codeOk {
			if err := h.luaEngine.SavePatternCode(name, code); err != nil {
				log.Printf("Error saving pattern: %v", err)
				return
			}
			patterns, _ := h.luaEngine.GetPatternList()
			hub.Broadcast(server.NewMessage("pattern_list", patterns))
		}

	case "deletePattern":
		if name, ok := cmd.Payload["name"].(string); ok {
			if err := h.luaEngine.DeletePattern(name); err != nil {
				log.Printf("Error deleting pattern: %v", err)
				return
			}
			patterns, _ := h.luaEngine.GetPatternList()
			hub.Broadcast(server.NewMessage("pattern_list", patterns))
		}

	default:
		log.Printf("Unknown command type: %s", cmd.Type)
	}
}
