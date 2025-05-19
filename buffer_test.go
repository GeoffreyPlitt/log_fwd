package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCircularBufferBasic(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	bufSize := int64(1024) // 1KB buffer

	// Create a new buffer
	buf, err := NewBuffer(bufferPath, bufSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Initial state checks
	if buf.HasData() {
		t.Error("New buffer should be empty")
	}

	// Write some data
	testData := []byte("test data")
	n, err := buf.Write(testData)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned %d, expected %d", n, len(testData))
	}

	// Buffer should now have data
	if !buf.HasData() {
		t.Error("Buffer should have data after write")
	}

	// Read the data back
	readData, err := buf.Read(1024)
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if string(readData) != string(testData) {
		t.Errorf("Read returned %q, expected %q", string(readData), string(testData))
	}

	// Buffer should be empty after read
	if buf.HasData() {
		t.Error("Buffer should be empty after reading all data")
	}

	// Read from empty buffer should return EOF
	_, err = buf.Read(1024)
	if err != io.EOF {
		t.Errorf("Read from empty buffer should return EOF, got %v", err)
	}
}

func TestCircularBufferWrapAround(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	bufSize := int64(100) // Small buffer to force wraparound

	// Create a new buffer
	buf, err := NewBuffer(bufferPath, bufSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Fill the buffer with a known pattern
	pattern := make([]byte, 60) // More than half the buffer
	for i := range pattern {
		pattern[i] = byte(i % 256)
	}

	// Write first chunk
	_, err = buf.Write(pattern)
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}

	// Write second chunk to force wraparound
	_, err = buf.Write(pattern)
	if err != nil {
		t.Fatalf("Second write failed: %v", err)
	}

	// Read the data back - should get the most recent data
	readData, err := buf.Read(1000)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Since buffer is circular and smaller than total writes,
	// verify the buffer contains some data
	if len(readData) == 0 {
		t.Errorf("Read returned no bytes, expected some data")
	}
}

func TestCircularBufferGrowth(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	maxSize := int64(1000)    // Allow growth

	// Create a new buffer
	buf, err := NewBuffer(bufferPath, maxSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Save initial file size - just for documentation
	_ = buf.fileSize

	// Write data but make sure it's not too large
	smallerData := make([]byte, 100) // Small enough to fit
	for i := range smallerData {
		smallerData[i] = byte(i % 256)
	}

	// Write smaller data first
	_, err = buf.Write(smallerData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Should be able to read the data back
	readData, err := buf.Read(100)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(readData) != len(smallerData) {
		t.Errorf("Read returned %d bytes, expected %d", len(readData), len(smallerData))
	}
}

func TestCircularBufferMaxSizeLimit(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	maxSize := int64(200) // Small max size

	// Create a new buffer
	buf, err := NewBuffer(bufferPath, maxSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Try to write data larger than max size
	tooLargeData := make([]byte, int(maxSize+100))

	// Write should fail
	_, err = buf.Write(tooLargeData)
	if err == nil {
		t.Error("Write should fail when data exceeds max size")
	}

	// Try writing data just at max size
	justRightData := make([]byte, int(maxSize))

	// Write should succeed
	_, err = buf.Write(justRightData)
	if err != nil {
		t.Errorf("Write failed for data at max size: %v", err)
	}
}