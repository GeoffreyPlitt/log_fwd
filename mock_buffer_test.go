package main

import (
	"io"
	"testing"
)

func TestMockBuffer(t *testing.T) {
	// Create a new mock buffer
	buffer := NewMockBuffer()
	
	// Test that it starts empty
	if buffer.HasData() {
		t.Error("New buffer should be empty")
	}
	
	// Test Write method
	testData := []byte("test data")
	n, err := buffer.Write(testData)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned %d, expected %d", n, len(testData))
	}
	
	// Test HasData after write
	if !buffer.HasData() {
		t.Error("Buffer should have data after write")
	}
	
	// Test GetContents method
	contents := buffer.GetContents()
	if string(contents) != string(testData) {
		t.Errorf("GetContents returned %q, expected %q", string(contents), string(testData))
	}
	
	// Test Read method
	readData, err := buffer.Read(1024)
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if string(readData) != string(testData) {
		t.Errorf("Read returned %q, expected %q", string(readData), string(testData))
	}
	
	// Test HasData after read (should be empty)
	if buffer.HasData() {
		t.Error("Buffer should be empty after reading all data")
	}
	
	// Test Read from empty buffer (should return EOF)
	_, err = buffer.Read(1024)
	if err != io.EOF {
		t.Errorf("Read from empty buffer should return EOF, got %v", err)
	}
	
	// Test Reset method
	buffer.Write([]byte("more data"))
	if !buffer.HasData() {
		t.Error("Buffer should have data after second write")
	}
	
	buffer.Reset()
	if buffer.HasData() {
		t.Error("Buffer should be empty after Reset")
	}
	
	// Test Close method
	err = buffer.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
	
	// Operations on closed buffer should fail
	_, err = buffer.Write([]byte("after close"))
	if err != io.ErrClosedPipe {
		t.Errorf("Write after close should return ErrClosedPipe, got %v", err)
	}
	
	_, err = buffer.Read(1024)
	if err != io.ErrClosedPipe {
		t.Errorf("Read after close should return ErrClosedPipe, got %v", err)
	}
}