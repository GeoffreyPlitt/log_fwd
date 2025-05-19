package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTPClient handles sending logs to the HTTP API
type HTTPClient struct {
	config       *Config
	client       *http.Client
	url          string
	authToken    string
	insecureSSL  bool
}

// LogEntry represents a JSON log entry for the HTTP API
type LogEntry struct {
	Timestamp string `json:"dt"`
	Message   string `json:"message"`
}

// NewClient creates a new HTTP API client
func NewClient(cfg *Config) (*HTTPClient, error) {
	// Create HTTP transport with proper TLS config
	var transport *http.Transport
	
	if cfg.CertFile != "" {
		debugf("Using custom certificate: %s", cfg.CertFile)
		tlsConfig, err := loadTLSConfig(cfg.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
		transport = &http.Transport{TLSClientConfig: tlsConfig}
	} else {
		// Use system root certificates
		debugf("Using system root certificates")
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				InsecureSkipVerify: cfg.InsecureSSL,
			},
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	// Build URL from host and port
	url := fmt.Sprintf("https://%s", cfg.Host)
	if cfg.Port != 443 {
		url = fmt.Sprintf("https://%s:%d", cfg.Host, cfg.Port)
	}
	
	debugf("Configured HTTP API endpoint: %s", url)

	return &HTTPClient{
		config:      cfg,
		client:      client,
		url:         url,
		authToken:   cfg.AuthToken,
		insecureSSL: cfg.InsecureSSL,
	}, nil
}

// Buffer defines the interface for buffer types used with SendLogs
type Buffer interface {
	Read(maxBytes int64) ([]byte, error)
	HasData() bool
	GetSize() int64
}

// SendLogs reads from buffer and sends to the HTTP API
func (c *HTTPClient) SendLogs(ctx context.Context, buffer Buffer, signal chan struct{}) {
	debugf("SendLogs started for HTTP API endpoint %s", c.url)
	
	for {
		// Check if we should exit
		select {
		case <-ctx.Done():
			debugf("Context canceled, shutting down SendLogs")
			return
		default:
		}
		
		// Check for data in buffer
		if !buffer.HasData() {
			debugf("No data in buffer, waiting for new logs")
			// Wait for new logs signal or timeout
			select {
			case <-signal:
				debugf("Received signal that new logs are available")
				// Immediately continue to process new logs
			case <-time.After(5 * time.Second):
				debugf("Regular polling for new logs")
				// Regular poll
			case <-ctx.Done():
				debugf("Context canceled while waiting for new logs")
				return
			}
			continue
		}

		// Read log data from buffer
		debugf("Reading logs from buffer")
		data, err := buffer.Read(ReadChunkSize)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Error reading from buffer: %v\n", err)
			debugf("Error reading from buffer: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(data) > 0 {
			debugf("Processing %d bytes of log data", len(data))
			
			// Parse the syslog format and convert to JSON for HTTP API
			// Assuming each line is a separate log entry
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				
				// Log the actual content being sent
				logData([]byte(line))
				
				// Extract the actual message from the syslog format
				// Example: <13>1 2023-04-14T15:04:05Z hostname program - - - Actual log message
				message := extractMessage(line)
				
				// Create JSON payload
				timestamp := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
				logEntry := LogEntry{
					Timestamp: timestamp,
					Message:   message,
				}
				
				jsonData, err := json.Marshal(logEntry)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating JSON payload: %v\n", err)
					debugf("Error creating JSON payload: %v", err)
					continue
				}
				
				// Send HTTP request
				if err := c.sendHTTPRequest(ctx, jsonData); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to send log: %v\n", err)
					debugf("Failed to send log: %v", err)
					// Wait a bit before retrying
					time.Sleep(1 * time.Second)
					continue
				}
				
				debugf("Successfully sent log entry")
			}
		} else {
			// No data, short pause
			debugf("No data read from buffer, short pause")
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// sendHTTPRequest sends a single log entry via HTTP POST
func (c *HTTPClient) sendHTTPRequest(ctx context.Context, jsonData []byte) error {
	// Create request with timeout context
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	debugf("Sending HTTP request to %s", c.url)
	req, err := http.NewRequestWithContext(reqCtx, "POST", c.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	}
	
	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("got non-success status code %d: %s", resp.StatusCode, body)
	}
	
	debugf("HTTP request succeeded with status %d", resp.StatusCode)
	return nil
}

// extractMessage extracts the actual message from a syslog formatted line
func extractMessage(line string) string {
	// Simple extraction - find the last " - - - " separator and take everything after it
	parts := strings.Split(line, " - - - ")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	
	// If we can't parse it, just return the original line
	return line
}

// loadTLSConfig loads certificate and prepares TLS configuration
func loadTLSConfig(certFile string) (*tls.Config, error) {
	debugf("Loading certificate from %s", certFile)
	caCert, err := os.ReadFile(certFile)
	if err != nil {
		debugf("Failed to read certificate file: %v", err)
		return nil, fmt.Errorf("error loading certificate: %w", err)
	}
	debugf("Successfully read %d bytes from certificate file", len(caCert))

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		debugf("Failed to parse PEM certificate data")
		return nil, fmt.Errorf("failed to append CA certificate")
	}
	debugf("Successfully added certificate to pool")

	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	}
	debugf("TLS config created with min version TLS 1.2")
	
	return tlsConfig, nil
}