.PHONY: build test clean run fmt lint check-fmt check-mod

# Variables
BINARY_NAME=img-upgr
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X gitlab.com/sdko-core/appli/img-upgr/pkg/version.Version=${VERSION} -X gitlab.com/sdko-core/appli/img-upgr/pkg/version.Commit=${COMMIT} -X gitlab.com/sdko-core/appli/img-upgr/pkg/version.BuildDate=${DATE}"

# Default target
all: build

# Build the binary
build:
	@echo "Building ${BINARY_NAME}..."
	go build ${LDFLAGS} -o ${BINARY_NAME}

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f ${BINARY_NAME}
	go clean

# Run the application with environment variables
run:
	./${BINARY_NAME} check

# Run with debug logging
run-debug:
	IMG_UPGR_LOG_LEVEL=DEBUG ./${BINARY_NAME} check -v

# Run scan with debug logging
scan-debug:
	@echo "Running ${BINARY_NAME} scan with debug logging..."
	IMG_UPGR_LOG_LEVEL=DEBUG ./${BINARY_NAME} scan -v

# CI task to scan and check for updates
ci-scan:
	@echo "Running CI scan..."
	@if [ -z "$$IMG_UPGR_SCANDIR" ]; then echo "ERROR: IMG_UPGR_SCANDIR is not set"; exit 1; fi
	./${BINARY_NAME} check "$$IMG_UPGR_SCANDIR" --fail-on-update

# CI task to scan and create MRs
ci-update:
	@echo "Running CI update with merge requests..."
	@if [ -z "$$IMG_UPGR_SCANDIR" ]; then echo "ERROR: IMG_UPGR_SCANDIR is not set"; exit 1; fi
	@if [ -z "$$IMG_UPGR_GL_USER" ]; then echo "ERROR: IMG_UPGR_GL_USER is not set"; exit 1; fi
	@if [ -z "$$IMG_UPGR_GL_TOKEN" ]; then echo "ERROR: IMG_UPGR_GL_TOKEN is not set"; exit 1; fi
	@if [ -z "$$IMG_UPGR_GL_REPO" ]; then echo "ERROR: IMG_UPGR_GL_REPO is not set"; exit 1; fi
	@if [ -z "$$IMG_UPGR_GL_EMAIL" ]; then echo "ERROR: IMG_UPGR_GL_EMAIL is not set"; exit 1; fi
	./${BINARY_NAME} scan --create-mr

# Format Go code
fmt:
	@echo "Formatting Go code..."
	go fmt ./...

# Run golangci-lint
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --timeout 5m

# Check if code is formatted correctly
check-fmt:
	@echo "Checking Go formatting..."
	@UNFORMATTED=$$(gofmt -l .) && \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files are not formatted correctly:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	else \
		echo "All files are formatted correctly."; \
	fi

# Check if go.mod and go.sum are tidy
check-mod:
	@echo "Checking go.mod and go.sum..."
	@go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum are not tidy. Run 'go mod tidy' locally and commit changes." && exit 1)

# Check environment variables
check-env:
	@echo "Checking environment variables..."
	@if [ -z "$$IMG_UPGR_SCANDIR" ]; then echo "WARNING: IMG_UPGR_SCANDIR is not set"; fi
	@if [ -z "$$IMG_UPGR_GL_USER" ]; then echo "WARNING: IMG_UPGR_GL_USER is not set"; fi
	@if [ -z "$$IMG_UPGR_GL_TOKEN" ]; then echo "WARNING: IMG_UPGR_GL_TOKEN is not set"; fi
	@if [ -z "$$IMG_UPGR_GL_REPO" ]; then echo "WARNING: IMG_UPGR_GL_REPO is not set"; fi
	@if [ -z "$$IMG_UPGR_GL_EMAIL" ]; then echo "WARNING: IMG_UPGR_GL_EMAIL is not set"; fi
	@echo "Log level: $${IMG_UPGR_LOG_LEVEL:-INFO}"

# Help target
help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  install    - Install the binary to GOPATH/bin"
	@echo "  test       - Run tests"
	@echo "  clean      - Remove build artifacts"
	@echo "  run        - Run the application"
	@echo "  run-debug   - Run with debug logging"
	@echo "  scan-debug  - Run scan with debug logging"
	@echo "  ci-scan     - Run in CI mode to check for updates"
	@echo "  ci-update   - Run in CI mode to create merge requests"
	@echo "  check-env   - Check environment variables"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Environment variables:"
	@echo "  IMG_UPGR_SCANDIR   - Directory to scan for docker-compose files"
	@echo "  IMG_UPGR_GL_USER   - GitLab bot username"
	@echo "  IMG_UPGR_GL_TOKEN  - GitLab personal access token"
	@echo "  IMG_UPGR_GL_REPO   - GitLab repository URL"
	@echo "  IMG_UPGR_GL_EMAIL  - Email for git commits"
	@echo "  IMG_UPGR_LOG_LEVEL - Log level (DEBUG, INFO, WARN, ERROR, FATAL)"