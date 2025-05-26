package main

import (
	"bytes"
	"io"
	"sync"
)

// MockBuffer is a simple in-memory implementation of the buffer for testing
type MockBuffer struct {
	buffer     bytes.Buffer
	mutex      sync.Mutex
	closed     bool
	WriteError error // Error to return on Write calls
	ReadError  error // Error to return on Read calls
}

// NewMockBuffer creates a new mock buffer
func NewMockBuffer() *MockBuffer {
	return &MockBuffer{}
}

// Write implements the Write method for the mock buffer
func (m *MockBuffer) Write(data []byte) (int, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return 0, io.ErrClosedPipe
	}

	// Return error if one is set
	if m.WriteError != nil {
		return 0, m.WriteError
	}

	return m.buffer.Write(data)
}

// Read implements the Read method for the mock buffer
func (m *MockBuffer) Read(maxBytes int64) ([]byte, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return nil, io.ErrClosedPipe
	}

	// Return error if one is set
	if m.ReadError != nil {
		return nil, m.ReadError
	}

	if m.buffer.Len() == 0 {
		return nil, io.EOF
	}

	// Determine how much to read
	toRead := int(maxBytes)
	if toRead > m.buffer.Len() {
		toRead = m.buffer.Len()
	}

	data := make([]byte, toRead)
	n, err := m.buffer.Read(data)
	if err != nil {
		return nil, err
	}

	return data[:n], nil
}

// HasData returns whether the buffer has data
func (m *MockBuffer) HasData() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.buffer.Len() > 0
}

// GetSize returns the current size of data in the buffer
func (m *MockBuffer) GetSize() int64 {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return int64(m.buffer.Len())
}

// Close closes the buffer
func (m *MockBuffer) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.closed = true
	return nil
}

// GetContents returns the current contents of the buffer for inspection
func (m *MockBuffer) GetContents() []byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.buffer.Bytes()
}

// Reset empties the buffer
func (m *MockBuffer) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.buffer.Reset()
}
