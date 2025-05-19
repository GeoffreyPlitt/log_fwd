package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessInput(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "processor-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buffer, err := NewBuffer(bufferPath, 1024)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buffer.Close()

	// Create test data
	testInput := "test log message"
	testHostname := "test-host"
	testProgram := "test-program"

	// We'll create a pipe for stdin
	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a signaling channel
	signal := make(chan struct{}, 1)

	// Start processing in a goroutine
	done := make(chan struct{})
	go func() {
		ProcessInput(ctx, buffer, testHostname, testProgram, signal)
		close(done)
	}()

	// Write a test message to stdin
	_, err = fmt.Fprintln(w, testInput)
	if err != nil {
		t.Fatalf("Failed to write to pipe: %v", err)
	}

	// Wait for signal or timeout
	select {
	case <-signal:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for signal")
	}

	// Close stdin to simulate EOF
	w.Close()

	// Cancel the context to stop processing
	cancel()

	// Wait for processor to finish
	<-done

	// Now read the buffer and verify the contents
	data, err := buffer.Read(1024)
	if err != nil {
		t.Fatalf("Failed to read from buffer: %v", err)
	}

	// The log message should contain our input and be properly formatted
	if !bytes.Contains(data, []byte(testInput)) {
		t.Errorf("Buffer doesn't contain our input %q: %s", testInput, data)
	}

	// Should contain the syslog priority and hostname/program
	if !strings.Contains(string(data), "<13>1 ") {
		t.Errorf("Missing syslog priority: %s", data)
	}
	
	if !bytes.Contains(data, []byte(testHostname)) {
		t.Errorf("Missing hostname: %s", data)
	}
	
	if !bytes.Contains(data, []byte(testProgram)) {
		t.Errorf("Missing program name: %s", data)
	}
}