package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"time"
)

const (
	DefaultMaxSize    = 100 * 1024 * 1024 // 100MB default
	ReconnectTimeout  = 60 * time.Second
	ReadChunkSize     = 4096      // 4KB read chunks
	InitialBufferSize = 64 * 1024 // Start with 64KB
	DefaultBatchSize  = 10        // Default number of logs per batch
	DefaultMaxRetries = 3         // Default number of retries
	DefaultHTTPTimeout = 30 * time.Second // Default HTTP client timeout
	DefaultRequestTimeout = 10 * time.Second // Default per-request timeout
)

// ErrInvalidConfig is returned when required configuration is missing
var ErrInvalidConfig = errors.New("invalid configuration")

// Config holds all program configuration
type Config struct {
	CertFile        string
	Host            string
	Port            int
	ProgramName     string
	BufferPath      string
	MaxSize         int64
	ShowVersion     bool
	Verbose         bool
	Quiet           bool          // Suppress echoing logs to stdout
	AuthToken       string
	InsecureSSL     bool
	BatchSize       int           // Number of log entries to batch in a single HTTP request
	MaxRetries      int           // Maximum number of retries for failed requests
	HTTPTimeout     time.Duration // Overall HTTP client timeout
	RequestTimeout  time.Duration // Per-request timeout
	EnableBatching  bool          // Whether to enable log batching
	CompressLogs    bool          // Whether to compress logs (gzip) before sending
}

// Validate checks if the config has all required fields
func (c *Config) Validate() error {
	// Certificate path is optional
	if c.Host == "" {
		return fmt.Errorf("%w: host is required", ErrInvalidConfig)
	}
	if c.Port <= 0 {
		return fmt.Errorf("%w: valid port is required", ErrInvalidConfig)
	}
	if c.AuthToken == "" {
		return fmt.Errorf("%w: authorization token is required", ErrInvalidConfig)
	}
	return nil
}

// LogFatalFunc defines the signature for a fatal logging function
type LogFatalFunc func(v ...interface{})

// DefaultLogFatal is the default implementation of LogFatalFunc
var DefaultLogFatal LogFatalFunc = log.Fatal

// CurrentLogFatal is the current implementation of LogFatalFunc (can be swapped for testing)
var CurrentLogFatal LogFatalFunc = DefaultLogFatal

// ParseFlags parses command line flags and returns a config
func ParseFlags() *Config {
	config := &Config{}

	flag.StringVar(&config.CertFile, "cert", "", "Path to certificate bundle (optional, uses system certs if not provided)")
	flag.StringVar(&config.Host, "host", "", "Log destination host (e.g., s8676456.eu-nbg-2.betterstackdata.com)")
	flag.IntVar(&config.Port, "port", 443, "Port for log destination (defaults to 443 for HTTPS)")
	flag.StringVar(&config.ProgramName, "program", "custom-logger", "Program name for log identification")
	flag.StringVar(&config.BufferPath, "buffer", "papertrail_buffer.log", "Path to buffer file")
	flag.StringVar(&config.AuthToken, "token", "", "Authorization token (required for HTTP API)")
	maxSize := flag.Int64("maxsize", DefaultMaxSize, "Maximum buffer size in bytes")
	batchSize := flag.Int("batch", DefaultBatchSize, "Number of log entries to batch in a single request")
	maxRetries := flag.Int("retries", DefaultMaxRetries, "Maximum number of retries for failed requests")
	httpTimeout := flag.Duration("timeout", DefaultHTTPTimeout, "Overall HTTP client timeout")
	requestTimeout := flag.Duration("req-timeout", DefaultRequestTimeout, "Per-request timeout")
	enableBatching := flag.Bool("enable-batch", true, "Enable log batching")
	compressLogs := flag.Bool("compress", false, "Compress logs using gzip before sending")
	showVersion := flag.Bool("version", false, "Show version information and exit")
	verbose := flag.Bool("v", false, "Enable verbose debug logging")
	quiet := flag.Bool("q", false, "Quiet mode - don't echo log lines to stdout")
	quietLong := flag.Bool("quiet", false, "Quiet mode - don't echo log lines to stdout")
	insecureSSL := flag.Bool("k", false, "Allow insecure SSL connections (skip certificate validation)")
	flag.Parse()

	config.MaxSize = *maxSize
	config.BatchSize = *batchSize
	config.MaxRetries = *maxRetries
	config.HTTPTimeout = *httpTimeout
	config.RequestTimeout = *requestTimeout
	config.EnableBatching = *enableBatching
	config.CompressLogs = *compressLogs
	config.ShowVersion = *showVersion
	config.Verbose = *verbose
	// Set quiet mode if either -q or --quiet is specified
	config.Quiet = *quiet || *quietLong
	config.InsecureSSL = *insecureSSL

	// If version flag is set, we'll handle this separately in main() so skip validation
	if !config.ShowVersion {
		// Validate required flags
		if err := config.Validate(); err != nil {
			CurrentLogFatal(err)
		}
	}

	return config
}
