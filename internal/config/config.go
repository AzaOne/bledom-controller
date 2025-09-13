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

    // File system settings
    PatternsDir   string `json:"PatternsDir"`
    SchedulesFile string `json:"SchedulesFile"`
}
