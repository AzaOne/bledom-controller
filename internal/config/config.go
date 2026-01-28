package config

// Config holds the application's configurable settings.
type Config struct {
    // Server settings
    ServerPort     string   `json:"ServerPort"`
    StaticFilesDir string   `json:"StaticFilesDir"`
    AllowedOrigins []string `json:"AllowedOrigins"`

    // BLE settings
    DeviceNames           []string `json:"DeviceNames"`
    BLEScanTimeout        string   `json:"BLEScanTimeout"`
    BLEConnectTimeout     string   `json:"BLEConnectTimeout"`
    BLEHeartbeatInterval  string   `json:"BLEHeartbeatInterval"`
    BLERetryDelay         string   `json:"BLERetryDelay"`
    BLECommandRateLimitRate  float64 `json:"BLECommandRateLimitRate"`
    BLECommandRateLimitBurst int     `json:"BLECommandRateLimitBurst"`
    
    // File system settings
    PatternsDir   string `json:"PatternsDir"`
    SchedulesFile string `json:"SchedulesFile"`
    
    // MQTT settings
    MQTTEnabled     bool   `json:"MQTTEnabled"`
    MQTTBroker      string `json:"MQTTBroker"`      // e.g., "tcp://localhost:1883"
    MQTTUsername    string `json:"MQTTUsername"`
    MQTTPassword    string `json:"MQTTPassword"`
    MQTTClientId    string `json:"MQTTClientId"`
    MQTTTopicPrefix string `json:"MQTTTopicPrefix"` // e.g., "bledom"
    
    MQTTHADiscoveryEnabled bool   `json:"MQTTHADiscoveryEnabled"` // Enable Auto Discovery
    MQTTHADiscoveryPrefix  string `json:"MQTTHADiscoveryPrefix"`  // Default: "homeassistant"
}
