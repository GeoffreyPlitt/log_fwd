package main

import (
	"bytes"
	"compress/gzip"
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

// LogEntry represents a JSON log entry for the HTTP API
type LogEntry struct {
	Timestamp string `json:"dt"`
	Message   string `json:"message"`
}

// LogBatch represents a batch of log entries
type LogBatch []LogEntry

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

// extractMessage extracts the actual message from a syslog formatted line
func extractMessage(line string) string {
	// For syslog format, find " - - - " as separator
	if idx := strings.Index(line, " - - - "); idx != -1 {
		return line[idx+7:] // Skip the separator
	}

	// If we can't parse it, just return the original line
	return line
}

// createHTTPClient creates an HTTP client with proper TLS configuration
func createHTTPClient(cfg *Config) (*http.Client, error) {
	// Create HTTP transport with proper TLS config
	var transport *http.Transport

	if cfg.CertFile != "" {
		debugf("Using custom certificate: %s", cfg.CertFile)
		tlsConfig, err := loadTLSConfig(cfg.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
		transport = &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		}
	} else {
		// Use system root certificates
		debugf("Using system root certificates")
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: cfg.InsecureSSL,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.HTTPTimeout,
	}

	return client, nil
}

// sendHTTPRequest sends log entries via HTTP POST and returns the status code
func sendHTTPRequest(client *http.Client, ctx context.Context, url, token string, jsonData []byte, config *Config) (int, error) {
	// Create request with timeout context
	reqCtx, cancel := context.WithTimeout(ctx, config.RequestTimeout)
	defer cancel()

	// Just log that we're sending a request
	debugf("Sending HTTP request to %s", url)

	// Create request body, with optional compression
	var requestBody io.Reader = bytes.NewBuffer(jsonData)
	contentType := "application/json"

	if config.CompressLogs {
		var compressedBuffer bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuffer)
		if _, err := gzipWriter.Write(jsonData); err != nil {
			return 0, fmt.Errorf("error compressing data: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return 0, fmt.Errorf("error closing gzip writer: %w", err)
		}
		requestBody = &compressedBuffer
		contentType = "application/json+gzip"
		debugf("Compressed request payload from %d to %d bytes", len(jsonData), compressedBuffer.Len())
	}

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, requestBody)
	if err != nil {
		return 0, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)
	if config.CompressLogs {
		req.Header.Set("Content-Encoding", "gzip")
	}

	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		// Log HTTP request info (only in verbose mode)
		debugf("HTTP Request: %s %s (Auth: [REDACTED], %d bytes)",
			req.Method, req.URL, len(jsonData))
	} else {
		debugf("HTTP Request: %s %s (NO AUTH TOKEN, %d bytes)",
			req.Method, req.URL, len(jsonData))
	}

	// Log request details in verbose mode, but mask sensitive information
	// Copy headers and mask sensitive values
	maskedHeaders := make(http.Header)
	for k, v := range req.Header {
		if k == "Authorization" && len(v) > 0 {
			// Completely mask the token
			maskedHeaders.Set(k, "Bearer [REDACTED]")
		} else {
			maskedHeaders[k] = v
		}
	}
	debugf("Request headers: %+v", maskedHeaders)

	// Only log payload details in verbose mode
	if isVerbose {
		// Show sample of payload
		sampleSize := 500
		if len(jsonData) > sampleSize {
			debugf("Request payload (first %d bytes): %s...", sampleSize, string(jsonData[:sampleSize]))
		} else {
			debugf("Request payload: %s", string(jsonData))
		}
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Always log response status and headers
	statusCode := resp.StatusCode

	// Log detailed response information (only in verbose mode)
	debugf("Response received: %d %s", statusCode, resp.Status)
	debugf("Response protocol: %s", resp.Proto)

	// Log all response headers
	debugf("Response headers:")
	headerSummary := ""
	for k, v := range resp.Header {
		debugf("  %s: %s", k, strings.Join(v, ", "))
		if k == "Content-Type" || k == "Content-Length" || k == "Date" {
			headerSummary += fmt.Sprintf("%s: %s ", k, strings.Join(v, ", "))
		}
	}
	debugf("Response headers summary: %s", headerSummary)

	// Always read the response body for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		debugf("Error reading response body: %v", err)
	}

	// Create a new reader with the same content for potential error handling
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Log the body content (only in verbose mode)
	if len(bodyBytes) > 0 {
		if len(bodyBytes) > 1024 {
			debugf("Response body (%d bytes, first 1024): %s", len(bodyBytes), string(bodyBytes[:1024]))
			debugf("[truncated - total length: %d bytes]", len(bodyBytes))
		} else {
			debugf("Response body (%d bytes): %s", len(bodyBytes), string(bodyBytes))
		}
	} else {
		debugf("Response body: [empty]")
	}

	// Check response status
	if statusCode < 200 || statusCode >= 300 {
		return statusCode, fmt.Errorf("got non-success status code %d: %s", statusCode, string(bodyBytes))
	}

	// Log summary for successful responses
	debugf("HTTP request succeeded with status %d", statusCode)

	return statusCode, nil
}

// sendBatchedLogs sends multiple log entries in a single request
func sendBatchedLogs(client *http.Client, ctx context.Context, url, token string, batch LogBatch, config *Config) (int, error) {
	// Marshal the batch to JSON
	jsonData, err := json.Marshal(batch)
	if err != nil {
		return 0, fmt.Errorf("error creating JSON batch payload: %w", err)
	}

	debugf("Sending batch of %d log entries (%d bytes)", len(batch), len(jsonData))
	return sendHTTPRequest(client, ctx, url, token, jsonData, config)
}
