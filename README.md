# Log Forwarder

[![Go Tests](https://github.com/GeoffreyPlitt/log_fwd/actions/workflows/go.yml/badge.svg)](https://github.com/GeoffreyPlitt/log_fwd/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/GeoffreyPlitt/log_fwd/branch/main/graph/badge.svg)](https://codecov.io/gh/GeoffreyPlitt/log_fwd)
[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/doc/go1.21)
[![Go Report Card](https://goreportcard.com/badge/github.com/GeoffreyPlitt/log_fwd)](https://goreportcard.com/report/github.com/GeoffreyPlitt/log_fwd)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

This program forwards logs to log management services using HTTP APIs. It reads data from stdin, stores the logs in a circular buffer file, and sends them to the log service over HTTPS. It handles reconnections, buffering during disconnections, and supports various command-line arguments for configuration.

## Features

- Disk-based circular buffer for log persistence
- Automatic buffer growth as needed (up to configured maximum)
- Reconnection with exponential backoff and jitter
- HTTP API integration with JSON payload formatting
- Log batching for improved throughput
- Optional compression (gzip) for efficient transport
- Bearer token authentication with secure token handling
- Configurable retries and timeouts
- Clean shutdown on signal interrupts
- Efficient buffer management for large volumes of logs
- Panic recovery
- Modern Go standards and error handling

## Installation

Requires Go 1.21 or higher.

```bash
go install github.com/GeoffreyPlitt/log_fwd@latest
```

Or clone and build:

```bash
git clone https://github.com/GeoffreyPlitt/log_fwd.git
cd log_fwd
go build
```

## Usage

```bash
# Basic usage
some_program | ./log_fwd -host logs.example.com -token YOUR_API_TOKEN

# With all options
some_program | ./log_fwd \
  -cert /path/to/cert-bundle.pem \
  -host logs.example.com \
  -port 443 \
  -program "my-custom-app" \
  -buffer "/var/log/log_buffer.log" \
  -maxsize 209715200 \
  -token YOUR_API_TOKEN \
  -k
```

## Command-line options

| Option | Description | Default |
|--------|-------------|--------|
| `-cert` | Path to certificate bundle | (uses system certs) |
| `-host` | Log service host | (required) |
| `-port` | Log service port | 443 |
| `-program` | Program name for log identification | "custom-logger" |
| `-buffer` | Path to buffer file | "log_fwd_buffer.log" |
| `-maxsize` | Maximum buffer size in bytes | 100MB |
| `-token` | Authorization token | (required) |
| `-k` | Allow insecure SSL connections | false |
| `-batch` | Number of log entries to batch in a single request | 10 |
| `-enable-batch` | Enable log batching | true |
| `-retries` | Maximum number of retries for failed requests | 3 |
| `-timeout` | Overall HTTP client timeout | 30s |
| `-req-timeout` | Per-request timeout | 10s |
| `-compress` | Compress logs using gzip before sending | false |
| `-v` | Enable verbose debug logging | false |
| `-q`, `-quiet` | Quiet mode - don't echo log lines to stdout | false |

## Examples

### Basic Usage

```bash
# Forward application logs to BetterStack Logs (logs will also be printed to stdout)
tail -f /var/log/application.log | ./log_fwd -host logs.example.com -token YOUR_API_TOKEN

# Forward application logs in quiet mode (logs won't be echoed to stdout)
tail -f /var/log/application.log | ./log_fwd -q -host logs.example.com -token YOUR_API_TOKEN
```

### Advanced Usage

```bash
# Forward logs with batching, compression, and custom retry settings
tail -f /var/log/application.log | ./log_fwd \
  -host logs.example.com \
  -token YOUR_API_TOKEN \
  -batch 20 \
  -retries 5 \
  -compress \
  -timeout 60s \
  -req-timeout 15s

# High-throughput configuration for reliable delivery
tail -f /var/log/high-volume.log | ./log_fwd \
  -host logs.example.com \
  -token YOUR_API_TOKEN \
  -batch 50 \
  -compress \
  -buffer "/var/log/high_volume_buffer.log" \
  -maxsize 1073741824  # 1GB buffer
```

## Development

This project includes a Makefile to simplify common operations.

```bash
# Build the application
make build

# Install the application
make install

# Run tests
make test

# Run tests with race detection
make test-race

# Generate test coverage and open report
make cover

# Run linting
make lint

# Show all available commands
make help
```

### Testing

Run the tests with:

```bash
make test
```

Or manually:

```bash
go test -v ./...
```

For test coverage:

```bash
make cover
```

Or manually:

```bash
go test -race -coverprofile=coverage.txt -covermode=atomic ./...
go tool cover -html=coverage.txt -o coverage.html
```

### Continuous Integration

This project uses GitHub Actions for continuous integration:

- Automated tests run on every push to the main branch and on all pull requests
- Code coverage is tracked with Codecov
- Code quality is monitored with go vet

### Code Linting

This project uses Go's built-in `go vet` tool for linting to maintain code quality.

Run the linter:

```bash
make lint
```

This will run `go vet` to analyze your code and report potential issues.

## License

MIT