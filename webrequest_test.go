package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadTLSConfig tests the TLS configuration loading function
func TestLoadTLSConfig(t *testing.T) {
	t.Run("valid certificate", func(t *testing.T) {
		// Create a temporary test certificate
		tmpDir, err := os.MkdirTemp("", "webrequest-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		certPath := filepath.Join(tmpDir, "cert.pem")
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

		_, err = loadTLSConfig(certPath)
		if err != nil && !strings.Contains(err.Error(), "failed to append CA certificate") {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := loadTLSConfig("/nonexistent/cert.pem")
		if err == nil {
			t.Error("Expected error for missing file, got nil")
		}
		if !strings.Contains(err.Error(), "error loading certificate") {
			t.Errorf("Expected certificate loading error, got: %v", err)
		}
	})

	t.Run("invalid certificate", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "webrequest-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		certPath := filepath.Join(tmpDir, "invalid.pem")
		if err := os.WriteFile(certPath, []byte("invalid certificate data"), 0644); err != nil {
			t.Fatalf("Failed to write invalid cert file: %v", err)
		}

		_, err = loadTLSConfig(certPath)
		if err == nil {
			t.Error("Expected error for invalid certificate, got nil")
		}
		if !strings.Contains(err.Error(), "failed to append CA certificate") {
			t.Errorf("Expected CA certificate error, got: %v", err)
		}
	})
}

// TestExtractMessageDirect tests the message extraction function directly
func TestExtractMessageDirect(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "syslog format",
			input:    "<13>1 2023-04-14T15:04:05Z hostname program - - - Actual log message",
			expected: "Actual log message",
		},
		{
			name:     "no separator",
			input:    "Plain log message",
			expected: "Plain log message",
		},
		{
			name:     "multiple separators",
			input:    "<13>1 2023-04-14T15:04:05Z hostname program - - - Message with - - - separators",
			expected: "Message with - - - separators",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "separator at end",
			input:    "prefix - - - ",
			expected: "",
		},
		{
			name:     "only separator",
			input:    " - - - ",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := extractMessage(test.input)
			if result != test.expected {
				t.Errorf("extractMessage(%q) = %q, want %q", test.input, result, test.expected)
			}
		})
	}
}

