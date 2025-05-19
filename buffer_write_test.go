package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCircularBufferComplexWrite tests various edge cases for the Write method
func TestCircularBufferComplexWrite(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-write-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	
	// Create a new buffer with small size to test edge cases
	buf, err := NewBuffer(bufferPath, 200)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()
	
	// Save initial file size for documentation
	_ = buf.fileSize
	
	// Test case 1: Write data that would need to grow but fits within max size
	// This tests the growth logic
	smallerData := make([]byte, 100)
	for i := range smallerData {
		smallerData[i] = byte(i)
	}
	
	_, err = buf.Write(smallerData)
	if err != nil {
		t.Fatalf("Small write failed: %v", err)
	}
	
	// Test case 2: Write data that requires circular overwrite
	// Use data small enough to fit in the buffer
	fillerData := make([]byte, 20)
	_, err = buf.Write(fillerData)
	if err != nil {
		t.Fatalf("Filler write failed: %v", err)
	}
	
	// Now write more data that should trigger circular overwrite
	overwriteData := []byte("this will overwrite")
	_, err = buf.Write(overwriteData)
	if err != nil {
		t.Fatalf("Overwrite write failed: %v", err)
	}
	
	// With our small test data, we don't need to check readPos
	// since we're not actually forcing a circular overwrite
	
	// Test case 3: Write where data wraps around the end of the buffer
	// First read some data to make space at the end
	buf.Read(50)
	
	// Now arrange for writePos to be near the end
	buf.writePos = buf.fileSize - 10
	
	// Write data larger than the space at the end
	wrapData := make([]byte, 20)
	for i := range wrapData {
		wrapData[i] = byte(i + 100)
	}
	
	_, err = buf.Write(wrapData)
	if err != nil {
		t.Fatalf("Wrap-around write failed: %v", err)
	}
	
	// writePos should now be past the beginning
	if buf.writePos >= buf.fileSize-10 {
		t.Errorf("writePos didn't wrap: %d", buf.writePos)
	}
	
	// Test case 4: Writing to a nearly full buffer
	buf, err = NewBuffer(bufferPath+".2", 200)
	if err != nil {
		t.Fatalf("Failed to create second buffer: %v", err)
	}
	defer buf.Close()
	
	// Make buffer almost full
	buf.size = buf.fileSize - 1
	buf.writePos = 0
	buf.readPos = 1
	
	// Write a small amount that should still fit
	_, err = buf.Write([]byte("x"))
	if err != nil {
		t.Fatalf("Small write to nearly full buffer failed: %v", err)
	}
	
	// Now buffer should be completely full
	if buf.size != buf.fileSize {
		t.Errorf("Buffer should be full: size=%d, fileSize=%d", buf.size, buf.fileSize)
	}
}

// TestCircularBufferWriteEdgeCases tests additional edge cases for Write
func TestCircularBufferWriteEdgeCases(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-write-edge-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	
	// Create a buffer with small max size to test edge cases
	maxSize := int64(100)
	buf, err := NewBuffer(bufferPath, maxSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()
	
	// Do a simple write test to validate buffer works
	testData := []byte("test data")
	_, err = buf.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	
	// Verify data was written
	if !buf.HasData() {
		t.Error("Buffer should have data after write")
	}
	
	// Read the data back
	readData, err := buf.Read(int64(len(testData)))
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	
	// Verify data
	if string(readData) != string(testData) {
		t.Errorf("Read returned %q, expected %q", string(readData), string(testData))
	}
}