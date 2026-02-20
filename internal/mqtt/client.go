// Package mqtt provides an MQTT client for remote control and status reporting.
package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"bledom-controller/internal/config"
	"bledom-controller/internal/core"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Client handles communication with an MQTT broker.
type Client struct {
	client mqtt.Client
	cfg    *config.Config
	prefix string

	eventBus        *core.EventBus
	commandChannel  core.CommandChannel
	state           *core.State
	patternListFunc func() ([]string, error)
}

// NewClient creates a new MQTT client with robust reconnection logic.
func NewClient(cfg *config.Config, eb *core.EventBus, st *core.State, cmdChan core.CommandChannel, patternListFunc func() ([]string, error)) *Client {
	if !cfg.MQTT.Enabled {
		return nil
	}

	// Format topic prefix, removing trailing slashes
	prefix := strings.TrimSuffix(cfg.MQTT.TopicPrefix, "/")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTT.Broker)
	opts.SetClientID(cfg.MQTT.ClientID)
	opts.SetUsername(cfg.MQTT.Username)
	opts.SetPassword(cfg.MQTT.Password)

	// --- Connection Stability Settings ---

	// KeepAlive: frequency of pinging the broker
	opts.SetKeepAlive(10 * time.Second)
	// PingTimeout: how long to wait for a ping response
	opts.SetPingTimeout(5 * time.Second)

	// AutoReconnect: automatically restore connection after drop
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(1 * time.Minute)

	// ConnectRetry: attempts to connect at startup even if broker is down (important for Docker)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)

	// OrderMatters: false improves throughput by not blocking message processing order
	opts.SetOrderMatters(false)

	// LWT (Last Will and Testament): Message broker sends if we disconnect unexpectedly
	opts.SetWill(prefix+"/availability", "offline", 1, true)

	c := &Client{
		cfg:             cfg,
		prefix:          prefix,
		eventBus:        eb,
		state:           st,
		commandChannel:  cmdChan,
		patternListFunc: patternListFunc,
	}

	opts.SetOnConnectHandler(c.onConnect)

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("[MQTT] Connection lost: %v. Retrying in background...", err)
	})

	opts.SetReconnectingHandler(func(client mqtt.Client, options *mqtt.ClientOptions) {
		log.Println("[MQTT] Attempting to reconnect...")
	})

	c.client = mqtt.NewClient(opts)

	// Subscribe to internal events
	go c.listenEvents()

	return c
}

// listenEvents subscribes to the event bus and publishes state changes to MQTT.
func (c *Client) listenEvents() {
	if c.eventBus == nil {
		return
	}

	sub := c.eventBus.Subscribe(
		core.StateChangedEvent,
		core.DeviceConnectedEvent,
		core.PatternChangedEvent,
		core.PowerChangedEvent,
		core.ColorChangedEvent,
	)

	for event := range sub {
		switch event.Type {
		case core.DeviceConnectedEvent:
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if connected, ok := payload["connected"].(bool); ok {
					statusStr := "disconnected"
					if connected {
						statusStr = "connected"
					}
					c.Publish("connection", statusStr, true)

					if connected {
						if rssi, ok := payload["rssi"].(int16); ok {
							c.Publish("rssi", rssi, false)
						}
					}
				}
			}
		case core.StateChangedEvent:
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if powerIsOn, ok := payload["isOn"].(bool); ok {
					powerStr := "OFF"
					if powerIsOn {
						powerStr = "ON"
					}
					c.Publish("power/state", powerStr, true)
				}
				if brightness, ok := payload["brightness"].(int); ok {
					c.Publish("brightness/state", brightness, true)
				}
				if r, okR := payload["r"].(int); okR {
					if g, okG := payload["g"].(int); okG {
						if b, okB := payload["b"].(int); okB {
							c.Publish("color/state", fmt.Sprintf("%d,%d,%d", r, g, b), true)
						}
					}
				}
			}

		case core.PatternChangedEvent:
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if pattern, ok := payload["running"].(string); ok {
					state := pattern
					if state == "" {
						state = "IDLE"
					}
					c.Publish("pattern/state", state, true)
				}
			}
		case core.PowerChangedEvent:
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if powerIsOn, ok := payload["isOn"].(bool); ok {
					powerStr := "OFF"
					if powerIsOn {
						powerStr = "ON"
					}
					c.Publish("power/state", powerStr, true)
				}
			}
		case core.ColorChangedEvent:
			if payload, ok := event.Payload.(map[string]interface{}); ok {
				if hex, okHex := payload["hex"].(string); okHex {
					c.Publish("color/state/hex", hex, true)
				}
				if r, okR := payload["r"].(int); okR {
					if g, okG := payload["g"].(int); okG {
						if b, okB := payload["b"].(int); okB {
							c.Publish("color/state", fmt.Sprintf("%d,%d,%d", r, g, b), true)
						}
					}
				}
			}
		}
	}
}

