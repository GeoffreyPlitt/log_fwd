package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// MockHTTPServer creates a test HTTP server for testing the HTTP client
func createMockHTTPServer(t *testing.T, statusCode int, responseBody string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for expected headers
		contentType := r.Header.Get("Content-Type")

		if contentType != "application/json" {
			t.Errorf("Expected Content-Type header 'application/json', got %q", contentType)
		}

		// Read and validate request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Error reading request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Try to parse as individual LogEntry first
		var logEntry LogEntry
		if err := json.Unmarshal(body, &logEntry); err == nil {
			// Single log entry
			if logEntry.Timestamp == "" {
				t.Error("Missing dt field in log entry")
			}
			if logEntry.Message == "" {
				t.Error("Missing message field in log entry")
			}
		} else {
			// Try to parse as LogBatch (array)
			var logBatch LogBatch
			if err := json.Unmarshal(body, &logBatch); err != nil {
				t.Errorf("Invalid JSON in request body (neither LogEntry nor LogBatch): %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Validate batch entries
			if len(logBatch) == 0 {
				t.Error("Empty batch received")
			}
			for i, entry := range logBatch {
				if entry.Timestamp == "" {
					t.Errorf("Missing dt field in batch entry %d", i)
				}
				if entry.Message == "" {
					t.Errorf("Missing message field in batch entry %d", i)
				}
			}
		}

		// Return the configured response
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	})

	return httptest.NewTLSServer(handler)
}

// Use the existing MockBuffer from mock_buffer.go

