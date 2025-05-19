package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockTLSDialer implements the TLSDialer interface for testing
type MockTLSDialer struct {
	MockConn  *MockConn
	DialError error
	DialCalls int
}

// MockConn implements net.Conn interface for testing
type MockConn struct {
	buffer     bytes.Buffer
	readBuffer bytes.Buffer
	mutex      sync.Mutex
	closed     bool
	readError  error
	writeError error
}

func (m *MockConn) Read(p []byte) (n int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed {
		return 0, net.ErrClosed
	}
	if m.readError != nil {
		return 0, m.readError
	}
	return m.readBuffer.Read(p)
}

func (m *MockConn) Write(p []byte) (n int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed {
		return 0, net.ErrClosed
	}
	if m.writeError != nil {
		return 0, m.writeError
	}
	return m.buffer.Write(p)
}

func (m *MockConn) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.closed = true
	return nil
}

func (m *MockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *MockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *MockConn) SetDeadline(t time.Time) error      { return nil }
func (m *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *MockConn) SetWriteDeadline(t time.Time) error { return nil }

// MockTLSConn is a mock implementation of a TLS connection
type MockTLSConn struct {
	net.Conn
	handshakeCalled bool
}

func (c *MockTLSConn) Handshake() error {
	c.handshakeCalled = true
	return nil
}

// Dial implements the TLSDialer interface
func (d *MockTLSDialer) Dial(ctx context.Context) (*tls.Conn, error) {
	d.DialCalls++
	if d.DialError != nil {
		return nil, d.DialError
	}
	
	// Use our mock connection
	conn := d.MockConn
	if conn == nil {
		conn = &MockConn{}
	}
	
	// Return a real tls.Conn for use in testing
	// We'll create a temporary certificate for this
	return createMockTLSConn(conn)
}

// Helper to create a temporary certificate and return a TLS connection
func createMockTLSConn(conn net.Conn) (*tls.Conn, error) {
	// For testing purposes, we'll avoid actual TLS handshakes
	// by returning a nil *tls.Conn - we'll handle this in our tests
	return nil, nil
}

