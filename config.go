package main

import (
	"flag"
	"log"
	"time"
)

const (
	DefaultMaxSize    = 100 * 1024 * 1024 // 100MB default
	ReconnectTimeout  = 60 * time.Second
	ReadChunkSize     = 4096               // 4KB read chunks
	InitialBufferSize = 64 * 1024          // Start with 64KB
)

// Config holds all program configuration
type Config struct {
	CertFile    string
	Host        string
	Port        int
	ProgramName string
	BufferPath  string
	MaxSize     int64
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
	
	flag.StringVar(&config.CertFile, "cert", "", "Path to Papertrail certificate bundle")
	flag.StringVar(&config.Host, "host", "", "Papertrail host")
	flag.IntVar(&config.Port, "port", 0, "Papertrail port")
	flag.StringVar(&config.ProgramName, "program", "custom-logger", "Program name for log identification")
	flag.StringVar(&config.BufferPath, "buffer", "papertrail_buffer.log", "Path to buffer file")
	maxSize := flag.Int64("maxsize", DefaultMaxSize, "Maximum buffer size in bytes")
	flag.Parse()
	
	config.MaxSize = *maxSize
	
	// Validate required flags
	if config.CertFile == "" || config.Host == "" || config.Port == 0 {
		CurrentLogFatal("Certificate path, host, and port are required")
	}
	
	return config
}