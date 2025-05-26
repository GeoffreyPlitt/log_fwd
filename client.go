package main

import (
	"context"
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
	config      *Config
	client      *http.Client
	url         string
	authToken   string
	insecureSSL bool
}

// Constants
const (
	TimestampFormat = "2006-01-02 15:04:05 UTC" // Format for timestamp sent to log service
	MaxBatchLines   = 100                       // Maximum number of lines to process in one batch
)

// NewClient creates a new HTTP API client
func NewClient(cfg *Config) (*HTTPClient, error) {
	// Create HTTP client using the extracted function
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
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
		message     string
		retries     int
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
			// Calculate how much to read based on the buffer size
			// Try to read enough to get MaxBatchLines lines in one go
			// Estimate 100 bytes per line on average as a heuristic
			bufferSize := buffer.GetSize()
			readSize := int64(MaxBatchLines * 100) // Estimate to read enough for MaxBatchLines
			if readSize > bufferSize {
				readSize = bufferSize // Don't try to read more than what's available
			}
			if readSize < ReadChunkSize {
				readSize = ReadChunkSize // Read at least ReadChunkSize
			}

			debugf("Reading more logs from buffer (size: %d bytes, reading up to %d bytes)", bufferSize, readSize)
			data, err := buffer.Read(readSize)
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

				// Split data into lines and queue them for processing (up to MaxBatchLines)
				lines := strings.Split(string(data), "\n")
				linesAdded := 0

				for _, line := range lines {
					if line != "" {
						requestQueue = append(requestQueue, queuedMessage{
							message:     line,
							retries:     0,
							lastAttempt: time.Time{}, // Zero time (never attempted)
						})
						linesAdded++

						// Cap the number of lines we add in one go
						if linesAdded >= MaxBatchLines {
							debugf("Reached max lines limit (%d), will process remaining lines in next batch", MaxBatchLines)
							break
						}
					}
				}

				debugf("Queued %d messages for processing", linesAdded)
			}
		}

		// If there's nothing to do, wait for signal or timeout
		if len(requestQueue) == 0 && !buffer.HasData() {
			debugf("No data in buffer or request queue, waiting for new logs")
			select {
			case <-signal:
				debugf("Received signal that new logs are available")
				// Immediately continue to process new logs
			case <-time.After(1 * time.Second):
				debugf("Regular polling for new logs")
				// Short fallback polling interval for responsiveness
			case <-ctx.Done():
				debugf("Context canceled while waiting for new logs")
				return
			}
			continue
		}

		// Process messages from the queue (as a batch if enabled)
		if len(requestQueue) > 0 {
			// Determine if we should process in batch or single mode
			if c.config.EnableBatching && len(requestQueue) >= 1 {
				// Batch processing - use greedier approach with larger batches
				processBatch := true
				// Use either MaxBatchLines or the configured batch size, whichever is larger
				batchSize := MaxBatchLines
				if c.config.BatchSize > MaxBatchLines {
					batchSize = c.config.BatchSize
				}
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
					statusCode, err := sendBatchedLogs(c.client, ctx, c.url, c.authToken, batch, c.config)

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

					// Log success with detailed information (only in verbose mode)
					debugf("------------------------------------------------------")
					debugf("Successfully sent batch of %d log entries (HTTP %d)",
						batchSize, statusCode)
					debugf("All messages delivered successfully to %s", c.url)
					debugf("------------------------------------------------------")
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
				statusCode, err := sendHTTPRequest(c.client, ctx, c.url, c.authToken, jsonData, c.config)
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

				// Log success with detailed information (only in verbose mode)
				debugf("------------------------------------------------------")
				debugf("Successfully sent log entry (HTTP %d)", statusCode)
				debugf("Message delivered successfully to %s", c.url)
				debugf("------------------------------------------------------")
				debugf("Successfully sent log entry with status code: %d", statusCode)

				// Remove the processed message from the queue
				requestQueue = requestQueue[1:]

				// Short pause between requests to avoid flooding
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}
