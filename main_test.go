package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSetupSignalHandling tests that the signal handling works correctly
func TestSetupSignalHandling(t *testing.T) {
	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling with our cancel function
	setupSignalHandling(cancel)

	// Create a channel to track if cancel was called
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()

	// Send a SIGINT signal to our process
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}

	// Send the signal
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}

	// Wait for cancellation or timeout
	select {
	case <-done:
		// Success - context was cancelled
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for signal handler")
	}
}
