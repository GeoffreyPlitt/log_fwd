package main

import (
	"bytes"
	"context"
	"os"
	"strings"
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

func TestDebugf(t *testing.T) {
	// Save original stderr
	originalStderr := os.Stderr
	defer func() {
		os.Stderr = originalStderr
	}()

	// Create a pipe to capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Test with verbose mode disabled (default)
	isVerbose = false
	debugf("test message %s", "arg")

	// Close write end and read from pipe
	w.Close()
	output := make([]byte, 1024)
	n, _ := r.Read(output)

	// Should be empty when verbose is false
	if n > 0 {
		t.Errorf("Expected no output when verbose=false, got: %s", string(output[:n]))
	}

	// Test with verbose mode enabled
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create second pipe: %v", err)
	}
	os.Stderr = w2

	isVerbose = true
	debugf("test message %s %d", "hello", 42)

	w2.Close()
	output2 := make([]byte, 1024)
	n2, _ := r2.Read(output2)

	if n2 == 0 {
		t.Error("Expected output when verbose=true, got none")
	}

	outputStr := string(output2[:n2])
	if !strings.Contains(outputStr, "[DEBUG]") {
		t.Errorf("Expected [DEBUG] prefix in output, got: %s", outputStr)
	}
	if !strings.Contains(outputStr, "test message hello 42") {
		t.Errorf("Expected formatted message in output, got: %s", outputStr)
	}

	// Reset verbose mode
	isVerbose = false
}

func TestLogData(t *testing.T) {
	// Save original stderr and verbose state
	originalStderr := os.Stderr
	originalVerbose := isVerbose
	defer func() {
		os.Stderr = originalStderr
		isVerbose = originalVerbose
	}()

	// Test with verbose mode disabled
	isVerbose = false

	// Create a pipe to capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	logData([]byte("test data"))

	w.Close()
	output := make([]byte, 1024)
	n, _ := r.Read(output)

	// Should be empty when verbose is false
	if n > 0 {
		t.Errorf("Expected no output when verbose=false, got: %s", string(output[:n]))
	}

	// Test with verbose mode enabled
	isVerbose = true

	tests := []struct {
		name     string
		input    []byte
		expected []string
	}{
		{
			name:     "simple text",
			input:    []byte("hello world"),
			expected: []string{"hello world"},
		},
		{
			name:     "text with newline",
			input:    []byte("line1\nline2"),
			expected: []string{"line1\\nline2"},
		},
		{
			name:     "text with carriage return",
			input:    []byte("line1\rline2"),
			expected: []string{"line1\\rline2"},
		},
		{
			name:     "text with tab",
			input:    []byte("col1\tcol2"),
			expected: []string{"col1\\tcol2"},
		},
		{
			name:     "binary data",
			input:    []byte{0x00, 0x01, 0x02, 0xFF},
			expected: []string{"\\x00\\x01\\x02\\xff"},
		},
		{
			name:     "mixed printable and non-printable",
			input:    []byte("hello\x00world\xFF"),
			expected: []string{"hello\\x00world\\xff"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a pipe to capture stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("Failed to create pipe: %v", err)
			}
			os.Stderr = w

			logData(tt.input)

			w.Close()
			output := make([]byte, 2048)
			n, _ := r.Read(output)

			if n == 0 {
				t.Error("Expected output when verbose=true, got none")
				return
			}

			outputStr := string(output[:n])
			for _, expected := range tt.expected {
				if !strings.Contains(outputStr, expected) {
					t.Errorf("Expected %q in output, got: %s", expected, outputStr)
				}
			}
		})
	}
}

func TestLogDataLargeData(t *testing.T) {
	// Save original stderr and verbose state
	originalStderr := os.Stderr
	originalVerbose := isVerbose
	defer func() {
		os.Stderr = originalStderr
		isVerbose = originalVerbose
	}()

	isVerbose = true

	// Create a pipe to capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	// Create data larger than 1024 bytes
	largeData := bytes.Repeat([]byte("A"), 2000)
	logData(largeData)

	w.Close()
	output := make([]byte, 4096)
	n, _ := r.Read(output)

	if n == 0 {
		t.Error("Expected output for large data, got none")
		return
	}

	outputStr := string(output[:n])

	// Should contain message about showing first bytes
	if !strings.Contains(outputStr, "Showing first 1024 bytes of 2000 total") {
		t.Errorf("Expected truncation message in output, got: %s", outputStr)
	}
}

func TestLogDataEmptyData(t *testing.T) {
	// Save original stderr and verbose state
	originalStderr := os.Stderr
	originalVerbose := isVerbose
	defer func() {
		os.Stderr = originalStderr
		isVerbose = originalVerbose
	}()

	isVerbose = true

	// Create a pipe to capture stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	logData([]byte{})

	w.Close()
	output := make([]byte, 1024)
	n, _ := r.Read(output)

	if n == 0 {
		t.Error("Expected output for empty data, got none")
		return
	}

	outputStr := string(output[:n])

	// Should contain log content message
	if !strings.Contains(outputStr, "Log content:") {
		t.Errorf("Expected 'Log content:' in output, got: %s", outputStr)
	}
}
