// Package main is the entry point for the BLEDOM Controller Agent.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"bledom-controller/internal/agent"
	"bledom-controller/internal/config"
)

// These variables are populated during the build process using -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	log.Printf("[Main] Starting BLEDOM Controller (Version: %s, Commit: %s, Built: %s)", version, commit, date)

	configPath := "./config.json"
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("[Main] FATAL: Failed to load configuration: %v", err)
	}

	log.Printf("[Main] Configuration loaded successfully (Port=%s, WebDir=%s)",
		cfg.Server.Port,
		cfg.Server.WebFilesDir,
	)

	a, err := agent.NewAgent(cfg)
	if err != nil {
		log.Fatalf("[Main] FATAL: Failed to initialize agent: %v", err)
	}

	// Start the agent orchestration in a separate goroutine
	go a.Run()

	// Wait for termination signals for a graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Main] Shutdown signal received, stopping agent...")
	a.Shutdown()
	log.Println("[Main] Graceful shutdown complete.")
}
