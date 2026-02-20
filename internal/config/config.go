package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ServerConfig - налаштування HTTP сервера
type ServerConfig struct {
	Port           string   `json:"port"`
	WebFilesDir    string   `json:"web_files_dir"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// BLEConfig - налаштування Bluetooth Low Energy
type BLEConfig struct {
	DeviceNames       []string `json:"device_names"`
	ScanTimeout       string   `json:"scan_timeout"`
	ConnectTimeout    string   `json:"connect_timeout"`
	HeartbeatInterval string   `json:"heartbeat_interval"`
	RetryDelay        string   `json:"retry_delay"`
	RateLimit         float64  `json:"command_rate_limit"`
	RateBurst         int      `json:"command_rate_burst"`
}

// MQTTConfig - налаштування MQTT та Home Assistant Discovery
type MQTTConfig struct {
	Enabled            bool   `json:"enabled"`
	Broker             string `json:"broker"` // tcp://IP:PORT
	Username           string `json:"username"`
	Password           string `json:"password"`
	ClientID           string `json:"client_id"`
	TopicPrefix        string `json:"topic_prefix"`
	HADiscoveryEnabled bool   `json:"ha_discovery_enabled"`
	HADiscoveryPrefix  string `json:"ha_discovery_prefix"`
}

// Config - головна структура
type Config struct {
	Server ServerConfig `json:"server"`
	BLE    BLEConfig    `json:"ble"`
	MQTT   MQTTConfig   `json:"mqtt"`

	// File system settings
	PatternsDir   string `json:"patterns_dir"`
	SchedulesFile string `json:"schedules_file"`
}

// Load зчитує файл, парсить JSON та застосовує валідацію/дефолти
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := &Config{}
			cfg.setDefaults()
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to open config file '%s': %w", path, err)
	}
	defer file.Close()

	cfg := &Config{}
	if err := json.NewDecoder(file).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	// Тут можна додати зчитування ENV змінних, якщо потрібно
	// if envPort := os.Getenv("SERVER_PORT"); envPort != "" { cfg.Server.Port = envPort }

	cfg.sanitize()
	cfg.setDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) sanitize() {
	c.Server.Port = strings.TrimSpace(c.Server.Port)
	c.Server.WebFilesDir = strings.TrimSpace(c.Server.WebFilesDir)
	c.PatternsDir = strings.TrimSpace(c.PatternsDir)
	c.SchedulesFile = strings.TrimSpace(c.SchedulesFile)

	// Очищення пробілів у назвах девайсів (хоча іноді в BLE іменах важливі пробіли,
	// але зазвичай це помилка копіювання, окрім випадку точного матчингу)
	// Залишимо як є або зробимо TrimSpace залежно від логіки.
	// В оригіналі було "ELK-BLEDOM   ", тому краще не чіпати масив імен тут,
	// або робити це обережно.
}

func (c *Config) setDefaults() {
	// Server Defaults
	if c.Server.Port == "" {
		c.Server.Port = "8080"
	}
	if c.Server.WebFilesDir == "" {
		c.Server.WebFilesDir = "./web"
	}
	if len(c.Server.AllowedOrigins) == 0 {
		c.Server.AllowedOrigins = []string{"http://localhost:8080"}
	}

	// BLE Defaults
	if len(c.BLE.DeviceNames) == 0 {
		c.BLE.DeviceNames = []string{"ELK-BLEDOM   ", "BLEDOM"}
	}
	if c.BLE.ScanTimeout == "" {
		c.BLE.ScanTimeout = "30s"
	}
	if c.BLE.ConnectTimeout == "" {
		c.BLE.ConnectTimeout = "7s"
	}
	if c.BLE.HeartbeatInterval == "" {
		c.BLE.HeartbeatInterval = "60s"
	}
	if c.BLE.RetryDelay == "" {
		c.BLE.RetryDelay = "5s"
	}
	if c.BLE.RateLimit <= 0 {
		c.BLE.RateLimit = 25.0
	}
	if c.BLE.RateBurst <= 0 {
		c.BLE.RateBurst = 25
	}

	// File Defaults
	if c.PatternsDir == "" {
		c.PatternsDir = "patterns"
	}
	if c.SchedulesFile == "" {
		c.SchedulesFile = "schedules.json"
	}

	// MQTT Defaults
	if c.MQTT.Broker == "" {
		c.MQTT.Broker = "tcp://localhost:1883"
	}
	if c.MQTT.ClientID == "" {
		c.MQTT.ClientID = "bledom-controller"
	}
	if c.MQTT.TopicPrefix == "" {
		c.MQTT.TopicPrefix = "bledom"
	}
	if c.MQTT.HADiscoveryPrefix == "" {
		c.MQTT.HADiscoveryPrefix = "homeassistant"
	}
}

func (c *Config) validate() error {
	// Валідація критичних помилок
	if c.BLE.RateLimit <= 0 {
		// Хоча ми ставимо дефолт, якщо користувач явно ввів мінус - це помилка або корекція
		return fmt.Errorf("config error: 'command_rate_limit' must be positive")
	}
	return nil
}
