package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Parse command line arguments
	cfg := ParseFlags()

	// Create context for clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	setupSignalHandling(cancel)

	// Initialize the buffer
	buffer, err := NewBuffer(cfg.BufferPath, cfg.MaxSize)
	if err != nil {
		log.Fatalf("Failed to create buffer: %v", err)
	}
	defer buffer.Close()

	// Get hostname for log formatting
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}

	// Create a channel to signal new logs are available
	newLogs := make(chan struct{}, 1)

	// Initialize Papertrail client
	client, err := NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create Papertrail client: %v", err)
	}

	// Start sender goroutine
	go client.SendLogs(ctx, buffer, newLogs)

	// Process stdin and write to buffer
	ProcessInput(ctx, buffer, hostname, cfg.ProgramName, newLogs)
}

// setupSignalHandling sets up handlers for OS signals
func setupSignalHandling(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
	}()
}
