package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"time"
)

// TLSDialer defines an interface for establishing TLS connections
type TLSDialer interface {
	Dial(ctx context.Context) (*tls.Conn, error)
}

// StandardTLSDialer implements the TLSDialer interface with standard TLS dialing
type StandardTLSDialer struct {
	tlsConfig *tls.Config
	addr      string
}

// Dial establishes a TLS connection
func (d *StandardTLSDialer) Dial(ctx context.Context) (*tls.Conn, error) {
	var dialer tls.Dialer
	dialer.Config = d.tlsConfig
	
	conn, err := dialer.DialContext(ctx, "tcp", d.addr)
	if err != nil {
		return nil, err
	}

	return conn.(*tls.Conn), nil
}

// PapertrailClient handles sending logs to Papertrail
type PapertrailClient struct {
	config    *Config
	dialer    TLSDialer
	addr      string
}

// NewClient creates a new Papertrail client
func NewClient(cfg *Config) (*PapertrailClient, error) {
	// Load TLS config if cert file provided, otherwise use system certs
	var tlsConfig *tls.Config
	var err error
	
	if cfg.CertFile != "" {
		debugf("Using custom certificate: %s", cfg.CertFile)
		tlsConfig, err = loadTLSConfig(cfg.CertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
	} else {
		// Use system root certificates
		debugf("Using system root certificates")
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	debugf("Configured Papertrail endpoint: %s", addr)

	// Create standard dialer
	dialer := &StandardTLSDialer{
		tlsConfig: tlsConfig,
		addr:      addr,
	}

	return &PapertrailClient{
		config: cfg,
		dialer: dialer,
		addr:   addr,
	}, nil
}

// NewClientWithDialer creates a new Papertrail client with a custom dialer (useful for testing)
func NewClientWithDialer(cfg *Config, dialer TLSDialer) *PapertrailClient {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	
	return &PapertrailClient{
		config: cfg,
		dialer: dialer,
		addr:   addr,
	}
}

// Buffer defines the interface for buffer types used with SendLogs
type Buffer interface {
	Read(maxBytes int64) ([]byte, error)
	HasData() bool
	GetSize() int64
}

// SendLogs reads from buffer and sends to Papertrail
func (c *PapertrailClient) SendLogs(ctx context.Context, buffer Buffer, signal chan struct{}) {
	var conn *tls.Conn
	
	debugf("SendLogs started for endpoint %s", c.addr)
	
	for {
		// Check if we should exit
		select {
		case <-ctx.Done():
			debugf("Context canceled, shutting down SendLogs")
			if conn != nil {
				debugf("Closing connection on shutdown")
				// Try to close connection gracefully on exit
				if err := conn.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Error closing connection on shutdown: %v\n", err)
					debugf("Failed to close connection gracefully: %v", err)
				} else {
					debugf("Connection closed gracefully")
				}
			}
			return
		default:
		}
		
		// Check for data and connection
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
				if conn != nil {
					debugf("Closing connection on shutdown")
					if err := conn.Close(); err != nil {
						fmt.Fprintf(os.Stderr, "Error closing connection on shutdown: %v\n", err)
						debugf("Failed to close connection gracefully: %v", err)
					} else {
						debugf("Connection closed gracefully")
					}
				}
				return
			}
			continue
		}

		// Ensure connection is established
		if conn == nil {
			debugf("No active connection, attempting to connect")
			var err error
			conn, err = c.connectWithRetry(ctx)
			if err != nil {
				// Failed to connect even after retries
				fmt.Fprintf(os.Stderr, "Connection failed after retries: %v\n", err)
				debugf("Connection failed even after retries: %v", err)
				debugf("Waiting %v before trying again", ReconnectTimeout)
				time.Sleep(ReconnectTimeout)
				continue
			}
			debugf("Connection established successfully")
		}

		// Read and send logs
		debugf("Reading logs from buffer")
		data, err := buffer.Read(ReadChunkSize)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Error reading from buffer: %v\n", err)
			debugf("Error reading from buffer: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(data) > 0 {
			debugf("Sending %d bytes of log data", len(data))
			// Log the actual content being sent
			logData(data)
			if _, err := conn.Write(data); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send logs: %v\n", err)
				debugf("Failed to send logs: %v", err)
				// Try to close the connection gracefully, log if error
				debugf("Closing failed connection")
				if closeErr := conn.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Error closing connection: %v\n", closeErr)
					debugf("Error closing failed connection: %v", closeErr)
				} else {
					debugf("Failed connection closed successfully")
				}
				conn = nil
				continue
			}
			debugf("Successfully sent %d bytes of log data", len(data))
		} else {
			// No data, short pause
			debugf("No data read from buffer, short pause")
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// connectWithRetry attempts to connect to Papertrail with retries
func (c *PapertrailClient) connectWithRetry(ctx context.Context) (*tls.Conn, error) {
	fmt.Fprintf(os.Stderr, "Connecting to %s\n", c.addr)
	
	// Try to connect immediately first
	debugf("Attempting initial connection to %s", c.addr)
	conn, err := c.dialer.Dial(ctx)
	if err == nil {
		debugf("Successfully connected to %s on first attempt", c.addr)
		return conn, nil
	}
	debugf("Initial connection failed: %v", err)
	
	// Set up retry with backoff
	retryDelay := 5 * time.Second
	maxRetryDelay := 60 * time.Second
	attempts := 1
	
	for {
		select {
		case <-ctx.Done():
			debugf("Context canceled during connection retry after %d attempts", attempts)
			return nil, fmt.Errorf("context canceled during connection retry: %w", ctx.Err())
		case <-time.After(retryDelay):
			// Try to connect
			attempts++
			debugf("Connection attempt %d after %v delay", attempts, retryDelay)
			conn, err := c.dialer.Dial(ctx)
			if err == nil {
				fmt.Fprintf(os.Stderr, "Connected to %s\n", c.addr)
				debugf("Successfully connected after %d attempts", attempts)
				return conn, nil
			}
			
			fmt.Fprintf(os.Stderr, "Connection attempt failed: %v\n", err)
			debugf("Connection attempt %d failed: %v", attempts, err)
			
			// Increase retry delay with exponential backoff (capped)
			retryDelay *= 2
			if retryDelay > maxRetryDelay {
				retryDelay = maxRetryDelay
			}
			debugf("Next retry in %v", retryDelay)
		}
	}
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