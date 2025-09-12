package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"bledom-controller/internal/agent"
)

// These variables will be set by the build script
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Print the version information on startup
	log.Printf("Starting BLEDOM Controller Agent version: %s, commit: %s, built: %s", version, commit, date)

	a, err := agent.NewAgent()
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	go a.Run()

	// Wait for termination signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down agent...")
	a.Shutdown()
	log.Println("Agent shut down gracefully.")
}
