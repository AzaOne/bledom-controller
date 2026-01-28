package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"bledom-controller/internal/ble"
	"bledom-controller/internal/config"
	"bledom-controller/internal/lua"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Client manages the MQTT connection and subscriptions.
type Client struct {
	client        mqtt.Client
	cfg           *config.Config
	bleController *ble.Controller
	luaEngine     *lua.Engine
	prefix        string
}

// NewClient creates a new MQTT client wrapper.
func NewClient(cfg *config.Config, bc *ble.Controller, le *lua.Engine) *Client {
	if !cfg.MQTTEnabled {
		return nil
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientId)
	opts.SetUsername(cfg.MQTTUsername)
	opts.SetPassword(cfg.MQTTPassword)
	opts.SetAutoReconnect(true)

	// Set Last Will and Testament (LWT)
	prefix := strings.TrimSuffix(cfg.MQTTTopicPrefix, "/")
	opts.SetWill(prefix+"/availability", "offline", 1, true)

	c := &Client{
		cfg:           cfg,
		bleController: bc,
		luaEngine:     le,
		prefix:        prefix,
	}

	opts.SetOnConnectHandler(c.onConnect)
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("[MQTT] Connection lost: %v", err)
	})

	c.client = mqtt.NewClient(opts)

	return c
}

// Connect starts the MQTT connection.
func (c *Client) Connect() error {
	if c.client == nil {
		return nil
	}
	log.Printf("[MQTT] Connecting to %s...", c.cfg.MQTTBroker)
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// Disconnect cleans up the connection gracefully.
func (c *Client) Disconnect() {
	if c.client != nil && c.client.IsConnected() {
		c.Publish("availability", "offline", true)
		c.client.Disconnect(250)
		log.Println("[MQTT] Disconnected.")
	}
}

// Publish sends a message to a subtopic.
func (c *Client) Publish(subtopic string, payload interface{}, retained bool) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}
	topic := fmt.Sprintf("%s/%s", c.prefix, subtopic)
	msg := fmt.Sprintf("%v", payload)

	token := c.client.Publish(topic, 0, retained, msg)
	token.Wait()
}

// onConnect is called when the client successfully connects.
func (c *Client) onConnect(client mqtt.Client) {
	log.Println("[MQTT] Connected to broker.")

	c.Publish("availability", "online", true)

	// Trigger Home Assistant Discovery
	if c.cfg.MQTTHADiscoveryEnabled {
		go c.PublishHADiscovery()
	}

	topics := map[string]mqtt.MessageHandler{
		"power/set":      c.handlePower,
		"brightness/set": c.handleBrightness,
		"color/set":      c.handleColor,
		"pattern/run":    c.handlePatternRun,
		"pattern/stop":   c.handlePatternStop,
	}

	for sub, handler := range topics {
		topic := fmt.Sprintf("%s/%s", c.prefix, sub)
		if token := client.Subscribe(topic, 0, handler); token.Wait() && token.Error() != nil {
			log.Printf("[MQTT] Error subscribing to %s: %v", topic, token.Error())
		} else {
			log.Printf("[MQTT] Subscribed to %s", topic)
		}
	}
}

