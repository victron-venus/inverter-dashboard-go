# Multi-stage build for Go binary - matches Python functionality
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o inverter-dashboard .

# Runtime stage - match Python slim image
FROM debian:bookworm-slim

# Install ca-certificates for HTTPS calls
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create app user and group (match Python non-root)
RUN groupadd -r app && useradd -r -g app app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/inverter-dashboard /app/inverter-dashboard

# Copy VERSION file
COPY VERSION /app/VERSION

# Create config directory (for config.yaml mount)
RUN mkdir -p /app/config && chown -R app:app /app

# Switch to non-root user
USER app

# Expose port
EXPOSE 8080

# Healthcheck - matches Python docker_healthcheck.py
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:8080/api/state || exit 1

# Default environment variables (match Python)
ENV MQTT_HOST=192.168.160.150
ENV MQTT_PORT=1883
ENV WEB_PORT=8080

# Run the dashboard
ENTRYPOINT ["/app/inverter-dashboard"]
