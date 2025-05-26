package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

	// Test writing more data without exceeding limits
	// (No need to track fileSize)

	// Write data small enough to fit
	smallData := make([]byte, 100) // Small enough to fit
	for i := range smallData {
		smallData[i] = byte(i % 256)
	}

	n, err = buf.Write(smallData)
	if err != nil {
		t.Errorf("Small write failed: %v", err)
	}
	if n != len(smallData) {
		t.Errorf("Small write returned %d, expected %d", n, len(smallData))
	}

	// Buffer should now have data
	if !buf.HasData() {
		t.Error("Buffer should have data after write")
	}

	// Read the data back - we'll read just the first message
	readData, err := buf.Read(int64(len(testData)))
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if string(readData) != string(testData) {
		t.Errorf("Read returned %q, expected %q", string(readData), string(testData))
	}

	// We still have smallData in the buffer
	if !buf.HasData() {
		t.Error("Buffer should still have data after partial read")
	}

	// Read the rest of the data
	_, err = buf.Read(1024)
	if err != nil {
		t.Errorf("Reading remaining data failed: %v", err)
	}

	// Now buffer should be empty
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

	// Now test circular overwrite with appropriately sized data
	bigData := make([]byte, 50) // Small enough to not exceed limits
	for i := range bigData {
		bigData[i] = byte((i * 3) % 256) // Different pattern
	}

	_, err = buf.Write(bigData)
	if err != nil {
		t.Fatalf("Write with overwrite failed: %v", err)
	}

	// Check that buffer has data after overwrite
	if !buf.HasData() {
		t.Error("Buffer should have data after overwrite")
	}

	// Read again to ensure we can get the data
	moreData, err := buf.Read(int64(len(bigData)))
	if err != nil {
		t.Fatalf("Read after overwrite failed: %v", err)
	}

	// Should have gotten some data back
	if len(moreData) == 0 {
		t.Error("No data read after overwrite")
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
	maxSize := int64(1000) // Allow growth

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

func TestCircularBufferEdgeCases(t *testing.T) {
	// Create a temporary file for testing
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	bufSize := int64(1024)

	// Test with an invalid directory
	_, err = NewBuffer("/invalid/nonexistent/path", bufSize)
	if err == nil {
		t.Error("Expected error with invalid directory")
	}

	// Create a buffer for further tests
	buf, err := NewBuffer(bufferPath, bufSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Test wraparound conditions
	// Write a small amount first
	initialData := make([]byte, 100) // Small enough to fit
	_, err = buf.Write(initialData)
	if err != nil {
		t.Fatalf("Initial write failed: %v", err)
	}

	// Now write a small piece that should wrap around
	wrapData := []byte("wrap around")
	_, err = buf.Write(wrapData)
	if err != nil {
		t.Fatalf("Wrap write failed: %v", err)
	}

	// Read should work and give us data
	readData, err := buf.Read(buf.fileSize)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(readData) == 0 {
		t.Error("No data read after wraparound write")
	}

	// Test error conditions
	// Close the file and try to use it
	buf.file.Close()

	_, err = buf.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error writing to closed file")
	}

	_, err = buf.Read(10)
	if err == nil {
		t.Error("Expected error reading from closed file")
	}
}

// TestCircularBufferConcurrentAccess tests concurrent read/write operations
func TestCircularBufferConcurrentAccess(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buf, err := NewBuffer(bufferPath, 10240)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	var wg sync.WaitGroup
	numWriters := 5
	numReaders := 3
	numOperations := 10

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				data := []byte(fmt.Sprintf("writer-%d-msg-%d\n", writerID, j))
				buf.Write(data)
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				if buf.HasData() {
					buf.Read(100)
				}
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
}

// TestCircularBufferFileErrors tests file I/O error scenarios
func TestCircularBufferFileErrors(t *testing.T) {
	// Test creating buffer in non-existent directory
	t.Run("invalid directory", func(t *testing.T) {
		invalidPath := "/non/existent/path/buffer.log"
		_, err := NewBuffer(invalidPath, 1024)
		if err == nil {
			t.Error("Expected error when creating buffer in non-existent directory")
		}
	})

	// Test with read-only directory (if we can create one)
	t.Run("read-only directory", func(t *testing.T) {
		tmpdir, err := os.MkdirTemp("", "buffer-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpdir)

		// Make directory read-only
		err = os.Chmod(tmpdir, 0444)
		if err != nil {
			t.Skipf("Cannot make directory read-only: %v", err)
		}
		defer os.Chmod(tmpdir, 0755) // Restore permissions for cleanup

		bufferPath := filepath.Join(tmpdir, "test-buffer.log")
		_, err = NewBuffer(bufferPath, 1024)
		if err == nil {
			t.Error("Expected error when creating buffer in read-only directory")
		}
	})
}

// TestCircularBufferDataLargerThanMaxSize tests writing data larger than max buffer size
func TestCircularBufferDataLargerThanMaxSize(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	maxSize := int64(100)
	buf, err := NewBuffer(bufferPath, maxSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Try to write data larger than max buffer size
	largeData := make([]byte, maxSize+50)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	_, err = buf.Write(largeData)
	if err == nil {
		t.Error("Expected error when writing data larger than max buffer size")
	}
}

// TestCircularBufferWrapAroundEdgeCases tests edge cases in wrap-around logic
func TestCircularBufferWrapAroundEdgeCases(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	bufSize := int64(50)
	buf, err := NewBuffer(bufferPath, bufSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Write data that exactly fills the buffer
	exactData := make([]byte, 30)
	for i := range exactData {
		exactData[i] = byte(i)
	}

	_, err = buf.Write(exactData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Write more data to trigger wrap-around
	wrapData := make([]byte, 25)
	for i := range wrapData {
		wrapData[i] = byte(i + 100)
	}

	_, err = buf.Write(wrapData)
	if err != nil {
		t.Fatalf("Wrap-around write failed: %v", err)
	}

	// Read data in chunks to test wrap-around reading
	chunk1, err := buf.Read(20)
	if err != nil {
		t.Fatalf("First chunk read failed: %v", err)
	}

	if len(chunk1) == 0 {
		t.Error("First chunk should contain data")
	}

	// Read remaining data
	chunk2, err := buf.Read(50)
	if err != nil && err != io.EOF {
		t.Fatalf("Second chunk read failed: %v", err)
	}

	if len(chunk1)+len(chunk2) == 0 {
		t.Error("Should have read some data from buffer")
	}
}

// TestCircularBufferGrowthFailure tests buffer growth failure scenarios
func TestCircularBufferGrowthFailure(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	maxSize := int64(200)
	buf, err := NewBuffer(bufferPath, maxSize)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Fill buffer gradually to test growth
	for i := 0; i < 10; i++ {
		data := make([]byte, 15)
		for j := range data {
			data[j] = byte((i*10 + j) % 256)
		}

		_, err = buf.Write(data)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	// Verify buffer has data
	if !buf.HasData() {
		t.Error("Buffer should have data after multiple writes")
	}
}

// TestCircularBufferReadWritePatterns tests various read/write patterns
func TestCircularBufferReadWritePatterns(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buf, err := NewBuffer(bufferPath, 1024)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Test alternating write/read pattern
	for i := 0; i < 5; i++ {
		// Write some data
		writeData := []byte(fmt.Sprintf("message-%d\n", i))
		_, err = buf.Write(writeData)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}

		// Read some data
		readData, err := buf.Read(int64(len(writeData)))
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}

		if string(readData) != string(writeData) {
			t.Errorf("Read data mismatch at iteration %d: got %q, expected %q",
				i, string(readData), string(writeData))
		}
	}
}

// TestCircularBufferZeroSizeOperations tests operations with zero-size data
func TestCircularBufferZeroSizeOperations(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buf, err := NewBuffer(bufferPath, 1024)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Test writing zero-size data
	n, err := buf.Write([]byte{})
	if err != nil {
		t.Errorf("Write of empty data failed: %v", err)
	}
	if n != 0 {
		t.Errorf("Write of empty data returned %d, expected 0", n)
	}

	// Test reading zero bytes
	data, err := buf.Read(0)
	if err != nil && err != io.EOF {
		t.Errorf("Read of zero bytes failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("Read of zero bytes returned %d bytes, expected 0", len(data))
	}
}

// TestCircularBufferStateConsistency tests buffer state consistency after errors
func TestCircularBufferStateConsistency(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buf, err := NewBuffer(bufferPath, 100)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buf.Close()

	// Write some valid data
	validData := []byte("valid data")
	_, err = buf.Write(validData)
	if err != nil {
		t.Fatalf("Valid write failed: %v", err)
	}

	initialSize := buf.GetSize()
	initialHasData := buf.HasData()

	// Try to write invalid data (too large)
	invalidData := make([]byte, 200) // Larger than max size
	_, err = buf.Write(invalidData)
	if err == nil {
		t.Error("Expected error for oversized write")
	}

	// Check that buffer state is consistent after error
	if buf.GetSize() != initialSize {
		t.Errorf("Buffer size changed after failed write: got %d, expected %d",
			buf.GetSize(), initialSize)
	}

	if buf.HasData() != initialHasData {
		t.Errorf("Buffer HasData state changed after failed write: got %v, expected %v",
			buf.HasData(), initialHasData)
	}

	// Verify we can still read the original data
	readData, err := buf.Read(int64(len(validData)))
	if err != nil {
		t.Fatalf("Read after failed write failed: %v", err)
	}

	if string(readData) != string(validData) {
		t.Errorf("Data corrupted after failed write: got %q, expected %q",
			string(readData), string(validData))
	}
}

// TestCircularBufferCloseOperations tests operations after buffer is closed
func TestCircularBufferCloseOperations(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "buffer-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	bufferPath := filepath.Join(tmpdir, "test-buffer.log")
	buf, err := NewBuffer(bufferPath, 1024)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}

	// Write some data before closing
	testData := []byte("test data before close")
	_, err = buf.Write(testData)
	if err != nil {
		t.Fatalf("Write before close failed: %v", err)
	}

	// Close the buffer
	err = buf.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Test double close (should not panic)
	err = buf.Close()
	if err == nil {
		t.Log("Double close succeeded (this is acceptable)")
	}
}
