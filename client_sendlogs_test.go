package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestSendLogsNoDataPath tests the path where buffer has no data
func TestSendLogsNoDataPath(t *testing.T) {
	// Create a mock server that will accept our logs
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()
	
	// Create a mock buffer with NO data
	mockBuffer := &MockBuffer{}
	
	// Create a client
	client := &HTTPClient{
		config: &Config{
			Host:      "example.com",
			Port:      443,
			AuthToken: "test-token",
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
	mockBuffer := &MockBuffer{}
	
	// Create a client
	client := &HTTPClient{
		config: &Config{
			Host:      "example.com",
			Port:      443,
			AuthToken: "test-token",
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
	mockBuffer := &MockBuffer{}
	mockBuffer.Write([]byte("test log message\n"))
	
	// Create a client
	client := &HTTPClient{
		config: &Config{
			Host:      "example.com",
			Port:      443,
			AuthToken: "test-token",
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