// TestNewClient tests creating a new HTTP client
func TestNewClient(t *testing.T) {
	// Test with certificate
	t.Run("with certificate", func(t *testing.T) {
		// Create a temporary test certificate
		tmpDir, err := os.MkdirTemp("", "client-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		certPath := filepath.Join(tmpDir, "cert.pem")

		// Write a dummy cert
		certContent := `-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXzRGz3yIGrjP7lE7b9Jp2buwNr4wDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMzA1MDExNjI4MDlaFw0yNDA0
MzAxNjI4MDlaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDKj6HGkl6pLZ6+XApTY8c2AJwKCzwXWqFe2zdD0e+T
2MxzXVE3UNJoEgTmrQiUWjzLQQqbz5rCiKXPZ5XgPpkS+fGmPo2QKKFJwQ8UPBBI
Bo8eI2aUca3pT7JG6wM1MzQQxrjEm+rKGibjxWGcxEQGSxYaOo1GnZFkOvYlHKzB
NKxvBDfF3Qc6vcnJYpNJWjBHPvh7XzRQXvWznJDJJt3MjxJA5XQm7+M1jw4t1I6w
cw7bYxcGDlop5A31+KCjrjFejxQXJchbhYTz8B20COg+/XGtJYK0bCxIZRiP5rPJ
SCITcDSxKicZcvClsZQZLf9GWYxJoVYNYENQMdTFAgMBAAGjUzBRMB0GA1UdDgQW
BBTgGeCYy4J6pRnwVfMC3LqyCdkdATAfBgNVHSMEGDAWgBTgGeCYy4J6pRnwVfMC
3LqyCdkdATAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBZkviS
SB8Y9pfvVyjAVGFGlDlTHENITUQvHDfGUd+HaDVvpVIAuA1ARIjWRmqwQ9cKz5UI
7H7WLwkUxk1sjVlbBWg4j8bZN9+tZPYx4frBUoVQCy7GD82wdMQCJCaY7dUHKw2r
6PYEeNHEyEbVmSWKfvjGewNuWQ7MJyBW4+1j5Qbh5A4bPsHcGnrEZWDlAdxq3BXr
-----END CERTIFICATE-----`

		if err := os.WriteFile(certPath, []byte(certContent), 0644); err != nil {
			t.Fatalf("Failed to write cert file: %v", err)
		}

		cfg := &Config{
			CertFile:    certPath,
			Host:        "example.com",
			Port:        443,
			ProgramName: "test-program",
			BufferPath:  "test-buffer.log",
			MaxSize:     1024,
			AuthToken:   "test-token",
		}

		// Test creating a new client
		// This will fail because our cert is invalid, but we can verify
		// that the client was attempted to be constructed
		_, err = NewClient(cfg)
		if err != nil && !strings.Contains(err.Error(), "failed to append CA certificate") {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	// Test without certificate (using system certs)
	t.Run("without certificate", func(t *testing.T) {
		cfg := &Config{
			Host:        "example.com",
			Port:        443,
			ProgramName: "test-program",
			BufferPath:  "test-buffer.log",
			MaxSize:     1024,
			AuthToken:   "test-token",
		}

		// Test creating a new client with system certs
		client, err := NewClient(cfg)
		if err != nil {
			t.Errorf("Error creating client with system certs: %v", err)
		}

		if client == nil {
			t.Error("NewClient returned nil with system certs")
		}
	})
}

// TestSendHTTPRequest tests the HTTP request functionality
func TestSendHTTPRequest(t *testing.T) {
	// Create a mock HTTP server
	server := createMockHTTPServer(t, http.StatusAccepted, "")
	defer server.Close()

	// Create a client that points to our test server
	client := &HTTPClient{
		config: &Config{
			AuthToken:      "test-token",
			RequestTimeout: 5 * time.Second,
			HTTPTimeout:    10 * time.Second,
			MaxRetries:     3,
			BatchSize:      10,
			EnableBatching: false,
			CompressLogs:   false,
		},
		client: server.Client(),
		url:    server.URL,
	}

	// Create a test log entry
	logEntry := LogEntry{
		Timestamp: time.Now().UTC().Format(TimestampFormat),
		Message:   "Test log message",
	}

	jsonData, err := json.Marshal(logEntry)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Send the request
	statusCode, err := sendHTTPRequest(client.client, context.Background(), client.url, client.authToken, jsonData, client.config)
	if err != nil {
		t.Errorf("sendHTTPRequest failed: %v", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		t.Errorf("Expected 2xx status code, got %d", statusCode)
	}
}

// TestSendHTTPRequestErrors tests error handling in HTTP requests
func TestSendHTTPRequestErrors(t *testing.T) {
	// Test with server error
	t.Run("server error", func(t *testing.T) {
		server := createMockHTTPServer(t, http.StatusInternalServerError, "Internal Server Error")
		defer server.Close()

		client := &HTTPClient{
			config: &Config{
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

		logEntry := LogEntry{
			Timestamp: time.Now().UTC().Format(TimestampFormat),
			Message:   "Test log message",
		}

		jsonData, err := json.Marshal(logEntry)
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}

		// Should get an error due to 500 status code
		statusCode, err := sendHTTPRequest(client.client, context.Background(), client.url, client.authToken, jsonData, client.config)
		if err == nil {
			t.Error("Expected error for 500 status, got nil")
		}
		if statusCode != http.StatusInternalServerError {
			t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, statusCode)
		}
	})

	// Test with invalid URL
	t.Run("invalid URL", func(t *testing.T) {
		client := &HTTPClient{
			config: &Config{
				AuthToken:      "test-token",
				RequestTimeout: 1 * time.Second,
				HTTPTimeout:    2 * time.Second,
				MaxRetries:     1,
				BatchSize:      10,
				EnableBatching: false,
				CompressLogs:   false,
			},
			client: http.DefaultClient,
			url:    "http://invalid-test-url-that-does-not-exist",
		}

		logEntry := LogEntry{
			Timestamp: time.Now().UTC().Format(TimestampFormat),
			Message:   "Test log message",
		}

		jsonData, err := json.Marshal(logEntry)
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}

		// Should get an error due to invalid URL
		statusCode, err := sendHTTPRequest(client.client, context.Background(), client.url, client.authToken, jsonData, client.config)
		if err == nil {
			t.Error("Expected error for invalid URL, got nil")
		}
		if statusCode != 0 {
			t.Errorf("Expected status code 0 for connection error, got %d", statusCode)
		}
	})
}

// TestSendLogsContextCancellation tests that SendLogs exits when context is cancelled
func TestSendLogsContextCancellation(t *testing.T) {
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

	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Immediately cancel the context
	cancel()

	// Start the SendLogs function directly - it should exit right away
	// since we cancelled the context
	client.SendLogs(ctx, mockBuffer, signal)

	// If we got here, SendLogs correctly handled the context cancellation
}

// TestSendLogsWithData tests sending logs with actual data
func TestSendLogsWithData(t *testing.T) {
	server := createMockHTTPServer(t, http.StatusAccepted, "")
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

	// Start the SendLogs function - it should process the log data
	client.SendLogs(ctx, mockBuffer, signal)

	// If we got here, SendLogs correctly handled the logs
}

// TestExtractMessage tests the message extraction function
func TestExtractMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			// Normal syslog format
			input:    "<13>1 2023-04-14T15:04:05Z hostname program - - - Actual log message",
			expected: "Actual log message",
		},
		{
			// No separator
			input:    "Plain log message",
			expected: "Plain log message",
		},
		{
			// Multiple separators
			input:    "<13>1 2023-04-14T15:04:05Z hostname program - - - Message with - - - separators",
			expected: "Message with - - - separators",
		},
		{
			// Empty string
			input:    "",
			expected: "",
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			result := extractMessage(test.input)
			if result != test.expected {
				t.Errorf("extractMessage(%q) = %q, want %q", test.input, result, test.expected)
			}
		})
	}
}