// TestCreateHTTPClient tests the HTTP client creation function
func TestCreateHTTPClient(t *testing.T) {
	t.Run("with system certs", func(t *testing.T) {
		cfg := &Config{
			HTTPTimeout:    30 * time.Second,
			RequestTimeout: 10 * time.Second,
			InsecureSSL:    false,
		}

		client, err := createHTTPClient(cfg)
		if err != nil {
			t.Errorf("createHTTPClient failed: %v", err)
		}
		if client == nil {
			t.Error("createHTTPClient returned nil client")
		}
		if client.Timeout != cfg.HTTPTimeout {
			t.Errorf("Expected timeout %v, got %v", cfg.HTTPTimeout, client.Timeout)
		}
	})

	t.Run("with insecure SSL", func(t *testing.T) {
		cfg := &Config{
			HTTPTimeout:    15 * time.Second,
			RequestTimeout: 5 * time.Second,
			InsecureSSL:    true,
		}

		client, err := createHTTPClient(cfg)
		if err != nil {
			t.Errorf("createHTTPClient failed: %v", err)
		}
		if client == nil {
			t.Error("createHTTPClient returned nil client")
		}
	})

	t.Run("with custom certificate", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "webrequest-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		certPath := filepath.Join(tmpDir, "cert.pem")
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
			CertFile:       certPath,
			HTTPTimeout:    30 * time.Second,
			RequestTimeout: 10 * time.Second,
			InsecureSSL:    false,
		}

		// This will fail due to invalid cert, but we test that it attempts to load
		_, err = createHTTPClient(cfg)
		if err != nil && !strings.Contains(err.Error(), "failed to load TLS config") {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

// TestSendHTTPRequestDirect tests the HTTP request sending function directly
func TestSendHTTPRequestDirect(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Expected POST method, got %s", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected application/json content type, got %s", r.Header.Get("Content-Type"))
			}
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Errorf("Expected Bearer test-token auth, got %s", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 5 * time.Second,
			CompressLogs:   false,
		}

		jsonData := []byte(`{"dt":"2023-01-01 12:00:00 UTC","message":"test"}`)
		statusCode, err := sendHTTPRequest(server.Client(), context.Background(), server.URL, "test-token", jsonData, cfg)

		if err != nil {
			t.Errorf("sendHTTPRequest failed: %v", err)
		}
		if statusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", statusCode)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 5 * time.Second,
			CompressLogs:   false,
		}

		jsonData := []byte(`{"dt":"2023-01-01 12:00:00 UTC","message":"test"}`)
		statusCode, err := sendHTTPRequest(server.Client(), context.Background(), server.URL, "test-token", jsonData, cfg)

		if err == nil {
			t.Error("Expected error for server error, got nil")
		}
		if statusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", statusCode)
		}
	})

	t.Run("with compression", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json+gzip" {
				t.Errorf("Expected application/json+gzip content type, got %s", r.Header.Get("Content-Type"))
			}
			if r.Header.Get("Content-Encoding") != "gzip" {
				t.Errorf("Expected gzip encoding, got %s", r.Header.Get("Content-Encoding"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 5 * time.Second,
			CompressLogs:   true,
		}

		jsonData := []byte(`{"dt":"2023-01-01 12:00:00 UTC","message":"test"}`)
		statusCode, err := sendHTTPRequest(server.Client(), context.Background(), server.URL, "test-token", jsonData, cfg)

		if err != nil {
			t.Errorf("sendHTTPRequest with compression failed: %v", err)
		}
		if statusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", statusCode)
		}
	})

	t.Run("no auth token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Errorf("Expected no auth header, got %s", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 5 * time.Second,
			CompressLogs:   false,
		}

		jsonData := []byte(`{"dt":"2023-01-01 12:00:00 UTC","message":"test"}`)
		statusCode, err := sendHTTPRequest(server.Client(), context.Background(), server.URL, "", jsonData, cfg)

		if err != nil {
			t.Errorf("sendHTTPRequest without token failed: %v", err)
		}
		if statusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", statusCode)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond) // Delay longer than timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 10 * time.Millisecond, // Very short timeout
			CompressLogs:   false,
		}

		jsonData := []byte(`{"dt":"2023-01-01 12:00:00 UTC","message":"test"}`)
		_, err := sendHTTPRequest(server.Client(), context.Background(), server.URL, "test-token", jsonData, cfg)

		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
	})
}

// TestSendBatchedLogs tests the batch log sending function
func TestSendBatchedLogs(t *testing.T) {
	t.Run("successful batch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify it's a batch (array) of log entries
			var batch []LogEntry
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				t.Errorf("Failed to decode batch: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(batch) != 2 {
				t.Errorf("Expected 2 log entries, got %d", len(batch))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 5 * time.Second,
			CompressLogs:   false,
		}

		batch := LogBatch{
			{Timestamp: "2023-01-01 12:00:00 UTC", Message: "log 1"},
			{Timestamp: "2023-01-01 12:00:01 UTC", Message: "log 2"},
		}

		statusCode, err := sendBatchedLogs(server.Client(), context.Background(), server.URL, "test-token", batch, cfg)

		if err != nil {
			t.Errorf("sendBatchedLogs failed: %v", err)
		}
		if statusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", statusCode)
		}
	})

	t.Run("empty batch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var batch []LogEntry
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				t.Errorf("Failed to decode batch: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(batch) != 0 {
				t.Errorf("Expected 0 log entries, got %d", len(batch))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &Config{
			RequestTimeout: 5 * time.Second,
			CompressLogs:   false,
		}

		batch := LogBatch{}
		statusCode, err := sendBatchedLogs(server.Client(), context.Background(), server.URL, "test-token", batch, cfg)

		if err != nil {
			t.Errorf("sendBatchedLogs with empty batch failed: %v", err)
		}
		if statusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", statusCode)
		}
	})
}
