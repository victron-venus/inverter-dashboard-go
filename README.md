# Inverter Dashboard (Go)

[![Docker Hub](https://img.shields.io/docker/v/alvit/inverter-dashboard-go?label=Docker%20Hub&logo=docker)](https://hub.docker.com/r/alvit/inverter-dashboard-go)
[![Docker Pulls](https://img.shields.io/docker/pulls/alvit/inverter-dashboard-go?label=Docker%20Pulls&logo=docker)](https://hub.docker.com/r/alvit/inverter-dashboard-go)
[![CI](https://github.com/victron-venus/inverter-dashboard-go/actions/workflows/ci.yml/badge.svg)](https://github.com/victron-venus/inverter-dashboard-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![GitHub stars](https://img.shields.io/github/stars/victron-venus/inverter-dashboard-go)](https://github.com/victron-venus/inverter-dashboard-go/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/victron-venus/inverter-dashboard-go)](https://github.com/victron-venus/inverter-dashboard-go/network/members)
[![GitHub last commit](https://img.shields.io/github/last-commit/victron-venus/inverter-dashboard-go)](https://github.com/victron-venus/inverter-dashboard-go/commits/main)
[![Maintenance](https://img.shields.io/badge/Maintained%3F-yes-green.svg)](https://github.com/victron-venus/inverter-dashboard-go/graphs/commit-activity)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://go.dev/)

Remote web dashboard for Victron inverter control, rewritten in Go for better performance and deployment.

The original **[inverter-dashboard](https://github.com/victron-venus/inverter-dashboard)** stack is Python/FastAPI (`alvit/inverter-dashboard` on Docker Hub). This repository ships the same front-end assets as a **single binary** and publishes **`alvit/inverter-dashboard-go`**.

## Features

- **Real-time monitoring** of solar, grid, battery, and consumption
- **WebSocket-based live updates** with automatic reconnection
- **MQTT integration** for communication with Victron Cerbo GX
- **Optional Home Assistant** direct integration for enhanced control
- **Multiple device support**: batteries, EV charging, water management
- **Appliance monitoring**: washer, dryer, dishwasher status
- **Dark/light theme** support with local preference storage
- **Cross-platform** binaries for easy deployment
- **Chart visualization** with uPlot for power flow history

## Quick Start

### Prerequisites

- Victron Cerbo GX with MQTT enabled (or standalone MQTT broker)
- Home Assistant (optional, for enhanced features)
- Go 1.22+ (for building from source)

### Installation

#### Using Pre-built Binaries

Download the appropriate binary for your platform from the Releases page:

- **macOS Silicon** (M1/M2/M3): `inverter-dashboard-macos-silicon`
- **macOS Intel** (x86_64): `inverter-dashboard-macos-intel`
- **Linux ARM64** (aarch64): `inverter-dashboard-linux-arm64`
- **Raspberry Pi 3** (ARMv7): `inverter-dashboard-raspberry-pi3`
- **Linux AMD64** (x86_64): `inverter-dashboard-linux-amd64`

```bash
# Download (example for Raspberry Pi)
wget https://github.com/victron-venus/inverter-dashboard-go/releases/latest/download/inverter-dashboard-raspberry-pi3

# Make executable
chmod +x inverter-dashboard-raspberry-pi3

# Run
./inverter-dashboard-raspberry-pi3

# Access the dashboard
open http://localhost:8080
```

#### Building from Source

```bash
git clone https://github.com/victron-venus/inverter-dashboard-go.git
cd inverter-dashboard-go

# Copy and configure
cp config/config.yaml.sample config/config.yaml
# Edit config/config.yaml with your settings

# Build
go build -o inverter-dashboard .

# Run
./inverter-dashboard
```

### Docker Deployment

```bash
# Build image
docker build -t inverter-dashboard .

# Run container
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config:/app/config \
  --name inverter-dashboard \
  inverter-dashboard
```

Docker Compose (see `docker-compose.yml` in the repo; CI pushes `alvit/inverter-dashboard-go` to Docker Hub on `main` and version tags):

```yaml
version: '3.8'
services:
  inverter-dashboard:
    image: alvit/inverter-dashboard-go:latest
    container_name: inverter-dashboard
    ports:
      - "8080:8080"
    volumes:
      - ./config:/app/config
    environment:
      - MQTT_HOST=192.168.160.150
    restart: unless-stopped
```

## Configuration

Copy the sample configuration and edit with your settings:

```bash
cp config/config.yaml.sample config/config.yaml
```

### MQTT Configuration

```yaml
mqtt:
  host: "192.168.160.150"  # Your MQTT broker IP
  port: 1883               # MQTT port (usually 1883)
```

### Web Server Configuration

```yaml
web:
  port: 8080               # Web dashboard port
  host: "0.0.0.0"          # Bind address
```

### Home Assistant Integration (Optional)

Enable direct Home Assistant integration for enhanced control:

```yaml
homeassistant:
  url: "http://192.168.151.21:8123"  # Your HA URL
  token: "YOUR_LONG_LIVED_TOKEN"     # From HA Profile > Long-Lived Tokens
  direct_controls: true               # Use HA instead of MQTT
  poll_interval_seconds: 12           # How often to poll HA

  # Boolean entities (on/off switches)
  boolean_entities:
    only_charging: "input_boolean.only_charging"
    # ... more entities

  # Switch entities for home automation
  switch_entities:
    home_recliner: ["switch.recliner_recliner", "Recliner"]
    # ... more entities
```

## Environment Variables

You can override configuration using environment variables:

- `MQTT_HOST` - MQTT broker host
- `MQTT_PORT` - MQTT broker port
- `WEB_PORT` - Web server port
- `HA_URL` - Home Assistant URL
- `HA_TOKEN` - Home Assistant token

### Example

```bash
export MQTT_HOST=192.168.1.100
export WEB_PORT=9090
./inverter-dashboard
```

## HTTP API Endpoints

### `GET /` - Dashboard UI

Serves the main dashboard page.

### `GET /api/state` - Health Check

Returns minimal state for monitoring/health checks:

```json
{
  "ok": true,
  "dashboard_version": "v1.0.0",
  "control_version": "v1.2.3",
  "has_mqtt_state": true
}
```

### `POST /api/check-update` - Check for Updates

Checks GitHub for new versions:

```json
{
  "current": "v1.0.0",
  "latest": "v1.1.0"
}
```

### WebSocket Endpoint: `/ws`

Real-time communication with Vue.js frontend.

## MQTT Topics

### Subscribed Topics

- `inverter/state` - Main inverter state updates
- `inverter/console` - Console logs

### Published Topics (Commands)

- `inverter/cmd/toggle` - Toggle entity state
- `inverter/cmd/press` - Press button entity
- `inverter/cmd/setpoint` - Set power setpoint
- `inverter/cmd/dry_run` - Toggle dry run mode
- `inverter/cmd/limits` - Set min/max limits
- `inverter/cmd/ess_mode` - Cycle ESS mode
- `inverter/cmd/loop_interval` - Set update interval

### Command Payload Format

```json
{
  "action": "toggle",
  "entity": "switch.example"
}
```

## Development

### Project Structure

```
inverter-dashboard-go/
├── main.go                  # Entry point
├── internal/
│   ├── config/              # Configuration loader
│   ├── mqtt/                # MQTT client
│   ├── homeassistant/       # HA REST API client
│   ├── websocket/           # WebSocket handler
│   ├── html/                # HTML templates
│   └── version/             # Version management
├── config/
│   ├── config.yaml.sample   # Configuration sample
│   └── config.yaml          # Your configuration (gitignored)
├── go.mod                   # Go module file
├── go.sum                   # Go checksums
└── README.md                # This file
```

### Building

```bash
# Install dependencies
go mod download

# Build
go build -o inverter-dashboard .

# Run with live reload during development
air
```

### Testing

```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/mqtt/...
```

## Architecture

The Go rewrite improves upon the original Python version by:

1. **Performance**: Lower memory usage and faster response times
2. **Single Binary**: Easy deployment without Python dependencies
3. **Type Safety**: Go's static typing catches errors at compile time
4. **Concurrent Safety**: Proper mutex usage and goroutine management
5. **Better Error Handling**: Structured error handling throughout
6. **Configuration Management**: YAML-based configuration with validation

### Communication Flow

1. **MQTT Client** subscribes to inverter topics and maintains state
2. **Web Server** serves dashboard and handles API requests
3. **WebSocket Handler** broadcasts real-time updates to connected browsers
4. **Home Assistant Client** (optional) polls HA for additional entity states
5. **State Merger** combines MQTT and HA states for unified view

## Troubleshooting

### MQTT Connection Issues

Check if MQTT broker is reachable:

```bash
mosquitto_sub -h 192.168.160.150 -t "inverter/state" -v
```

### WebSocket Connection Issues

Open browser DevTools console to see WebSocket connection status. Check that:

- Port 8080 is accessible
- No firewall blocking WebSocket connections
- Correct protocol (ws:// for http, wss:// for https)

### Home Assistant Connection

Test HA API manually:

```bash
curl -H "Authorization: Bearer <TOKEN>" \
     http://192.168.151.21:8123/api/states/input_boolean.only_charging
```

### Checking Logs

```bash
# With JSON format (default)
./inverter-dashboard

# With text format
./inverter-dashboard -logging.format=text

# Debug logging
./inverter-dashboard -logging.level=debug
```

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature-name`
3. Commit your changes: `git commit -am 'Add new feature'`
4. Push to the branch: `git push origin feature-name`
5. Create a Pull Request

## Related projects

This project is part of the same Victron Venus OS integration suite as the Python dashboard:

| Project | Description |
|---------|-------------|
| [inverter-dashboard](https://github.com/victron-venus/inverter-dashboard) | Original Python/FastAPI dashboard (`alvit/inverter-dashboard` on Docker Hub) |
| **inverter-dashboard-go** (this repository) | Go implementation: single binary, same web UI, `alvit/inverter-dashboard-go` on Docker Hub |
| [inverter-control](https://github.com/victron-venus/inverter-control) | ESS external control with optional web UI |
| [dbus-mqtt-battery](https://github.com/victron-venus/dbus-mqtt-battery) | MQTT to D-Bus bridge for BMS integration |
| [dbus-tasmota-pv](https://github.com/victron-venus/dbus-tasmota-pv) | Tasmota smart plug as PV inverter on D-Bus |
| [esphome-jbd-bms-mqtt](https://github.com/victron-venus/esphome-jbd-bms-mqtt) | ESP32 Bluetooth monitor for JBD BMS |
| [inverter-monitoring](https://github.com/victron-venus/inverter-monitoring) | Telegraf + InfluxDB + Grafana monitoring stack |

## Author

Created by [@4alvit](https://github.com/4alvit)

## License

MIT License - see [LICENSE](LICENSE).

## Acknowledgments

- Original Python project: [victron-venus/inverter-dashboard](https://github.com/victron-venus/inverter-dashboard)
- Victron Energy for the Cerbo GX platform
- Vue.js and Home Assistant communities

## Support

For issues and feature requests, please use the GitHub issue tracker.

---

**Note:** This is a community project and is not affiliated with Victron Energy.
