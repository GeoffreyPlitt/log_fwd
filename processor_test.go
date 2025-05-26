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

	// Start processing in a goroutine with quiet mode on to avoid test output
	done := make(chan struct{})
	testConfig := &Config{Quiet: true}
	go func() {
		ProcessInput(ctx, buffer, testHostname, testProgram, signal, testConfig)
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
}

func TestProcessInputContextCancellation(t *testing.T) {
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

	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	ctx, cancel := context.WithCancel(context.Background())
	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	testConfig := &Config{Quiet: true}

	go func() {
		ProcessInput(ctx, buffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	// Cancel context immediately
	cancel()
	w.Close()

	// Should exit quickly
	select {
	case <-done:
		// Success - processor should exit when context is cancelled
	case <-time.After(2 * time.Second):
		t.Fatal("Processor didn't exit when context was cancelled")
	}
}

func TestProcessInputLargeLines(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "processor-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buffer, err := NewBuffer(bufferPath, 1024*1024) // 1MB buffer
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buffer.Close()

	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	testConfig := &Config{Quiet: true}

	go func() {
		ProcessInput(ctx, buffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	// Create a large line (200KB)
	largeLine := strings.Repeat("A", 200*1024)
	_, err = fmt.Fprintln(w, largeLine)
	if err != nil {
		t.Fatalf("Failed to write large line: %v", err)
	}

	// Wait for signal
	select {
	case <-signal:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for signal with large line")
	}

	w.Close()
	cancel()
	<-done

	// Verify the large line was processed
	data, err := buffer.Read(1024 * 1024)
	if err != nil {
		t.Fatalf("Failed to read from buffer: %v", err)
	}

	if !bytes.Contains(data, []byte(largeLine[:100])) { // Check first 100 chars
		t.Error("Buffer doesn't contain the large line")
	}
}

func TestProcessInputQuietMode(t *testing.T) {
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

	originalStdin := os.Stdin
	originalStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r

	// Capture stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}
	os.Stdout = stdoutW

	defer func() {
		os.Stdin = originalStdin
		os.Stdout = originalStdout
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := make(chan struct{}, 1)
	done := make(chan struct{})

	// Test quiet mode
	testConfig := &Config{Quiet: true}
	go func() {
		ProcessInput(ctx, buffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	testInput := "test message for quiet mode"
	_, err = fmt.Fprintln(w, testInput)
	if err != nil {
		t.Fatalf("Failed to write to pipe: %v", err)
	}

	select {
	case <-signal:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for signal")
	}

	w.Close()
	stdoutW.Close()
	cancel()
	<-done

	// Read stdout - should be empty in quiet mode
	stdoutData := make([]byte, 1024)
	n, _ := stdoutR.Read(stdoutData)
	stdoutOutput := string(stdoutData[:n])

	if strings.Contains(stdoutOutput, testInput) {
		t.Error("Output appeared on stdout in quiet mode")
	}
}

func TestProcessInputNormalMode(t *testing.T) {
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

	originalStdin := os.Stdin
	originalStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r

	// Capture stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}
	os.Stdout = stdoutW

	defer func() {
		os.Stdin = originalStdin
		os.Stdout = originalStdout
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := make(chan struct{}, 1)
	done := make(chan struct{})

	// Test normal mode (not quiet)
	testConfig := &Config{Quiet: false}
	go func() {
		ProcessInput(ctx, buffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	testInput := "test message for normal mode"
	_, err = fmt.Fprintln(w, testInput)
	if err != nil {
		t.Fatalf("Failed to write to pipe: %v", err)
	}

	select {
	case <-signal:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for signal")
	}

	w.Close()
	stdoutW.Close()
	cancel()
	<-done

	// Read stdout - should contain the message in normal mode
	stdoutData := make([]byte, 1024)
	n, _ := stdoutR.Read(stdoutData)
	stdoutOutput := string(stdoutData[:n])

	if !strings.Contains(stdoutOutput, testInput) {
		t.Error("Output did not appear on stdout in normal mode")
	}
}

func TestProcessInputBufferFlush(t *testing.T) {
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

	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	testConfig := &Config{Quiet: true}

	// Write some data to buffer first
	buffer.Write([]byte("existing data\n"))

	go func() {
		ProcessInput(ctx, buffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	// Write a message and then close stdin to trigger flush
	_, err = fmt.Fprintln(w, "test message")
	if err != nil {
		t.Fatalf("Failed to write to pipe: %v", err)
	}

	// Wait for signal
	select {
	case <-signal:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for signal")
	}

	// Close stdin to trigger flush logic
	w.Close()

	// The processor should wait for buffer to flush
	// We'll simulate this by reading from buffer in another goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		// Read all data to simulate buffer being flushed
		for buffer.HasData() {
			buffer.Read(1024)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Wait for processor to finish
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Processor didn't finish buffer flush in time")
	}
}

func TestProcessInputWithMockBuffer(t *testing.T) {
	// Test with mock buffer that can simulate errors
	mockBuffer := NewMockBuffer()

	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	testConfig := &Config{Quiet: true}

	// Set mock buffer to return error on write
	mockBuffer.WriteError = fmt.Errorf("mock write error")

	go func() {
		ProcessInput(ctx, mockBuffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	// Write a message - this should trigger the write error
	_, err = fmt.Fprintln(w, "test message")
	if err != nil {
		t.Fatalf("Failed to write to pipe: %v", err)
	}

	// Even with write error, processor should continue
	w.Close()
	cancel()

	select {
	case <-done:
		// Success - processor should handle write errors gracefully
	case <-time.After(2 * time.Second):
		t.Fatal("Processor didn't handle write error gracefully")
	}
}

func TestProcessInputEmptyLines(t *testing.T) {
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

	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	testConfig := &Config{Quiet: true}

	go func() {
		ProcessInput(ctx, buffer, "test-host", "test-program", signal, testConfig)
		close(done)
	}()

	// Write empty lines and regular lines
	_, err = fmt.Fprintln(w, "")
	if err != nil {
		t.Fatalf("Failed to write empty line: %v", err)
	}
	_, err = fmt.Fprintln(w, "actual message")
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}
	_, err = fmt.Fprintln(w, "")
	if err != nil {
		t.Fatalf("Failed to write empty line: %v", err)
	}

	// Wait for signal
	select {
	case <-signal:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for signal")
	}

	w.Close()
	cancel()
	<-done

	// Verify buffer contains the actual message
	data, err := buffer.Read(1024)
	if err != nil {
		t.Fatalf("Failed to read from buffer: %v", err)
	}

	if !bytes.Contains(data, []byte("actual message")) {
		t.Error("Buffer doesn't contain the actual message")
	}
}
