package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
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
	// Load TLS config
	tlsConfig, err := loadTLSConfig(cfg.CertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS config: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

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
}

// SendLogs reads from buffer and sends to Papertrail
func (c *PapertrailClient) SendLogs(ctx context.Context, buffer Buffer, signal chan struct{}) {
	var conn *tls.Conn
	
	for {
		// Check if we should exit
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		// Check for data and connection
		if !buffer.HasData() {
			// Wait for new logs signal or timeout
			select {
			case <-signal:
				// Immediately continue to process new logs
			case <-time.After(5 * time.Second):
				// Regular poll
			case <-ctx.Done():
				return
			}
			continue
		}

		// Ensure connection is established
		if conn == nil {
			var err error
			conn, err = c.connectWithRetry(ctx)
			if err != nil {
				// Failed to connect even after retries
				log.Printf("Connection failed after retries: %v", err)
				time.Sleep(ReconnectTimeout)
				continue
			}
		}

		// Read and send logs
		data, err := buffer.Read(ReadChunkSize)
		if err != nil && err != io.EOF {
			log.Printf("Error reading from buffer: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(data) > 0 {
			if _, err := conn.Write(data); err != nil {
				log.Printf("Failed to send logs: %v", err)
				conn.Close()
				conn = nil
				continue
			}
		} else {
			// No data, short pause
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// connectWithRetry attempts to connect to Papertrail with retries
func (c *PapertrailClient) connectWithRetry(ctx context.Context) (*tls.Conn, error) {
	log.Printf("Connecting to %s", c.addr)
	
	// Try to connect immediately first
	conn, err := c.dialer.Dial(ctx)
	if err == nil {
		return conn, nil
	}
	
	// Set up retry with backoff
	retryDelay := 5 * time.Second
	maxRetryDelay := 60 * time.Second
	
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during connection retry: %w", ctx.Err())
		case <-time.After(retryDelay):
			// Try to connect
			conn, err := c.dialer.Dial(ctx)
			if err == nil {
				log.Printf("Connected to %s", c.addr)
				return conn, nil
			}
			
			log.Printf("Connection attempt failed: %v", err)
			
			// Increase retry delay with exponential backoff (capped)
			retryDelay *= 2
			if retryDelay > maxRetryDelay {
				retryDelay = maxRetryDelay
			}
		}
	}
}

// loadTLSConfig loads certificate and prepares TLS configuration
func loadTLSConfig(certFile string) (*tls.Config, error) {
	caCert, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("error loading certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	return &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	}, nil
}