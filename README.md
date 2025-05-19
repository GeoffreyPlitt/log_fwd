# papertrail_fwd

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
go install github.com/yourusername/papertrail_fwd@latest
```

Or clone and build:

```bash
git clone https://github.com/yourusername/papertrail_fwd.git
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

## Testing

Run the tests with:

```bash
go test -v ./...
```

For test coverage:

```bash
go test -cover ./...
```

## Code Linting

Run Go's standard code linting:

```bash
go vet ./...
```

## License

MIT