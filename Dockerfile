# Multi-stage build for Go binary - matches Python functionality
FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

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
FROM debian:bookworm-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171

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

# Healthcheck - the runtime image has no wget/curl, so probe via the binary's
# built-in "healthcheck" subcommand (GET /health, honors WEB_PORT).
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD ["/app/inverter-dashboard", "healthcheck"]

# Default environment variables (match Python)
ENV MQTT_HOST=192.168.160.150
ENV MQTT_PORT=1883
ENV WEB_PORT=8080

# Run the dashboard
ENTRYPOINT ["/app/inverter-dashboard"]
