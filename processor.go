package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"
)

// ProcessInput reads from stdin and writes to the buffer
func ProcessInput(ctx context.Context, buffer *CircularBuffer, hostname, programName string, signal chan struct{}) {
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
		
		// Format log message according to RFC5424
		timestamp := time.Now().Format(time.RFC3339)
		logMessage := fmt.Sprintf(
			"<%d>1 %s %s %s - - - %s\n",
			13, // facility/priority (user notice)
			timestamp,
			hostname,
			programName,
			line,
		)
		
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
		maxConnectionRetries := 30 // About 3 seconds of retrying
		lastSize := buffer.GetSize()
		noProgressCount := 0
		maxNoProgress := 50 // About 5 seconds with no progress
		
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
					debugf("No progress in buffer flushing for %d checks", noProgressCount)
					
					if noProgressCount >= maxNoProgress {
						// Signal possible connection issue
						connectionRetries++
						debugf("Possible connection issue detected (retry %d/%d)", connectionRetries, maxConnectionRetries)
						
						if connectionRetries >= maxConnectionRetries {
							fmt.Fprintf(os.Stderr, "No progress in buffer flushing. Connection appears to be down. Exiting.\n")
							debugf("Giving up on flush after %d retries", connectionRetries)
							return
						}
					}
				} else {
					// Reset counters if we're making progress
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
		fmt.Fprintf(os.Stderr, "Buffer flushed successfully.\n")
		debugf("Buffer flush completed")
	}
}