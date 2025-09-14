package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "bledom-controller/internal/agent"
    "bledom-controller/internal/config"
)

// These variables will be set by the build script
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

// LoadConfig reads the configuration from config.json, applying defaults.
func LoadConfig(path string) (*config.Config, error) {
    // Initialize with default values
    cfg := config.Config{
        ServerPort:           "8080",
        StaticFilesDir:       "./static",
        AllowedOrigins:       []string{"http://localhost:8080"},
        DeviceNames:          []string{"ELK-BLEDOM   ", "BLEDOM"},
        BLEScanTimeout:       "30s",
        BLEConnectTimeout:    "7s",
        BLEHeartbeatInterval: "60s",
        BLERetryDelay:        "5s",
        BLECommandRateLimitRate: 25.0,
        BLECommandRateLimitBurst: 25,
        PatternsDir:          "patterns",
        SchedulesFile:        "schedules.json",
    }

    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            log.Printf("Config file '%s' not found. Using default values.", path)
            return &cfg, nil
        }
        return nil, fmt.Errorf("failed to read config file '%s': %w", path, err)
    }

    // Unmarshal into the cfg struct, which will override defaults if specified in JSON
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to unmarshal config file '%s': %w", path, err)
    }

    // Post-unmarshal validation/defaulting for essential fields if they could be empty in JSON
    if cfg.ServerPort == "" {
        cfg.ServerPort = "8080"
    }
    if cfg.StaticFilesDir == "" {
        cfg.StaticFilesDir = "./static"
    }
    if cfg.PatternsDir == "" {
        cfg.PatternsDir = "patterns"
    }
    if cfg.SchedulesFile == "" {
        cfg.SchedulesFile = "schedules.json"
    }
    // Ensure rate limit values are sensible
    if cfg.BLECommandRateLimitRate <= 0 {
        cfg.BLECommandRateLimitRate = 1.0 // Minimum 1 command/sec
        log.Printf("Warning: BLECommandRateLimitRate was invalid or zero, defaulted to %.1f", cfg.BLECommandRateLimitRate)
    }
    if cfg.BLECommandRateLimitBurst <= 0 {
        cfg.BLECommandRateLimitBurst = 1 // Minimum burst of 1
        log.Printf("Warning: BLECommandRateLimitBurst was invalid or zero, defaulted to %d", cfg.BLECommandRateLimitBurst)
    }


    return &cfg, nil
}

func main() {
    log.Printf("Starting BLEDOM Controller Agent version: %s, commit: %s, built: %s", version, commit, date)

    configPath := "./config.json"
    cfg, err := LoadConfig(configPath)
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }
    log.Printf("Configuration loaded: ServerPort=%s, StaticFilesDir=%s, PatternsDir=%s, SchedulesFile=%s, BLECommandRateLimitRate=%.1f/s, BLECommandRateLimitBurst=%d",
        cfg.ServerPort, cfg.StaticFilesDir, cfg.PatternsDir, cfg.SchedulesFile, cfg.BLECommandRateLimitRate, cfg.BLECommandRateLimitBurst)

    a, err := agent.NewAgent(cfg)
    if err != nil {
        log.Fatalf("Failed to create agent: %v", err)
    }

    go a.Run()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Println("Shutting down agent...")
    a.Shutdown()
    log.Println("Agent shut down gracefully.")
}
