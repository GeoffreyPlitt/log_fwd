.PHONY: build test cover lint clean

# Build variables
BINARY_NAME=log_fwd
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
BUILD_TIME=$(shell date +%FT%T%z)
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

# Default target
all: build

# Build the application
build:
	go build ${LDFLAGS} -o ${BINARY_NAME}

# Install the application
install:
	go install ${LDFLAGS}

# Run all tests
test:
	go test -v ./...

# Run tests with race detection
test-race:
	go test -race -v ./...

# Generate test coverage
cover:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html

# Run code linting
lint:
	go vet ./...

# Clean build artifacts
clean:
	go clean
	rm -f ${BINARY_NAME}
	rm -f coverage.txt
	rm -f coverage.html

# Run the application (for quick testing)
run:
	go run .

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  install      - Install the application"
	@echo "  test         - Run tests"
	@echo "  test-race    - Run tests with race detection"
	@echo "  cover        - Generate and view test coverage"
	@echo "  lint         - Run code linting"
		@echo "  clean        - Clean build artifacts"
	@echo "  run          - Run the application"
	@echo "  help         - Show this help"