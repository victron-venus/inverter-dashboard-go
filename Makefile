.PHONY: build run test clean deps release

# Variables
BINARY_NAME=inverter-dashboard
DOCKER_IMAGE=ghcr.io/victron-venus/inverter-dashboard-go
VERSION=$(shell cat VERSION)
BUILD_FLAGS=-ldflags "-s -w -X main.Version=${VERSION}"

# Default target
all: build

# Install dependencies
deps:
	go mod download
	go mod tidy

# Build the binary
build: deps
	go build ${BUILD_FLAGS} -o ${BINARY_NAME} .

# Build for multiple platforms
release:
	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build ${BUILD_FLAGS} -o dist/${BINARY_NAME}-darwin-arm64 .

	# macOS AMD64 (Intel)
	GOOS=darwin GOARCH=amd64 go build ${BUILD_FLAGS} -o dist/${BINARY_NAME}-darwin-amd64 .

	# Linux ARM64
	GOOS=linux GOARCH=arm64 go build ${BUILD_FLAGS} -o dist/${BINARY_NAME}-linux-arm64 .

	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build ${BUILD_FLAGS} -o dist/${BINARY_NAME}-linux-amd64 .

	# Raspberry Pi 3 (ARMv7)
	GOOS=linux GOARCH=arm GOARM=7 go build ${BUILD_FLAGS} -o dist/${BINARY_NAME}-raspberry-pi3 .

	@echo "Binaries built successfully in dist/"

# Run the application
run: build
	./${BINARY_NAME}

# Run with live reload (requires air)
dev:
	air

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -f ${BINARY_NAME}
	rm -rf dist/
	rm -f coverage.out coverage.html

# Install the binary to /usr/local/bin
install: build
	sudo cp ${BINARY_NAME} /usr/local/bin/
	@echo "Installed to /usr/local/bin/${BINARY_NAME}"

# Uninstall the binary
uninstall:
	sudo rm -f /usr/local/bin/${BINARY_NAME}

# Run linter
lint:
	@which golangci-lint > /dev/null || echo "golangci-lint not found, install from https://golangci-lint.run/"
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...

# Build Docker image
docker-build:
	docker build -t ${DOCKER_IMAGE}:${VERSION} -t ${DOCKER_IMAGE}:latest .

# Run Docker container
docker-run: docker-build
	docker run -d \
		-p 8080:8080 \
		-v $(PWD)/config:/app/config \
		--name inverter-dashboard \
		${DOCKER_IMAGE}:latest

# Stop Docker container
docker-stop:
	docker stop inverter-dashboard || true
	docker rm inverter-dashboard || true

# Help
help:
	@echo "Available targets:"
	@echo "  make deps      - Install dependencies"
	@echo "  make build     - Build the binary"
	@echo "  make run       - Build and run"
	@echo "  make test      - Run tests"
	@echo "  make clean     - Clean build artifacts"
	@echo "  make release   - Build for all platforms"
	@echo "  make install   - Install to /usr/local/bin"
	@echo "  make lint      - Run linter"
	@echo "  make fmt       - Format code"
	@echo "  make help      - Show this help"
