package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// BufferInterface defines the interface for buffer types
type BufferInterface interface {
	Write(data []byte) (int, error)
	Read(maxBytes int64) ([]byte, error)
	HasData() bool
	GetSize() int64
	Close() error
}

// CircularBuffer implements a simple circular buffer using a file
type CircularBuffer struct {
	file     *os.File
	mutex    sync.Mutex
	readPos  int64
	writePos int64
	size     int64
	fileSize int64
	maxSize  int64
}

// NewBuffer creates a new circular buffer with dynamic growth
func NewBuffer(path string, maxSize int64) (*CircularBuffer, error) {
	debugf("Creating buffer with path: %s, maxSize: %d bytes", path, maxSize)

	// Create buffer directory if it doesn't exist
	dir := filepath.Dir(path)
	debugf("Ensuring buffer directory exists: %s", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		debugf("Failed to create buffer directory: %v", err)
		return nil, fmt.Errorf("failed to create buffer directory: %w", err)
	}

	// Open or create the buffer file
	debugf("Opening buffer file: %s", path)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		debugf("Failed to open buffer file: %v", err)
		return nil, fmt.Errorf("failed to open buffer file: %w", err)
	}
	debugf("Buffer file opened successfully")

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		debugf("Failed to stat buffer file: %v", err)
		file.Close()
		return nil, fmt.Errorf("failed to stat buffer file: %w", err)
	}

	fileSize := info.Size()
	debugf("Initial buffer file size: %d bytes", fileSize)

	// Initialize with minimal size if empty
	if fileSize == 0 {
		debugf("Buffer file is empty, initializing with size: %d bytes", InitialBufferSize)
		if err := file.Truncate(InitialBufferSize); err != nil {
			debugf("Failed to initialize buffer file: %v", err)
			file.Close()
			return nil, fmt.Errorf("failed to initialize buffer file: %w", err)
		}
		fileSize = InitialBufferSize
	}

	return &CircularBuffer{
		file:     file,
		maxSize:  maxSize,
		fileSize: fileSize,
		size:     0, // Start assuming buffer is empty
		readPos:  0,
		writePos: 0,
	}, nil
}

// Write writes data to the buffer
func (cb *CircularBuffer) Write(data []byte) (int, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	dataLen := int64(len(data))

	// Check if data is larger than max buffer
	if dataLen > cb.maxSize {
		return 0, fmt.Errorf("data exceeds maximum buffer size")
	}

	// Check if buffer needs to grow
	requiredSpace := cb.size + dataLen
	if requiredSpace > cb.fileSize {
		// Need to grow the file (if possible)
		newSize := cb.fileSize * 2
		if newSize > cb.maxSize {
			newSize = cb.maxSize
		}

		if newSize < requiredSpace && cb.size < cb.maxSize {
			// Overwrite oldest data in circular fashion
			cb.readPos = (cb.writePos + dataLen) % cb.fileSize
			cb.size = cb.fileSize
		} else if newSize >= requiredSpace {
			// Grow the file
			if err := cb.file.Truncate(newSize); err != nil {
				return 0, fmt.Errorf("failed to grow buffer: %w", err)
			}
			cb.fileSize = newSize
		}
	}

	// Write the data, handling wrapping if needed
	if cb.writePos+dataLen <= cb.fileSize {
		// Simple case: write in one chunk
		if _, err := cb.file.WriteAt(data, cb.writePos); err != nil {
			return 0, err
		}
	} else {
		// Write in two chunks (wrap around)
		firstChunkSize := cb.fileSize - cb.writePos
		if _, err := cb.file.WriteAt(data[:firstChunkSize], cb.writePos); err != nil {
			return 0, err
		}
		if _, err := cb.file.WriteAt(data[firstChunkSize:], 0); err != nil {
			return 0, err
		}
	}

	// Update write position and size
	cb.writePos = (cb.writePos + dataLen) % cb.fileSize

	// Update size, capped at fileSize
	if cb.size < cb.fileSize {
		cb.size += dataLen
		if cb.size > cb.fileSize {
			cb.size = cb.fileSize
		}
	}

	return len(data), nil
}

// Read reads data from the buffer
func (cb *CircularBuffer) Read(maxBytes int64) ([]byte, error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.size == 0 {
		return nil, io.EOF
	}

	// Calculate how much to read
	toRead := cb.size
	if toRead > maxBytes {
		toRead = maxBytes
	}

	data := make([]byte, toRead)

	// Handle reading with potential wrap-around
	if cb.readPos+toRead <= cb.fileSize {
		// Read in one chunk
		if _, err := cb.file.ReadAt(data, cb.readPos); err != nil {
			return nil, err
		}
	} else {
		// Read in two chunks
		firstChunkSize := cb.fileSize - cb.readPos
		if _, err := cb.file.ReadAt(data[:firstChunkSize], cb.readPos); err != nil {
			return nil, err
		}
		if _, err := cb.file.ReadAt(data[firstChunkSize:], 0); err != nil {
			return nil, err
		}
	}

	// Update read position and size
	cb.readPos = (cb.readPos + toRead) % cb.fileSize
	cb.size -= toRead

	return data, nil
}

// HasData returns true if buffer contains data
func (cb *CircularBuffer) HasData() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	return cb.size > 0
}

// GetSize returns the current size of data in the buffer
func (cb *CircularBuffer) GetSize() int64 {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	return cb.size
}

// Close closes the buffer file
func (cb *CircularBuffer) Close() error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	debugf("Closing buffer file")
	err := cb.file.Close()
	if err != nil {
		debugf("Error closing buffer file: %v", err)
	} else {
		debugf("Buffer file closed successfully")
	}
	return err
}