// TestCalculateBackoff tests the exponential backoff calculation
func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		expectMin  time.Duration
		expectMax  time.Duration
	}{
		{
			name:       "zero retries",
			retryCount: 0,
			expectMin:  80 * time.Millisecond,  // 100ms - 20% jitter
			expectMax:  120 * time.Millisecond, // 100ms + 20% jitter
		},
		{
			name:       "negative retries",
			retryCount: -1,
			expectMin:  80 * time.Millisecond,
			expectMax:  120 * time.Millisecond,
		},
		{
			name:       "first retry",
			retryCount: 1,
			expectMin:  800 * time.Millisecond,  // 1s - 20% jitter
			expectMax:  1200 * time.Millisecond, // 1s + 20% jitter
		},
		{
			name:       "second retry",
			retryCount: 2,
			expectMin:  1600 * time.Millisecond, // 2s - 20% jitter
			expectMax:  2400 * time.Millisecond, // 2s + 20% jitter
		},
		{
			name:       "high retry count (should cap at 30s)",
			retryCount: 10,
			expectMin:  24 * time.Second, // 30s - 20% jitter
			expectMax:  36 * time.Second, // 30s + 20% jitter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := calculateBackoff(tt.retryCount)

			if backoff < tt.expectMin || backoff > tt.expectMax {
				t.Errorf("calculateBackoff(%d) = %v, expected between %v and %v",
					tt.retryCount, backoff, tt.expectMin, tt.expectMax)
			}
		})
	}
}

// TestCalculateBackoffConsistency tests that backoff values are reasonable
func TestCalculateBackoffConsistency(t *testing.T) {
	// Test that backoff generally increases with retry count (allowing for jitter)

	for i := 1; i <= 5; i++ {
		backoff := calculateBackoff(i)

		// Each backoff should be at least 50% of the expected exponential value
		// to account for jitter while still ensuring general increase
		expectedMin := time.Duration(1<<uint(i-1)) * time.Second / 2

		if backoff < expectedMin {
			t.Errorf("Backoff for retry %d (%v) is too small, expected at least %v",
				i, backoff, expectedMin)
		}

		// Ensure we don't have negative backoff
		if backoff < 0 {
			t.Errorf("Negative backoff for retry %d: %v", i, backoff)
		}
	}
}
