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
	"bledom-controller/internal/server"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	client        mqtt.Client
	cfg           *config.Config
	bleController *ble.Controller
	luaEngine     *lua.Engine
	hub           *server.Hub // Доступ до Hub для оновлення WS
	prefix        string
}

// NewClient створює клієнта з покращеною логікою реконекту.
func NewClient(cfg *config.Config, bc *ble.Controller, le *lua.Engine, hub *server.Hub) *Client {
	if !cfg.MQTT.Enabled {
		return nil
	}

	// Формуємо префікс топіків, прибираючи зайві слеші в кінці
	prefix := strings.TrimSuffix(cfg.MQTT.TopicPrefix, "/")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTT.Broker)
	opts.SetClientID(cfg.MQTT.ClientID)
	opts.SetUsername(cfg.MQTT.Username)
	opts.SetPassword(cfg.MQTT.Password)

	// --- Налаштування стабільності з'єднання ---

	// KeepAlive: частота пінгування брокера (10 сек)
	opts.SetKeepAlive(10 * time.Second)
	// PingTimeout: скільки чекати відповіді на пінг (5 сек)
	opts.SetPingTimeout(5 * time.Second)

	// AutoReconnect: відновлювати з'єднання після розриву
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(1 * time.Minute)

	// ConnectRetry: намагатися підключитися при старті, навіть якщо брокер лежить (важливо для Docker)
	// Це дозволяє уникнути негайного падіння, якщо MQTT контейнер ще не завантажився.
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)

	// OrderMatters: false покращує пропускну здатність, оскільки не блокує обробку повідомлень
	opts.SetOrderMatters(false)

	// LWT (Last Will and Testament): Повідомлення, яке брокер надішле сам, якщо ми "впадемо"
	opts.SetWill(prefix+"/availability", "offline", 1, true)

	c := &Client{
		cfg:           cfg,
		bleController: bc,
		luaEngine:     le,
		hub:           hub,
		prefix:        prefix,
	}

	opts.SetOnConnectHandler(c.onConnect)

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("[MQTT] Connection lost: %v. Retrying in background...", err)
	})

	opts.SetReconnectingHandler(func(client mqtt.Client, options *mqtt.ClientOptions) {
		log.Println("[MQTT] Attempting to reconnect...")
	})

	c.client = mqtt.NewClient(opts)

	return c
}

// Connect ініціює підключення.
func (c *Client) Connect() error {
	if c.client == nil {
		return nil
	}
	log.Printf("[MQTT] Starting connection loop to %s...", c.cfg.MQTT.Broker)

	token := c.client.Connect()
	// Чекаємо завершення першої спроби рукостискання.
	// Якщо ConnectRetry=true, то помилка тут означатиме скоріше проблеми з конфігурацією (наприклад, невалідний протокол),
	// аніж просто недоступність мережі.
	if token.Wait() && token.Error() != nil {
		log.Printf("[MQTT] Initial connection error: %v", token.Error())
		return token.Error()
	}

	return nil
}

// Disconnect коректно завершує роботу: спочатку надсилає offline статус, потім закриває сокет.
func (c *Client) Disconnect() {
	if c.client != nil && c.client.IsConnected() {
		log.Println("[MQTT] Disconnecting...")

		// 1. Явно відправляємо статус offline перед розривом
		token := c.client.Publish(c.prefix+"/availability", 0, true, "offline")
		
		// Чекаємо завершення публікації з таймаутом (щоб не зависнути при виході)
		if token.WaitTimeout(2 * time.Second) {
			if token.Error() != nil {
				log.Printf("[MQTT] Warning: failed to publish offline status: %v", token.Error())
			}
		} else {
			log.Println("[MQTT] Warning: timed out publishing offline status")
		}

		// 2. Закриваємо з'єднання
		c.client.Disconnect(250)
		log.Println("[MQTT] Disconnected.")
	}
}

