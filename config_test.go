package main

import (
	"errors"
	"flag"
	"os"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				CertFile: "cert.pem",
				Host:     "example.com",
				Port:     12345,
			},
			wantErr: false,
		},
		{
			name: "missing cert",
			config: Config{
				Host: "example.com",
				Port: 12345,
			},
			wantErr: true,
		},
		{
			name: "missing host",
			config: Config{
				CertFile: "cert.pem",
				Port:     12345,
			},
			wantErr: true,
		},
		{
			name: "missing port",
			config: Config{
				CertFile: "cert.pem",
				Host:     "example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: Config{
				CertFile: "cert.pem",
				Host:     "example.com",
				Port:     -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErr {
				if !errors.Is(err, ErrInvalidConfig) {
					t.Errorf("Config.Validate() error = %v, want error type %v", err, ErrInvalidConfig)
				}
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	// Save original arguments and fatal function
	oldArgs := os.Args
	oldFatal := CurrentLogFatal
	
	// Restore them when we're done
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		CurrentLogFatal = oldFatal
	}()

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		expected *Config
	}{
		{
			name:    "missing required flags",
			args:    []string{"cmd"},
			wantErr: true,
		},
		{
			name:    "missing port",
			args:    []string{"cmd", "-cert", "cert.pem", "-host", "example.com"},
			wantErr: true,
		},
		{
			name:    "missing host",
			args:    []string{"cmd", "-cert", "cert.pem", "-port", "12345"},
			wantErr: true,
		},
		{
			name:    "missing cert",
			args:    []string{"cmd", "-host", "example.com", "-port", "12345"},
			wantErr: true,
		},
		{
			name: "minimal valid config",
			args: []string{
				"cmd",
				"-cert", "cert.pem",
				"-host", "example.com",
				"-port", "12345",
			},
			expected: &Config{
				CertFile:    "cert.pem",
				Host:        "example.com",
				Port:        12345,
				ProgramName: "custom-logger",
				BufferPath:  "papertrail_buffer.log",
				MaxSize:     DefaultMaxSize,
				ShowVersion: false,
			},
		},
		{
			name: "full config",
			args: []string{
				"cmd",
				"-cert", "cert.pem",
				"-host", "example.com",
				"-port", "12345",
				"-program", "myapp",
				"-buffer", "/var/log/buffer.log",
				"-maxsize", "1048576",
			},
			expected: &Config{
				CertFile:    "cert.pem",
				Host:        "example.com",
				Port:        12345,
				ProgramName: "myapp",
				BufferPath:  "/var/log/buffer.log",
				MaxSize:     1048576,
				ShowVersion: false,
			},
		},
		{
			name: "version flag",
			args: []string{
				"cmd",
				"-version",
			},
			expected: &Config{
				CertFile:    "",
				Host:        "",
				Port:        0,
				ProgramName: "custom-logger",
				BufferPath:  "papertrail_buffer.log",
				MaxSize:     DefaultMaxSize,
				ShowVersion: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset flags
			flag.CommandLine = flag.NewFlagSet(tc.args[0], flag.ContinueOnError)

			// Set up a mock fatal function
			var exitCalled bool
			CurrentLogFatal = func(v ...interface{}) {
				exitCalled = true
			}

			// Set args and parse
			os.Args = tc.args
			config := ParseFlags()

			if tc.wantErr {
				if !exitCalled {
					t.Error("expected ParseFlags to exit on error, but it didn't")
				}
				return
			}

			if exitCalled {
				t.Fatalf("ParseFlags exited unexpectedly")
			}

			// Compare config fields
			if config.CertFile != tc.expected.CertFile {
				t.Errorf("CertFile = %q, want %q", config.CertFile, tc.expected.CertFile)
			}
			if config.Host != tc.expected.Host {
				t.Errorf("Host = %q, want %q", config.Host, tc.expected.Host)
			}
			if config.Port != tc.expected.Port {
				t.Errorf("Port = %d, want %d", config.Port, tc.expected.Port)
			}
			if config.ProgramName != tc.expected.ProgramName {
				t.Errorf("ProgramName = %q, want %q", config.ProgramName, tc.expected.ProgramName)
			}
			if config.BufferPath != tc.expected.BufferPath {
				t.Errorf("BufferPath = %q, want %q", config.BufferPath, tc.expected.BufferPath)
			}
			if config.MaxSize != tc.expected.MaxSize {
				t.Errorf("MaxSize = %d, want %d", config.MaxSize, tc.expected.MaxSize)
			}
			if config.ShowVersion != tc.expected.ShowVersion {
				t.Errorf("ShowVersion = %t, want %t", config.ShowVersion, tc.expected.ShowVersion)
			}
		})
	}
}