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
	"math/rand"
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

// Timestamp format constants
const (
	TimestampFormat = "2006-01-02 15:04:05 UTC" // Format for timestamp sent to log service
)

// LogEntry represents a JSON log entry for the HTTP API
type LogEntry struct {
	Timestamp string `json:"dt"`
	Message   string `json:"message"`
}

// LogBatch represents a batch of log entries
type LogBatch []LogEntry

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
		transport = &http.Transport{
			TLSClientConfig: tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		}
	} else {
		// Use system root certificates
		debugf("Using system root certificates")
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
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

	// Build URL from host and port
	url := fmt.Sprintf("https://%s", cfg.Host)
	if cfg.Port != 443 {
		url = fmt.Sprintf("https://%s:%d", cfg.Host, cfg.Port)
	}
	
	debugf("Configured HTTP API endpoint: %s", url)
	debugf("HTTP client timeout: %s, Request timeout: %s", cfg.HTTPTimeout, cfg.RequestTimeout)
	debugf("Batching enabled: %v, Batch size: %d", cfg.EnableBatching, cfg.BatchSize)
	debugf("Max retries: %d, Compression enabled: %v", cfg.MaxRetries, cfg.CompressLogs)

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

// calculateBackoff calculates retry backoff with jitter
func calculateBackoff(retryCount int) time.Duration {
	if retryCount <= 0 {
		return 100 * time.Millisecond
	}
	
	// Exponential backoff with jitter
	backoff := time.Duration(1<<uint(retryCount-1)) * time.Second
	
	// Maximum backoff of 30 seconds
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	
	// Add jitter (±20%)
	jitter := rand.Float64()*0.4 - 0.2 // -20% to +20%
	backoff = time.Duration(float64(backoff) * (1 + jitter))
	
	return backoff
}

// SendLogs reads from buffer and sends to the HTTP API
func (c *HTTPClient) SendLogs(ctx context.Context, buffer Buffer, signal chan struct{}) {
	debugf("SendLogs started for HTTP API endpoint %s", c.url)
	
	// Initialize random number generator for jitter
	rand.Seed(time.Now().UnixNano())
	
	// Log the settings we're using
	fmt.Fprintf(os.Stderr, "Starting log forwarding to %s\n", c.url)
	if c.insecureSSL {
		fmt.Fprintf(os.Stderr, "Warning: TLS certificate verification is disabled\n")
	}
	
	// Log batching status
	if c.config.EnableBatching {
		fmt.Fprintf(os.Stderr, "Log batching enabled (batch size: %d)\n", c.config.BatchSize)
	} else {
		fmt.Fprintf(os.Stderr, "Log batching disabled (sending one log at a time)\n")
	}
	
	// Track successful and failed requests
	var successCount, failCount, batchCount int64
	lastStatusReport := time.Now()
	reportInterval := 1 * time.Minute
	
	// Create a request queue so we can process messages
	// and only mark them as processed after success
	type queuedMessage struct {
		message string
		retries int
		lastAttempt time.Time
	}
	
	requestQueue := make([]queuedMessage, 0, 1000)
	
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
			if batchCount > 0 {
				fmt.Fprintf(os.Stderr, "Log forwarding stats: %d successful, %d failed, %d batches sent\n", 
					successCount, failCount, batchCount)
			} else {
				fmt.Fprintf(os.Stderr, "Log forwarding stats: %d successful, %d failed\n", 
					successCount, failCount)
			}
			lastStatusReport = time.Now()
		}
		
		// If the queue is empty, check for more data in the buffer
		if len(requestQueue) == 0 && buffer.HasData() {
			debugf("Reading more logs from buffer (size: %d bytes)", buffer.GetSize())
			data, err := buffer.Read(ReadChunkSize)
			if err != nil && err != io.EOF {
				fmt.Fprintf(os.Stderr, "Error reading from buffer: %v\n", err)
				debugf("Error reading from buffer: %v", err)
				time.Sleep(1 * time.Second)
				
				// Try signal and continue
				select {
				case <-signal:
					// Got a signal, continue processing
				default:
					// No signal, just sleep
					time.Sleep(100 * time.Millisecond)
				}
				continue
			}
			
			if len(data) > 0 {
				debugf("Processing %d bytes of log data", len(data))
				
				// Split data into lines and queue them for processing
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					if line != "" {
						requestQueue = append(requestQueue, queuedMessage{
							message: line,
							retries: 0,
							lastAttempt: time.Time{}, // Zero time (never attempted)
						})
					}
				}
				
				debugf("Queued %d messages for processing", len(requestQueue))
			}
		}
		
		// If there's nothing to do, wait for signal or timeout
		if len(requestQueue) == 0 && !buffer.HasData() {
			debugf("No data in buffer or request queue, waiting for new logs")
			select {
			case <-signal:
				debugf("Received signal that new logs are available")
				// Immediately continue to process new logs
			case <-time.After(c.config.PollingInterval):
				debugf("Regular polling for new logs (interval: %v)", c.config.PollingInterval)
				// Use configured polling interval
			case <-ctx.Done():
				debugf("Context canceled while waiting for new logs")
				return
			}
			continue
		}
		
		// Process messages from the queue (as a batch if enabled)
		if len(requestQueue) > 0 {
			// Determine if we should process in batch or single mode
			if c.config.EnableBatching && len(requestQueue) >= c.config.BatchSize {
				// Batch processing
				processBatch := true
				batchSize := c.config.BatchSize
				if batchSize > len(requestQueue) {
					batchSize = len(requestQueue)
				}
				
				// Check if all messages in batch are ready for processing
				for i := 0; i < batchSize; i++ {
					// If any message is on backoff, skip processing this batch
					if !requestQueue[i].lastAttempt.IsZero() && 
					   time.Since(requestQueue[i].lastAttempt) < calculateBackoff(requestQueue[i].retries) {
						processBatch = false
						break
					}
				}
				
				if processBatch {
					// Create a batch of log entries
					batch := make(LogBatch, 0, batchSize)
					
					// Process each message in the batch
					for i := 0; i < batchSize; i++ {
						// Extract message and create log entry
						message := extractMessage(requestQueue[i].message)
						logEntry := LogEntry{
							Timestamp: time.Now().UTC().Format(TimestampFormat),
							Message:   message,
						}
						batch = append(batch, logEntry)
					}
					
					// Send the batch
					statusCode, err := c.sendBatchedLogs(ctx, batch)
					
					if err != nil {
						// If we get an error, increment retry count for all messages in batch
						fmt.Fprintf(os.Stderr, "Failed to send batch of %d logs: %v\n", batchSize, err)
						debugf("Failed to send batch: %v", err)
						
						// Update retry count and last attempt time for each message
						now := time.Now()
						for i := 0; i < batchSize; i++ {
							msg := requestQueue[i]
							msg.retries++
							msg.lastAttempt = now
							
							// If max retries reached, mark as failed
							if msg.retries > c.config.MaxRetries {
								failCount++
								// Mark for removal by setting retry count beyond max
								msg.retries = c.config.MaxRetries + 1
							}
							
							requestQueue[i] = msg
						}
						
						// Remove any messages that have exceeded max retries
						newQueue := make([]queuedMessage, 0, len(requestQueue))
						for _, msg := range requestQueue {
							if msg.retries <= c.config.MaxRetries {
								newQueue = append(newQueue, msg)
							} else {
								fmt.Fprintf(os.Stderr, "Giving up on message after %d attempts\n", c.config.MaxRetries)
							}
						}
						requestQueue = newQueue
						
						// Sleep for a bit before continuing
						time.Sleep(100 * time.Millisecond)
						continue
					}
					
					// Record success
					successCount += int64(batchSize)
					batchCount++
					
					// Log success with detailed information
					fmt.Fprintf(os.Stderr, "------------------------------------------------------\n")
					fmt.Fprintf(os.Stderr, "Successfully sent batch of %d log entries (HTTP %d)\n", 
						batchSize, statusCode)
					fmt.Fprintf(os.Stderr, "All messages delivered successfully to %s\n", c.url)
					fmt.Fprintf(os.Stderr, "------------------------------------------------------\n")
					debugf("Successfully sent batch with status code: %d", statusCode)
					
					// Remove the processed messages from the queue
					if batchSize == len(requestQueue) {
						requestQueue = requestQueue[:0] // Clear the queue
					} else {
						requestQueue = requestQueue[batchSize:]
					}
					
					// Short pause between requests to avoid flooding
					time.Sleep(50 * time.Millisecond)
				} else {
					// Some messages are on backoff, wait a bit and try again
					time.Sleep(50 * time.Millisecond)
				}
			} else {
				// Single message processing
				msg := requestQueue[0]
				
				// Check if this message is on backoff
				if !msg.lastAttempt.IsZero() {
					backoff := calculateBackoff(msg.retries)
					elapsed := time.Since(msg.lastAttempt)
					
					if elapsed < backoff {
						// Not ready to retry yet
						waitTime := backoff - elapsed
						debugf("Message on backoff, waiting %v before retry", waitTime)
						
						// Short sleep and continue
						time.Sleep(50 * time.Millisecond)
						continue
					}
				}
				
				// Log the actual content being sent
				logData([]byte(msg.message))
				
				// Extract the actual message from the syslog format if present
				message := extractMessage(msg.message)
				
				// Create JSON payload
				timestamp := time.Now().UTC().Format(TimestampFormat)
				logEntry := LogEntry{
					Timestamp: timestamp,
					Message:   message,
				}
				
				jsonData, err := json.Marshal(logEntry)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating JSON payload: %v\n", err)
					debugf("Error creating JSON payload: %v", err)
					failCount++
					
					// Remove this message from the queue since it can't be processed
					requestQueue = requestQueue[1:]
					continue
				}
				
				// Send HTTP request
				statusCode, err := c.sendHTTPRequest(ctx, jsonData)
				if err != nil {
					// If we get an error, increment retry count
					msg.retries++
					msg.lastAttempt = time.Now()
					
					fmt.Fprintf(os.Stderr, "Failed to send log (attempt %d): %v\n", 
						msg.retries, err)
					debugf("Failed to send log: %v", err)
					
					if msg.retries > c.config.MaxRetries {
						// Give up after max retries and remove from queue
						failCount++
						fmt.Fprintf(os.Stderr, "Giving up on message after %d attempts\n", msg.retries)
						requestQueue = requestQueue[1:]
					} else {
						// Put updated retry count back
						requestQueue[0] = msg
					}
					
					// Let the next loop iteration handle backoff
					continue
				}
				
				// Record success
				successCount++
				
				// Log success to stderr with status code and detailed information
				fmt.Fprintf(os.Stderr, "------------------------------------------------------\n")
				fmt.Fprintf(os.Stderr, "Successfully sent log entry (HTTP %d)\n", statusCode)
				fmt.Fprintf(os.Stderr, "Message delivered successfully to %s\n", c.url)
				fmt.Fprintf(os.Stderr, "------------------------------------------------------\n")
				debugf("Successfully sent log entry with status code: %d", statusCode)
				
				// Remove the processed message from the queue
				requestQueue = requestQueue[1:]
				
				// Short pause between requests to avoid flooding
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

// sendHTTPRequest sends log entries via HTTP POST and returns the status code
func (c *HTTPClient) sendHTTPRequest(ctx context.Context, jsonData []byte) (int, error) {
	// Create request with timeout context
	reqCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
	defer cancel()
	
	// Just log that we're sending a request
	debugf("Sending HTTP request to %s", c.url)
	
	// Create request body, with optional compression
	var requestBody io.Reader = bytes.NewBuffer(jsonData)
	contentType := "application/json"
	
	if c.config.CompressLogs {
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
	
	req, err := http.NewRequestWithContext(reqCtx, "POST", c.url, requestBody)
	if err != nil {
		return 0, fmt.Errorf("error creating request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", contentType)
	if c.config.CompressLogs {
		req.Header.Set("Content-Encoding", "gzip")
	}
	
	if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
		// Log HTTP method, URL and request size - but don't log token details
		fmt.Fprintf(os.Stderr, "HTTP Request: %s %s (Auth: [REDACTED], %d bytes)\n", 
			req.Method, req.URL, len(jsonData))
	} else {
		fmt.Fprintf(os.Stderr, "HTTP Request: %s %s (NO AUTH TOKEN, %d bytes)\n", 
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
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Always log response status and headers
	statusCode := resp.StatusCode
	
	// Log detailed response information - always log to stderr for visibility
	fmt.Fprintf(os.Stderr, "Response received: %d %s\n", statusCode, resp.Status)
	debugf("Response status: %d %s", statusCode, resp.Status)
	debugf("Response protocol: %s", resp.Proto)
	
	// Log all response headers
	debugf("Response headers:")
	// Print a summary to stderr as well
	headerSummary := ""
	for k, v := range resp.Header {
		debugf("  %s: %s", k, strings.Join(v, ", "))
		if k == "Content-Type" || k == "Content-Length" || k == "Date" {
			headerSummary += fmt.Sprintf("%s: %s ", k, strings.Join(v, ", "))
		}
	}
	if headerSummary != "" {
		fmt.Fprintf(os.Stderr, "Response headers: %s\n", headerSummary)
	}
	
	// Always read the response body for logging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		debugf("Error reading response body: %v", err)
	}
	
	// Create a new reader with the same content for potential error handling
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	
	// For all responses, log the body content
	if len(bodyBytes) > 0 {
		// Always print to stderr for easier debugging
		fmt.Fprintf(os.Stderr, "Response body (%d bytes):\n", len(bodyBytes))
		if len(bodyBytes) > 1024 {
			fmt.Fprintf(os.Stderr, "  (first 1024 bytes): %s\n", string(bodyBytes[:1024]))
			debugf("  [truncated - total length: %d bytes]", len(bodyBytes))
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", string(bodyBytes))
		}
	} else {
		fmt.Fprintf(os.Stderr, "Response body: [empty]\n")
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
func (c *HTTPClient) sendBatchedLogs(ctx context.Context, batch LogBatch) (int, error) {
	// Marshal the batch to JSON
	jsonData, err := json.Marshal(batch)
	if err != nil {
		return 0, fmt.Errorf("error creating JSON batch payload: %w", err)
	}
	
	debugf("Sending batch of %d log entries (%d bytes)", len(batch), len(jsonData))
	return c.sendHTTPRequest(ctx, jsonData)
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