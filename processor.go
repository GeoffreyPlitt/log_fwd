package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"
)

// ProcessInput reads from stdin and writes to the buffer
func ProcessInput(ctx context.Context, buffer *CircularBuffer, hostname, programName string, signal chan struct{}, cfg *Config) {
	scanner := bufio.NewScanner(os.Stdin)
	
	// Increase the buffer size to handle large lines
	const maxScannerBuffer = 256 * 1024 // 256KB
	buf := make([]byte, maxScannerBuffer)
	scanner.Buffer(buf, maxScannerBuffer)
	
	// Track if we got any logs to process
	hasProcessedLogs := false
	
	for scanner.Scan() {
		// Check if we should exit
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		line := scanner.Text()
		hasProcessedLogs = true
		
		// Echo the line to stdout if not in quiet mode
		if !cfg.Quiet {
			fmt.Println(line)
		}
		
		// We'll just store the raw message and format it later in the HTTP client
		// Just append a newline for readability in the buffer
		logMessage := line + "\n"
		
		// Write to buffer
		if _, err := buffer.Write([]byte(logMessage)); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to buffer: %v\n", err)
			// Check context before continuing
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		
		// Signal new logs (non-blocking)
		select {
		case signal <- struct{}{}:
		default:
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
	}
	
	// If we've processed logs, wait for buffer to empty before exiting
	if hasProcessedLogs && buffer.HasData() {
		fmt.Fprintf(os.Stderr, "Stdin closed. Waiting for buffer to flush...\n")
		debugf("Starting buffer flush process with %d bytes", buffer.GetSize())
		
		// Give the SendLogs goroutine time to flush remaining data
		flushTimeout := time.NewTicker(100 * time.Millisecond)
		defer flushTimeout.Stop()
		
		// Set up connection problem detection
		connectionRetries := 0
		maxConnectionRetries := 60 // About 6 seconds of retrying
		lastSize := buffer.GetSize()
		noProgressCount := 0
		maxNoProgress := 100 // About 10 seconds with no progress
		
		// Log initial buffer size for debugging
		fmt.Fprintf(os.Stderr, "Starting buffer flush with %d bytes of data\n", lastSize)
		
		for buffer.HasData() {
			// Check if we should exit
			select {
			case <-ctx.Done():
				debugf("Context canceled during buffer flush")
				return
			case <-flushTimeout.C:
				// Check for progress
				currentSize := buffer.GetSize()
				
				if currentSize == lastSize {
					noProgressCount++
					
					// Log every 10 checks (about 1 second) 
					if noProgressCount % 10 == 0 {
						fmt.Fprintf(os.Stderr, "No progress in buffer flushing for %.1f seconds (%d bytes remaining)\n", 
							float64(noProgressCount)/10.0, currentSize)
						debugf("No progress in buffer flushing for %d checks", noProgressCount)
					}
					
					if noProgressCount >= maxNoProgress {
						// Signal possible connection issue
						connectionRetries++
						fmt.Fprintf(os.Stderr, "Possible connection issue detected (retry %d/%d)\n", 
							connectionRetries, maxConnectionRetries)
						debugf("Possible connection issue detected (retry %d/%d)", connectionRetries, maxConnectionRetries)
						
						if connectionRetries >= maxConnectionRetries {
							fmt.Fprintf(os.Stderr, "No progress in buffer flushing. Connection appears to be down. Exiting.\n")
							debugf("Giving up on flush after %d retries", connectionRetries)
							return
						}
					}
				} else {
					// Reset counters if we're making progress
					if noProgressCount > 0 {
						fmt.Fprintf(os.Stderr, "Buffer flush resumed: %d bytes remaining\n", currentSize)
					}
					noProgressCount = 0
					connectionRetries = 0
					lastSize = currentSize
					debugf("Buffer flush progressing: %d bytes remaining", currentSize)
				}
				
				// Signal the sender that there are logs to process
				select {
				case signal <- struct{}{}:
					debugf("Signaled SendLogs to continue processing")
				default:
					debugf("Signal channel full, sender already processing")
				}
			}
		}
		
		// Extra delay to ensure all HTTP responses are received and logged
		// This gives time for the final HTTP response handling to complete
		fmt.Fprintf(os.Stderr, "Buffer is empty, waiting for final responses to be logged...\n")
		time.Sleep(1 * time.Second)
		
		fmt.Fprintf(os.Stderr, "Buffer flushed successfully.\n")
		debugf("Buffer flush completed")
	}
}