// TestNewClient tests creating a new client
func TestNewClient(t *testing.T) {
	// Create a temporary test certificate
	tmpDir, err := os.MkdirTemp("", "client-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	certPath := filepath.Join(tmpDir, "cert.pem")
	
	// Write a dummy cert
	certContent := `-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXzRGz3yIGrjP7lE7b9Jp2buwNr4wDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMzA1MDExNjI4MDlaFw0yNDA0
MzAxNjI4MDlaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDKj6HGkl6pLZ6+XApTY8c2AJwKCzwXWqFe2zdD0e+T
2MxzXVE3UNJoEgTmrQiUWjzLQQqbz5rCiKXPZ5XgPpkS+fGmPo2QKKFJwQ8UPBBI
Bo8eI2aUca3pT7JG6wM1MzQQxrjEm+rKGibjxWGcxEQGSxYaOo1GnZFkOvYlHKzB
NKxvBDfF3Qc6vcnJYpNJWjBHPvh7XzRQXvWznJDJJt3MjxJA5XQm7+M1jw4t1I6w
cw7bYxcGDlop5A31+KCjrjFejxQXJchbhYTz8B20COg+/XGtJYK0bCxIZRiP5rPJ
SCITcDSxKicZcvClsZQZLf9GWYxJoVYNYENQMdTFAgMBAAGjUzBRMB0GA1UdDgQW
BBTgGeCYy4J6pRnwVfMC3LqyCdkdATAfBgNVHSMEGDAWgBTgGeCYy4J6pRnwVfMC
3LqyCdkdATAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBZkviS
SB8Y9pfvVyjAVGFGlDlTHENITUQvHDfGUd+HaDVvpVIAuA1ARIjWRmqwQ9cKz5UI
7H7WLwkUxk1sjVlbBWg4j8bZN9+tZPYx4frBUoVQCy7GD82wdMQCJCaY7dUHKw2r
6PYEeNHEyEbVmSWKfvjGewNuWQ7MJyBW4+1j5Qbh5A4bPsHcGnrEZWDlAdxq3BXr
-----END CERTIFICATE-----`
	
	if err := os.WriteFile(certPath, []byte(certContent), 0644); err != nil {
		t.Fatalf("Failed to write cert file: %v", err)
	}
	
	cfg := &Config{
		CertFile:    certPath,
		Host:        "example.com",
		Port:        12345,
		ProgramName: "test-program",
		BufferPath:  "test-buffer.log",
		MaxSize:     1024,
	}
	
	// Test creating a new client
	// This will fail because our cert is invalid, but we can verify
	// that the client was attempted to be constructed
	_, err = NewClient(cfg)
	if err != nil && !strings.Contains(err.Error(), "failed to append CA certificate") {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestNewClientWithDialer tests creating a client with a custom dialer
func TestNewClientWithDialer(t *testing.T) {
	mockDialer := &MockTLSDialer{
		MockConn: &MockConn{},
	}
	
	cfg := &Config{
		CertFile:    "fake.pem",
		Host:        "example.com",
		Port:        12345,
		ProgramName: "test-program",
		BufferPath:  "test-buffer.log",
		MaxSize:     1024,
	}
	
	client := NewClientWithDialer(cfg, mockDialer)
	
	if client == nil {
		t.Error("NewClientWithDialer returned nil")
	}
	
	if client.config != cfg {
		t.Error("Client config doesn't match provided config")
	}
	
	if client.dialer != mockDialer {
		t.Error("Client dialer doesn't match provided dialer")
	}
	
	expectedAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if client.addr != expectedAddr {
		t.Errorf("Client addr = %q, want %q", client.addr, expectedAddr)
	}
}

// Test the Standard TLS Dialer implementation
func TestStandardTLSDialer(t *testing.T) {
	// Create a dialer - we'll use an obviously invalid address
	// so the connection will fail in a predictable way
	dialer := &StandardTLSDialer{
		tlsConfig: &tls.Config{},
		addr:      "invalid-host:12345", 
	}
	
	// Attempt to dial - this should fail with a known error pattern
	_, err := dialer.Dial(context.Background())
	if err == nil {
		t.Error("Expected dial to fail with invalid host")
	}
}

// Note: We can't test SendLogs and connectWithRetry properly without 
// mocking the TLS connections completely, which is complex.
// This test simply verifies that our mock TLS dialer is properly called.
func TestDialerUsage(t *testing.T) {
	// Set up a simple test scenario
	mockDialer := &MockTLSDialer{
		MockConn: &MockConn{},
		// First return an error, then succeed on retry
		DialError: fmt.Errorf("test error"),
	}
	
	cfg := &Config{
		CertFile:    "fake.pem",
		Host:        "example.com",
		Port:        12345,
		ProgramName: "test-program",
		BufferPath:  "test-buffer.log",
		MaxSize:     1024,
	}
	
	client := NewClientWithDialer(cfg, mockDialer)
	
	// Now attempt to connect with retry, but we'll use a cancel context to prevent blocking
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start the attempt in a goroutine
	var errResult error
	go func() {
		_, errResult = client.connectWithRetry(ctx)
	}()
	
	// Allow some time for the first attempt
	time.Sleep(10 * time.Millisecond)
	
	// Verify the dialer was called
	if mockDialer.DialCalls == 0 {
		t.Error("Dialer was not called")
	}
	
	// Cancel the context to end the test
	cancel()
	
	// Wait a bit to ensure goroutine finishes
	time.Sleep(10 * time.Millisecond)
	
	// Should have received an error (either our test error or context canceled)
	if errResult == nil {
		t.Error("Expected an error from connectWithRetry")
	}
}