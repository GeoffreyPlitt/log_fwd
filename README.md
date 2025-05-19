# papertrail_fwd --> Switching to HTTP 

[![Go Tests](https://github.com/GeoffreyPlitt/papertrail_fwd/actions/workflows/go.yml/badge.svg)](https://github.com/GeoffreyPlitt/papertrail_fwd/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/GeoffreyPlitt/papertrail_fwd/branch/main/graph/badge.svg)](https://codecov.io/gh/GeoffreyPlitt/papertrail_fwd)
[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/doc/go1.24)
[![Go Report Card](https://goreportcard.com/badge/github.com/GeoffreyPlitt/papertrail_fwd)](https://goreportcard.com/report/github.com/GeoffreyPlitt/papertrail_fwd)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

This program forwards logs to Papertrail (a log management service). It reads data from stdin, formats it as RFC5424 syslog messages, stores them in a circular buffer file, and sends them to Papertrail over TLS. It handles reconnections, buffering during disconnections, and supports command-line arguments for configuration.

## Features

- Disk-based circular buffer for log persistence
- Automatic buffer growth as needed (up to configured maximum)
- Reconnection with exponential backoff
- Properly formatted RFC5424 syslog messages
- Clean shutdown on signal interrupts
- Efficient buffer management for large volumes of logs
- Panic recovery
- Modern Go 1.24 standards and error handling

## Installation

Requires Go 1.24 or higher.

```bash
go install github.com/GeoffreyPlitt/papertrail_fwd@latest
```

Or clone and build:

```bash
git clone https://github.com/GeoffreyPlitt/papertrail_fwd.git
cd papertrail_fwd
go build
```

## Usage

```bash
# Basic usage
some_program | ./papertrail_fwd -cert /path/to/papertrail-bundle.pem -host logs.papertrailapp.com -port 12345

# With all options
some_program | ./papertrail_fwd \
  -cert /path/to/papertrail-bundle.pem \
  -host logs.papertrailapp.com \
  -port 12345 \
  -program "my-custom-app" \
  -buffer "/var/log/papertrail_buffer.log" \
  -maxsize 209715200
```

## Command-line options

| Option | Description | Default |
|--------|-------------|--------|
| `-cert` | Path to Papertrail certificate bundle | (required) |
| `-host` | Papertrail host | (required) |
| `-port` | Papertrail port | (required) |
| `-program` | Program name for log identification | "custom-logger" |
| `-buffer` | Path to buffer file | "papertrail_buffer.log" |
| `-maxsize` | Maximum buffer size in bytes | 100MB |

## Example

```bash
# Forward application logs to Papertrail
tail -f /var/log/application.log | ./papertrail_fwd -cert /etc/papertrail-bundle.pem -host logs.papertrailapp.com -port 12345 -program "my-application"
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