func (c *Client) Publish(subtopic string, payload interface{}, retained bool) {
	if c.client == nil || !c.client.IsConnected() {
		return
	}

	topic := fmt.Sprintf("%s/%s", c.prefix, subtopic)
	msg := fmt.Sprintf("%v", payload)

	token := c.client.Publish(topic, 0, retained, msg)

	// Не блокуємо основний потік, але і не допускаємо витоку горутин
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

// onConnect викликається бібліотекою Paho у внутрішній горутині обробки подій.
func (c *Client) onConnect(client mqtt.Client) {
	log.Println("[MQTT] Connected to broker.")

	// Підписка на топіки
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

	// Відправка Discovery та статусу Online.
	// Виконуємо в окремій горутині, щоб не блокувати onConnect (оскільки PublishHADiscovery має sleep).
	go func() {
		c.Publish("availability", "online", true)
		if c.cfg.MQTT.HADiscoveryEnabled {
			c.PublishHADiscovery()
		}
	}()
}

// PublishHADiscovery надсилає конфігурацію для Home Assistant
func (c *Client) PublishHADiscovery() {
	// Даємо трохи часу, щоб переконатися, що підписки пройшли і система стабільна
	time.Sleep(1 * time.Second)

	patterns, err := c.luaEngine.GetPatternList()
	if err != nil {
		log.Printf("[MQTT] Warning: Could not get patterns for HA discovery: %v", err)
		patterns = []string{}
	}

	safeID := strings.ReplaceAll(c.cfg.MQTT.ClientID, " ", "_")
	// Санітизація ID
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

		"command_topic": fmt.Sprintf("%s/power/set", c.prefix),
		"state_topic":   fmt.Sprintf("%s/power/state", c.prefix),

		"brightness_command_topic": fmt.Sprintf("%s/brightness/set", c.prefix),
		"brightness_state_topic":   fmt.Sprintf("%s/brightness/state", c.prefix),
		"brightness_scale":         100,

		"rgb_command_topic": fmt.Sprintf("%s/color/set", c.prefix),
		"rgb_state_topic":   fmt.Sprintf("%s/color/state", c.prefix),

		"effect_command_topic": fmt.Sprintf("%s/pattern/run", c.prefix),
		"effect_state_topic":   fmt.Sprintf("%s/pattern/state", c.prefix),
		"effect_list":          patterns,

		// Налаштування доступності (Availability)
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

// --- Handlers (Sync to WS and update State) ---

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

	c.luaEngine.StopCurrentPattern()
	c.bleController.SetPower(isOn)

	// Update MQTT State
	c.Publish("power/state", strings.ToUpper(payload), true)

	// Sync WebSocket clients
	c.hub.Broadcast(server.NewMessage("power_update", map[string]bool{"isOn": isOn}))
}

func (c *Client) handleBrightness(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	val, err := strconv.Atoi(payload)
	if err == nil {
		c.bleController.SetBrightness(val)
		c.Publish("brightness/state", val, true)

		// Sync WebSocket clients
		c.hub.Broadcast(server.NewMessage("brightness_update", map[string]int{"value": val}))
	}
}

func (c *Client) handleColor(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	c.luaEngine.StopCurrentPattern()

	var r, g, b int
	var colorHex string
	processed := false

	// Логіка парсингу (HEX або RGB)
	if strings.HasPrefix(payload, "#") || len(payload) == 6 {
		cleanHex := strings.TrimPrefix(payload, "#")
		if _, err := fmt.Sscanf(cleanHex, "%02x%02x%02x", &r, &g, &b); err == nil {
			processed = true
			colorHex = fmt.Sprintf("#%02X%02X%02X", r, g, b)
		}
	} else if strings.Contains(payload, ",") {
		parts := strings.Split(payload, ",")
		if len(parts) == 3 {
			r, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
			g, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			b, _ = strconv.Atoi(strings.TrimSpace(parts[2]))
			processed = true
			colorHex = fmt.Sprintf("#%02X%02X%02X", r, g, b)
		}
	}

	if processed {
		c.bleController.SetColor(r, g, b)

		rgbString := fmt.Sprintf("%d,%d,%d", r, g, b)
		c.Publish("color/state", rgbString, true)

		// Для Web UI відправляємо HEX і окремі компоненти
		c.hub.Broadcast(server.NewMessage("color_update", map[string]interface{}{
			"r": r, "g": g, "b": b, "hex": colorHex,
		}))
	}
}

func (c *Client) handlePatternRun(client mqtt.Client, msg mqtt.Message) {
	name := string(msg.Payload())
	// Callback агента, переданий в engine, зробить Broadcast статусу
	go c.luaEngine.RunPattern(name, nil)
}

func (c *Client) handlePatternStop(client mqtt.Client, msg mqtt.Message) {
	c.luaEngine.StopCurrentPattern()
}
