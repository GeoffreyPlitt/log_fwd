package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
)

func main() {
	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v\nStacktrace: %s", r, debug.Stack())
			os.Exit(1)
		}
	}()

	// Parse command line arguments
	cfg := ParseFlags()
	
	// Additional validation is already done in ParseFlags, but this demonstrates
	// how we would handle validation errors if we needed to

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
	defer func() {
		if err := buffer.Close(); err != nil {
			log.Printf("Error closing buffer: %v", err)
		}
	}()

	// Get hostname for log formatting
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Could not determine hostname: %v", err)
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
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-c
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()
}