// PublishHADiscovery sends the configuration payload for Home Assistant Auto Discovery.
func (c *Client) PublishHADiscovery() {
	// Small delay to ensure Lua engine has loaded patterns
	time.Sleep(2 * time.Second)

	patterns, err := c.luaEngine.GetPatternList()
	if err != nil {
		log.Printf("[MQTT] Warning: Could not get patterns for HA discovery: %v", err)
		patterns = []string{}
	}

	// Create a safe unique ID based on the configured ClientID
	safeID := strings.ReplaceAll(c.cfg.MQTTClientId, " ", "_")
	safeID = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1
	}, safeID)

	// Topic structure: <discovery_prefix>/light/<node_id>/<object_id>/config
	discoveryTopic := fmt.Sprintf("%s/light/%s/light/config", c.cfg.MQTTHADiscoveryPrefix, safeID)

	payload := map[string]interface{}{
		"name":      "Light",
		"unique_id": safeID + "_light",
		"object_id": safeID,
		"icon":      "mdi:led-strip",

		// Power
		"command_topic": fmt.Sprintf("%s/power/set", c.prefix),
		"state_topic":   fmt.Sprintf("%s/power/state", c.prefix),

		// Brightness
		"brightness_command_topic": fmt.Sprintf("%s/brightness/set", c.prefix),
		"brightness_state_topic":   fmt.Sprintf("%s/brightness/state", c.prefix),
		"brightness_scale":         100, // BLEDOM uses 0-100, HA defaults to 0-255

		// RGB Color
		"rgb_command_topic": fmt.Sprintf("%s/color/set", c.prefix),
		"rgb_state_topic":   fmt.Sprintf("%s/color/state", c.prefix),

		// Effects (Lua Patterns)
		"effect_command_topic": fmt.Sprintf("%s/pattern/run", c.prefix),
		"effect_state_topic":   fmt.Sprintf("%s/pattern/state", c.prefix),
		"effect_list":          patterns,

		// Availability
		"availability_topic":    fmt.Sprintf("%s/availability", c.prefix),
		"payload_available":     "online",
		"payload_not_available": "offline",

		// Device Registry
		"device": map[string]interface{}{
			"identifiers":  []string{safeID},
			"name":         "BLEDOM Controller",
			"manufacturer": "AzaOne",
			"model":        "BLEDOM BLE Agent",
			"sw_version":   "1.1-mqtt",
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[MQTT] Error marshaling HA discovery: %v", err)
		return
	}

	log.Printf("[MQTT] Sending Home Assistant discovery config to %s", discoveryTopic)
	// Retained message so HA sees it on reboot
	token := c.client.Publish(discoveryTopic, 0, true, jsonPayload)
	token.Wait()
}

// --- Message Handlers ---

func (c *Client) handlePower(client mqtt.Client, msg mqtt.Message) {
	payload := strings.ToLower(string(msg.Payload()))
	log.Printf("[MQTT] Command received: power %s", payload)

	switch payload {
	case "on", "true", "1":
		c.luaEngine.StopCurrentPattern()
		c.bleController.SetPower(true)
		c.Publish("power/state", "ON", true)
	case "off", "false", "0":
		c.luaEngine.StopCurrentPattern()
		c.bleController.SetPower(false)
		c.Publish("power/state", "OFF", true)
	}
}

func (c *Client) handleBrightness(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	val, err := strconv.Atoi(payload)
	if err == nil {
		log.Printf("[MQTT] Command received: brightness %d", val)
		c.bleController.SetBrightness(val)
		c.Publish("brightness/state", val, true)
	}
}

func (c *Client) handleColor(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	log.Printf("[MQTT] Command received: color %s", payload)

	c.luaEngine.StopCurrentPattern()

	var r, g, b int

	// 1. Try Hex Format (#RRGGBB or RRGGBB)
	cleanHex := strings.TrimPrefix(payload, "#")
	if len(cleanHex) == 6 {
		if _, err := fmt.Sscanf(cleanHex, "%02x%02x%02x", &r, &g, &b); err == nil {
			c.bleController.SetColor(r, g, b)
			c.Publish("color/state", fmt.Sprintf("#%02X%02X%02X", r, g, b), true)
			return
		}
	}

	// 2. Try JSON Format {"r": 255, "g": 0, "b": 0}
	type ColorJSON struct {
		R int `json:"r"`
		G int `json:"g"`
		B int `json:"b"`
	}
	var cObj ColorJSON
	if err := json.Unmarshal(msg.Payload(), &cObj); err == nil {
		c.bleController.SetColor(cObj.R, cObj.G, cObj.B)
		c.Publish("color/state", fmt.Sprintf("#%02X%02X%02X", cObj.R, cObj.G, cObj.B), true)
		return
	}

	// 3. Try CSV Format "255,0,0" (Default HA RGB format)
	parts := strings.Split(payload, ",")
	if len(parts) == 3 {
		r, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
		g, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		b, _ = strconv.Atoi(strings.TrimSpace(parts[2]))
		c.bleController.SetColor(r, g, b)
		// Convert back to Hex for state consistency, or send CSV back if preferred
		c.Publish("color/state", fmt.Sprintf("%d,%d,%d", r, g, b), true)
	}
}

func (c *Client) handlePatternRun(client mqtt.Client, msg mqtt.Message) {
	name := string(msg.Payload())
	log.Printf("[MQTT] Command received: run pattern %s", name)
	go c.luaEngine.RunPattern(name, nil)
}

func (c *Client) handlePatternStop(client mqtt.Client, msg mqtt.Message) {
	log.Printf("[MQTT] Command received: stop pattern")
	c.luaEngine.StopCurrentPattern()
}
