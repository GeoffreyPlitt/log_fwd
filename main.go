package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
)

// Version and BuildTime are set via ldflags during build
var (
	Version   = "1.0.0"
	BuildTime = "unknown"
	isVerbose = false
)

// debugf prints debug messages when verbose mode is enabled
func debugf(format string, args ...interface{}) {
	if isVerbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// logData safely logs binary data with non-printable characters escaped
func logData(data []byte) {
	if !isVerbose {
		return
	}

	// Create a safe version of the content for logging (up to a reasonable length)
	maxLen := 1024
	if len(data) > maxLen {
		debugf("(Showing first %d bytes of %d total)", maxLen, len(data))
		data = data[:maxLen]
	}

	safeContent := make([]byte, 0, len(data))
	for _, b := range data {
		// Replace non-printable characters with their escaped representation
		if b < 32 || b > 126 {
			// For common control characters, use their standard escape sequences
			switch b {
			case '\n':
				safeContent = append(safeContent, '\\', 'n')
			case '\r':
				safeContent = append(safeContent, '\\', 'r')
			case '\t':
				safeContent = append(safeContent, '\\', 't')
			default:
				// For other non-printables, use \xHH format
				hex := fmt.Sprintf("\\x%02x", b)
				safeContent = append(safeContent, []byte(hex)...)
			}
		} else {
			safeContent = append(safeContent, b)
		}
	}

	debugf("Log content: %s", string(safeContent))
}

func main() {
	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v\nStacktrace: %s", r, debug.Stack())
			os.Exit(1)
		}
	}()

	// Parse command line arguments
	cfg := ParseFlags()

	// Set global verbose flag
	isVerbose = cfg.Verbose

	// Check if version flag was specified
	if cfg.ShowVersion {
		fmt.Printf("log_fwd version %s (built at %s)\n", Version, BuildTime)
		return
	}

	if isVerbose {
		debugf("Verbose logging enabled")
	}

	// Additional validation is already done in ParseFlags, but this demonstrates
	// how we would handle validation errors if we needed to

	// Create context for clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	setupSignalHandling(cancel)

	// Initialize the buffer
	buffer, err := NewBuffer(cfg.BufferPath, cfg.MaxSize)
	if err != nil {
		log.Fatalf("Failed to create buffer: %v", err)
	}
	defer func() {
		if err := buffer.Close(); err != nil {
			log.Printf("Error closing buffer: %v", err)
		}
	}()

	// Get hostname for log formatting
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Could not determine hostname: %v", err)
		hostname = "unknown-host"
	}

	// Create a channel to signal new logs are available
	newLogs := make(chan struct{}, 1)

	// Initialize log_fwd client
	client, err := NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create log_fwd client: %v", err)
	}

	// Start sender goroutine
	go client.SendLogs(ctx, buffer, newLogs)

	// Process stdin and write to buffer
	ProcessInput(ctx, buffer, hostname, cfg.ProgramName, newLogs, cfg)
}

// setupSignalHandling sets up handlers for OS signals
func setupSignalHandling(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-c
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()
}
