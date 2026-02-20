package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"bledom-controller/internal/agent"
	"bledom-controller/internal/config"
)

// These variables will be set by the build script
var (
	commit = "none"
	date   = "unknown"
)

func main() {
	log.Printf("Starting BLEDOM Controller Agent commit: %s, built: %s", commit, date)

	configPath := "./config.json"
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: Port=%s, StaticDir=%s, PatternsDir=%s, SchedulesFile=%s, BLE Rate=%.1f/s",
		cfg.Server.Port, 
		cfg.Server.StaticFilesDir, 
		cfg.PatternsDir, 
		cfg.SchedulesFile, 
		cfg.BLE.RateLimit,
	)

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
