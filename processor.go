package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// ProcessInput reads from stdin and writes to the buffer
func ProcessInput(ctx context.Context, buffer *CircularBuffer, hostname, programName string, signal chan struct{}) {
	scanner := bufio.NewScanner(os.Stdin)
	
	for scanner.Scan() {
		// Check if we should exit
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		line := scanner.Text()
		
		// Format log message
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
			log.Printf("Error writing to buffer: %v", err)
			continue
		}
		
		// Signal new logs (non-blocking)
		select {
		case signal <- struct{}{}:
		default:
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading stdin: %v", err)
	}
}