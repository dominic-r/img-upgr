.PHONY: build test clean run fmt lint check-fmt check-mod help

# Variables
BINARY_NAME = img-upgr
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X gitlab.com/sdko-core/appli/img-upgr/pkg/version.Version=${VERSION} -X gitlab.com/sdko-core/appli/img-upgr/pkg/version.Commit=${COMMIT} -X gitlab.com/sdko-core/appli/img-upgr/pkg/version.BuildDate=${DATE}"

# Default target
all: build

build:
	@echo "Building ${BINARY_NAME}..."
	go build ${LDFLAGS} -o ${BINARY_NAME}

test:
	@echo "Running tests..."
	go test -v ./...

clean:
	@echo "Cleaning up..."
	rm -f ${BINARY_NAME}
	go clean

run:
	./${BINARY_NAME} check

# Code quality
fmt:
	go fmt ./...

lint:
	golangci-lint run --timeout 5m

check-fmt:
	@UNFORMATTED=$$(gofmt -l .) && \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files are not formatted correctly:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	else \
		echo "All files are formatted correctly."; \
	fi

check-mod:
	@go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "go.mod or go.sum are not tidy. Run 'go mod tidy' locally and commit changes." && exit 1)

help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  test        - Run tests"
	@echo "  clean       - Remove build artifacts"
	@echo "  run         - Run the application"
	@echo "  fmt         - Format Go code"
	@echo "  lint        - Run golangci-lint"
	@echo "  check-fmt   - Check code formatting"
	@echo "  check-mod   - Check if go.mod and go.sum are tidy"
	@echo "  help        - Show this help message"