// Connect initiates the connection to the MQTT broker.
func (c *Client) Connect() error {
	if c.client == nil {
		return nil
	}
	log.Printf("[MQTT] Starting connection loop to %s...", c.cfg.MQTT.Broker)

	token := c.client.Connect()
	// Wait for the first handshake attempt.
	if token.Wait() && token.Error() != nil {
		log.Printf("[MQTT] Initial connection error: %v", token.Error())
		return token.Error()
	}

	return nil
}

// Disconnect gracefully closes the MQTT connection, sending an offline status first.
func (c *Client) Disconnect() {
	if c.client != nil && c.client.IsConnected() {
		log.Println("[MQTT] Disconnecting...")

		// 1. Explicitly send offline status before disconnecting
		token := c.client.Publish(c.prefix+"/availability", 0, true, "offline")

		// Wait for publication with timeout
		if token.WaitTimeout(2 * time.Second) {
			if token.Error() != nil {
				log.Printf("[MQTT] Warning: failed to publish offline status: %v", token.Error())
			}
		} else {
			log.Println("[MQTT] Warning: timed out publishing offline status")
		}

		// 2. Close connection
		c.client.Disconnect(250)
		log.Println("[MQTT] Disconnected.")
	}
}

// Publish sends a message to a specific subtopic under the configured prefix.
func (c *Client) Publish(subtopic string, payload interface{}, retained bool) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	topic := fmt.Sprintf("%s/%s", c.prefix, subtopic)
	msg := fmt.Sprintf("%v", payload)

	token := c.client.Publish(topic, 0, retained, msg)

	// Wait in a separate goroutine to avoid blocking
	go func() {
		if token.WaitTimeout(5 * time.Second) {
			if token.Error() != nil {
				log.Printf("[MQTT] Publish error to %s: %v", topic, token.Error())
			}
		} else {
			log.Printf("[MQTT] Timeout publishing to %s", topic)
		}
	}()
}

// onConnect is the library callback triggered when the MQTT connection is established.
func (c *Client) onConnect(client mqtt.Client) {
	log.Println("[MQTT] Connected to broker.")

	// Topic subscriptions
	topics := map[string]mqtt.MessageHandler{
		"power/set":      c.handlePower,
		"brightness/set": c.handleBrightness,
		"color/set":      c.handleColor,
		"pattern/run":    c.handlePatternRun,
		"pattern/stop":   c.handlePatternStop,
	}

	for sub, handler := range topics {
		topic := fmt.Sprintf("%s/%s", c.prefix, sub)
		if token := client.Subscribe(topic, 1, handler); token.Wait() && token.Error() != nil {
			log.Printf("[MQTT] Error subscribing to %s: %v", topic, token.Error())
		} else {
			log.Printf("[MQTT] Subscribed to %s", topic)
		}
	}

	// Send Discovery and Online status
	go func() {
		c.Publish("availability", "online", true)
		if c.cfg.MQTT.HADiscoveryEnabled {
			c.PublishHADiscovery()
		}
	}()
}

