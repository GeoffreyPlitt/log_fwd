package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSendLogsContextCancellation tests that SendLogs exits when context is cancelled
func TestSendLogsContextCancellation(t *testing.T) {
	// This test only verifies the context cancellation path in SendLogs
	
	// Create a mock buffer with NO data
	mockBuffer := NewMockBuffer()
	
	// Create a client with nil dialer to avoid TLS issues
	client := &PapertrailClient{
		config: &Config{
			Host: "example.com",
			Port: 12345,
		},
		addr: "example.com:12345",
	}
	
	// Create a signal channel
	signal := make(chan struct{}, 1)
	
	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	
	// Immediately cancel the context
	cancel()
	
	// Start the SendLogs function directly - it should exit right away
	// since we cancelled the context
	client.SendLogs(ctx, mockBuffer, signal)
	
	// If we got here, SendLogs correctly handled the context cancellation
}

// TestSendLogsSignalPath tests the signal path in SendLogs
func TestSendLogsSignalPath(t *testing.T) {
	// Create a mock buffer with NO data initially
	mockBuffer := NewMockBuffer()
	
	// Create a mock dialer that returns error on dial
	mockDialer := &MockTLSDialer{
		DialError: errors.New("test error"),
	}
	
	// Create a client
	client := &PapertrailClient{
		config: &Config{
			Host: "example.com",
			Port: 12345,
		},
		dialer: mockDialer,
		addr:   "example.com:12345",
	}
	
	// Create a signal channel
	signal := make(chan struct{}, 1)
	
	// Create a context with cancel and timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	
	// Send a signal immediately
	signal <- struct{}{}
	
	// Start the SendLogs function - it will check for data, find none, and wait for
	// another signal or timeout
	client.SendLogs(ctx, mockBuffer, signal)
	
	// If we got here, SendLogs correctly handled the signal path
}

// TestSendLogsBufferHasData tests the path where buffer has data
func TestSendLogsBufferHasData(t *testing.T) {
	// Create a mock buffer WITH data
	mockBuffer := NewMockBuffer()
	mockBuffer.Write([]byte("test log message"))
	
	// Create a mock dialer that returns error on dial
	mockDialer := &MockTLSDialer{
		DialError: errors.New("test error"),
	}
	
	// Create a client
	client := &PapertrailClient{
		config: &Config{
			Host: "example.com",
			Port: 12345,
		},
		dialer: mockDialer,
		addr:   "example.com:12345",
	}
	
	// Create a signal channel
	signal := make(chan struct{}, 1)
	
	// Create a context with cancel and timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	
	// Start the SendLogs function - it will check for data, find some,
	// attempt to connect (which will fail), and eventually time out
	client.SendLogs(ctx, mockBuffer, signal)
	
	// If we got here, SendLogs correctly handled the timeout
}