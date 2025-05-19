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
	
	// Log the settings we're using
	fmt.Fprintf(os.Stderr, "Starting log forwarding to %s\n", c.url)
	if c.insecureSSL {
		fmt.Fprintf(os.Stderr, "Warning: TLS certificate verification is disabled\n")
	}
	
	// Track successful and failed requests
	var successCount, failCount int64
	lastStatusReport := time.Now()
	reportInterval := 1 * time.Minute
	
	for {
		// Check if we should exit
		select {
		case <-ctx.Done():
			debugf("Context canceled, shutting down SendLogs")
			return
		default:
		}
		
		// Periodically report statistics
		if time.Since(lastStatusReport) >= reportInterval {
			fmt.Fprintf(os.Stderr, "Log forwarding stats: %d successful, %d failed\n", 
				successCount, failCount)
			lastStatusReport = time.Now()
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
		debugf("Reading logs from buffer (size: %d bytes)", buffer.GetSize())
		data, err := buffer.Read(ReadChunkSize)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Error reading from buffer: %v\n", err)
			debugf("Error reading from buffer: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(data) > 0 {
			debugf("Processing %d bytes of log data", len(data))
			
			// Parse and convert to JSON for HTTP API
			// Each line is a separate log entry
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				
				// Log the actual content being sent
				logData([]byte(line))
				
				// Extract the actual message from the syslog format if present
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
					failCount++
					continue
				}
				
				// Send HTTP request
				statusCode, err := c.sendHTTPRequest(ctx, jsonData)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to send log: %v\n", err)
					debugf("Failed to send log: %v", err)
					failCount++
					// Wait a bit before retrying
					time.Sleep(1 * time.Second)
					continue
				}
				
				// Record success
				successCount++
				// Log success to stderr (important for stdout) with status code
				fmt.Fprintf(os.Stderr, "Successfully sent log entry (HTTP %d)\n", statusCode)
				debugf("Successfully sent log entry with status code: %d", statusCode)
			}
		} else {
			// No data, short pause
			debugf("No data read from buffer, short pause")
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// sendHTTPRequest sends a single log entry via HTTP POST and returns the status code
func (c *HTTPClient) sendHTTPRequest(ctx context.Context, jsonData []byte) (int, error) {
	// Create request with timeout context
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	debugf("Sending HTTP request to %s", c.url)
	req, err := http.NewRequestWithContext(reqCtx, "POST", c.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("error creating request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	}
	
	// Log request details in verbose mode
	debugf("Request headers: %+v", req.Header)
	debugf("Request payload: %s", string(jsonData))
	
	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Always log response status
	statusCode := resp.StatusCode
	
	// Check response status
	if statusCode < 200 || statusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return statusCode, fmt.Errorf("got non-success status code %d: %s", statusCode, body)
	}
	
	// For success responses, try to read response body for debugging
	debugf("HTTP request succeeded with status %d", statusCode)
	if isVerbose {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			debugf("Response body: %s", body)
		}
	}
	
	return statusCode, nil
}

// extractMessage extracts the actual message from a syslog formatted line
func extractMessage(line string) string {
	// For syslog format, find " - - - " as separator
	if idx := strings.Index(line, " - - - "); idx != -1 {
		return line[idx+7:] // Skip the separator
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