// PublishHADiscovery sends Home Assistant discovery configuration.
func (c *Client) PublishHADiscovery() {
	// Wait a moment to ensure subscriptions are processed
	time.Sleep(1 * time.Second)

	patterns := []string{}
	if c.patternListFunc != nil {
		if list, err := c.patternListFunc(); err == nil {
			patterns = list
		} else {
			log.Printf("[MQTT] Could not get pattern list for HA Discovery: %v", err)
		}
	}

	safeID := strings.ReplaceAll(c.cfg.MQTT.ClientID, " ", "_")
	// Sanitize ID
	safeID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, safeID)

	discoveryTopic := fmt.Sprintf("%s/light/%s/light/config", c.cfg.MQTT.HADiscoveryPrefix, safeID)

	payload := map[string]interface{}{
		"name":      "Light",
		"unique_id": safeID + "_light",
		"object_id": safeID,
		"icon":      "mdi:led-strip",

		// power
		"command_topic": fmt.Sprintf("%s/power/set", c.prefix),
		"state_topic":   fmt.Sprintf("%s/power/state", c.prefix),

		// brightness
		"brightness_command_topic": fmt.Sprintf("%s/brightness/set", c.prefix),
		"brightness_state_topic":   fmt.Sprintf("%s/brightness/state", c.prefix),
		"brightness_scale":         100,

		// color
		"rgb_command_topic": fmt.Sprintf("%s/color/set", c.prefix),
		"rgb_state_topic":   fmt.Sprintf("%s/color/state", c.prefix),

		// effects
		"effect_command_topic": fmt.Sprintf("%s/pattern/run", c.prefix),
		"effect_state_topic":   fmt.Sprintf("%s/pattern/state", c.prefix),
		"effect_list":          patterns,

		// availability
		"availability_mode": "all",
		"availability": []map[string]string{
			{
				"topic":                 fmt.Sprintf("%s/availability", c.prefix),
				"payload_available":     "online",
				"payload_not_available": "offline",
			},
			{
				"topic":                 fmt.Sprintf("%s/connection", c.prefix),
				"payload_available":     "connected",
				"payload_not_available": "disconnected",
			},
		},

		// device
		"device": map[string]interface{}{
			"identifiers":  []string{safeID},
			"name":         "BLEDOM Controller",
			"manufacturer": "AzaOne",
			"model":        "BLEDOM BLE Agent",
			"sw_version":   "1.5-mqtt-robust",
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	c.client.Publish(discoveryTopic, 0, true, jsonPayload)
	log.Printf("[MQTT] HA Discovery sent to %s", discoveryTopic)
}

// --- Handlers ---

// handlePower processes incoming power toggle commands from MQTT.
func (c *Client) handlePower(client mqtt.Client, msg mqtt.Message) {
	payload := strings.ToLower(string(msg.Payload()))
	var isOn bool
	switch payload {
	case "on", "true", "1":
		isOn = true
	case "off", "false", "0":
		isOn = false
	default:
		return
	}

	c.commandChannel <- core.Command{
		Type: core.CmdSetPower,
		Payload: map[string]interface{}{
			"isOn": isOn,
		},
	}
}

// handleBrightness processes incoming brightness level commands from MQTT.
func (c *Client) handleBrightness(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	val, err := strconv.Atoi(payload)
	if err == nil {
		c.commandChannel <- core.Command{
			Type: core.CmdSetBrightness,
			Payload: map[string]interface{}{
				"value": float64(val),
			},
		}
	}
}

// handleColor processes incoming color change commands (HEX or RGB) from MQTT.
func (c *Client) handleColor(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())

	var r, g, b int
	processed := false

	// Parsing logic (HEX or RGB)
	if strings.HasPrefix(payload, "#") || len(payload) == 6 {
		cleanHex := strings.TrimPrefix(payload, "#")
		if _, err := fmt.Sscanf(cleanHex, "%02x%02x%02x", &r, &g, &b); err == nil {
			processed = true
		}
	} else if strings.Contains(payload, ",") {
		parts := strings.Split(payload, ",")
		if len(parts) == 3 {
			r, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
			g, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			b, _ = strconv.Atoi(strings.TrimSpace(parts[2]))
			processed = true
		}
	}

	if processed {
		c.commandChannel <- core.Command{
			Type: core.CmdSetColor,
			Payload: map[string]interface{}{
				"r": float64(r),
				"g": float64(g),
				"b": float64(b),
			},
		}
	}
}

// handlePatternRun processes incoming Lua pattern execution commands from MQTT.
func (c *Client) handlePatternRun(client mqtt.Client, msg mqtt.Message) {
	name := string(msg.Payload())
	c.commandChannel <- core.Command{
		Type: core.CmdRunPattern,
		Payload: map[string]interface{}{
			"name": name,
		},
	}
}

// handlePatternStop processes incoming Lua pattern stop commands from MQTT.
func (c *Client) handlePatternStop(client mqtt.Client, msg mqtt.Message) {
	c.commandChannel <- core.Command{
		Type:    core.CmdStopPattern,
		Payload: nil,
	}
}
