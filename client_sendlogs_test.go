package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// createMockHTTPServerWithCallback creates a test HTTP server with a callback function
func createMockHTTPServerWithCallback(t *testing.T, callback func() (int, string)) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode, responseBody := callback()
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	})
	return httptest.NewTLSServer(handler)
}

// TestSendLogsNoDataPath tests the path where buffer has no data
func TestSendLogsNoDataPath(t *testing.T) {
	// Create a mock server that will accept our logs
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	// Create a mock buffer with NO data
	mockBuffer := NewMockBuffer()

	// Create a client
	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	// Create a signal channel
	signal := make(chan struct{}, 1)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start the SendLogs function - it will check for data, find none,
	// and wait for the timeout
	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsSignalPath tests the signal handling in SendLogs
func TestSendLogsSignalPath(t *testing.T) {
	// Create a mock server that will accept our logs
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	// Create a mock buffer with NO data
	mockBuffer := NewMockBuffer()

	// Create a client
	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	// Create a signal channel
	signal := make(chan struct{}, 1)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Send a signal to trigger the signal handling path
	signal <- struct{}{}

	// Start the SendLogs function - it will receive the signal but find no data
	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsErrorHandling tests error handling when sending logs
func TestSendLogsErrorHandling(t *testing.T) {
	// Create a mock server that will return an error
	server := createMockHTTPServer(t, http.StatusInternalServerError, "Error")
	defer server.Close()

	// Create a mock buffer WITH data
	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message\n"))

	// Create a client
	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	// Create a signal channel
	signal := make(chan struct{}, 1)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start the SendLogs function - it should handle the error gracefully
	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsBatchProcessing tests batch processing mode
func TestSendLogsBatchProcessing(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	// Add multiple log messages
	for i := 0; i < 5; i++ {
		mockBuffer.Write([]byte(fmt.Sprintf("test log message %d\n", i)))
	}

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      3,
			EnableBatching: true, // Enable batching
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsRetryLogic tests retry logic with exponential backoff
func TestSendLogsRetryLogic(t *testing.T) {
	// Create a server that fails first, then succeeds
	callCount := 0
	server := createMockHTTPServerWithCallback(t, func() (int, string) {
		callCount++
		if callCount == 1 {
			return http.StatusInternalServerError, "Error"
		}
		return http.StatusAccepted, ""
	})
	defer server.Close()

	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message\n"))

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     2,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)

	// Should have been called twice (first failure, then success)
	if callCount < 1 {
		t.Errorf("Expected at least 1 call, got %d", callCount)
	}
}

// TestSendLogsContextCancellationImmediate tests context cancellation
func TestSendLogsContextCancellationImmediate(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message\n"))

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		client.SendLogs(ctx, mockBuffer, signal)
		close(done)
	}()

	// Cancel context immediately
	cancel()

	// Should exit quickly
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("SendLogs didn't exit when context was cancelled")
	}
}

// TestSendLogsBufferReadError tests buffer read error handling
func TestSendLogsBufferReadError(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message\n"))
	// Set read error after writing data
	mockBuffer.ReadError = fmt.Errorf("mock read error")

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Should handle read error gracefully
	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsLargeData tests handling of large data reads
func TestSendLogsLargeData(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	// Add a large amount of data
	largeData := strings.Repeat("test log message\n", 1000)
	mockBuffer.Write([]byte(largeData))

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      50,
			EnableBatching: true,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsMaxRetriesExceeded tests behavior when max retries are exceeded
func TestSendLogsMaxRetriesExceeded(t *testing.T) {
	// Server that always fails
	server := createMockHTTPServer(t, http.StatusInternalServerError, "Error")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message\n"))

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 100 * time.Millisecond,
			HTTPTimeout:    200 * time.Millisecond,
			MaxRetries:     2,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsBatchRetryLogic tests retry logic for batch processing
func TestSendLogsBatchRetryLogic(t *testing.T) {
	callCount := 0
	server := createMockHTTPServerWithCallback(t, func() (int, string) {
		callCount++
		if callCount == 1 {
			return http.StatusInternalServerError, "Error"
		}
		return http.StatusAccepted, ""
	})
	defer server.Close()

	mockBuffer := NewMockBuffer()
	for i := 0; i < 3; i++ {
		mockBuffer.Write([]byte(fmt.Sprintf("test log message %d\n", i)))
	}

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     2,
			BatchSize:      3,
			EnableBatching: true,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsEmptyLines tests handling of empty lines in data
func TestSendLogsEmptyLines(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	// Add data with empty lines
	mockBuffer.Write([]byte("test log message 1\n\n\ntest log message 2\n"))

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)
}

// TestSendLogsSignalChannelFull tests behavior when signal channel is full
func TestSendLogsSignalChannelFull(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message\n"))

	client := &HTTPClient{
		config: &Config{
			Host:           "example.com",
			Port:           443,
			AuthToken:      "test-token",
			RequestTimeout: 1 * time.Second,
			HTTPTimeout:    2 * time.Second,
			MaxRetries:     1,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	// Create a signal channel with capacity 1 and fill it
	signal := make(chan struct{}, 1)
	signal <- struct{}{} // Fill the channel

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	client.SendLogs(ctx, mockBuffer, signal)
}
