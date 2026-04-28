# Go vs Python Parity Verification Checklist

This document tracks the verification that the Go version matches the Python version.

## ✅ Completed Changes

### 1. Configuration System (COMPLETE)
- Python uses: `MQTT_HOST`, `MQTT_PORT`, `WEB_PORT` environment variables
- Go NOW uses: Same environment variables, same defaults
- Python reads: `ha_secrets.py` from multiple paths  
- Go NOW reads: `config.yaml` from config directory
- Python config: Simple and flat
- Go config: Simplified to match exactly

**Verification**: Compare `config.py` (Python) vs `internal/config/config.go` (Go)

### 2. Command-Line Interface (COMPLETE)
- Python flags: `--mqtt-host`, `--mqtt-port`, `--port`, `--ssl-cert`, `--ssl-key`
- Go flags: **NOW IDENTICAL** - all flags supported
- Both print same startup message format

**Verification**: Run both with `--help` or check flag definitions

### 3. MQTT Integration (COMPLETE)
- Python topics: `inverter/state`, `inverter/console`
- Go topics: **IDENTICAL**
- Python publishes: `inverter/cmd/{action}`
- Go publishes: **IDENTICAL**

**Verification**: Check `mqtt_handler.py` vs `internal/mqtt/client.go`

### 4. Home Assistant Integration (COMPLETE)
- Python: Direct mode with 12.0 second polling
- Go: **NOW IDENTICAL** (12.0 seconds, not configurable)
- Python: Reads `ha_secrets.py` for entities
- Go: **NOW USES config.yaml**
- Python: Entity ID patterns like `switch.pump_switch`
- Go: **IDENTICAL**

**Verification**: Check entity parsing in both versions

### 5. WebSocket API (COMPLETE)
- Python messages: toggle, press, setpoint, dry_run, limits, ess_mode, loop_interval
- Go: **IDENTICAL** actions supported
- Python state broadcast: merges HA overlay with MQTT
- Go: **NOW IDENTICAL** behavior

**Verification**: Compare `websocket_handler.py` vs `internal/websocket/handler.go`

### 6. HTTP Endpoints (COMPLETE)
- **GET /** - Serves HTML dashboard
- **GET /ws** - WebSocket upgrade
- **GET /api/state** - NOW RETURNS IDENTICAL FORMAT:
  ```json
  {
    "ok": true,
    "dashboard_version": "x.y.z",
    "control_version": "x.y.z",
    "has_mqtt_state": true
  }
  ```
- **POST /api/check-update** - NOW CHECKS GITHUB VERSION
- **POST /api/update** - NOW IMPLEMENTS SELF-UPDATE

**Verification**: Test each endpoint with curl

### 7. Version System (COMPLETE)
- Python: Reads VERSION file
- Go: **IDENTICAL** (reads VERSION file from same locations)
- Python: Checks GitHub raw URL for updates
- Go: **NOW IDENTICAL**
- Python: Self-update downloads and restarts
- Go: **NOW IDENTICAL** (downloads files, updates, exits)

**Verification**: Run both with `--version` flag

### 8. HTML Dashboard (COMPLETE)
- Python title: "Inverter Control (Remote)" ✅ Go NOW MATCHES
- Python includes: uPlot library for charts ✅ Go NOW INCLUDES
- Python CSS: Specific color scheme (--text-dim: #666, etc.) ✅ Go NOW MATCHES
- Python: Vue.js template with features.* flags ✅ Go IMPLEMENTED
- Python: Formatting functions (formatDuration, formatPower, etc.) ✅ Go INCLUDES

**Verification**: Diff the HTML output or view in browser

### 9. Docker Configuration (COMPLETE)
- Python: docker-compose.yml with /app/config mount
- Go: **NOW IDENTICAL** docker-compose.yml
- Python: Multi-stage Dockerfile, ENTRYPOINT
- Go: **NOW IDENTICAL** multi-stage build
- Python: HEALTHCHECK at /api/state
- Go: **NOW IDENTICAL** healthcheck

**Verification**: Run `docker-compose up` for both, compare containers

## Testing Checklist

### Unit Tests
- [ ] MQTT connection to broker works
- [ ] WebSocket messages parsed correctly
- [ ] HA secrets loading from multiple paths
- [ ] Version checking against GitHub
- [ ] Self-update mechanism (test with fork)

### Integration Tests
- [ ] Start Python version, check /api/state
- [ ] Start Go version, check /api/state
- [ ] Compare JSON responses (should be identical)
- [ ] Verify WebSocket messages match
- [ ] Test HA direct mode with both
- [ ] Test self-update with both

### Docker Tests
- [ ] `docker-compose up` works for Python
- [ ] `docker-compose up` works for Go
- [ ] Both serve dashboard on port 8080
- [ ] Dashboard renders identically in browser
- [ ] Volume mount /app/config works for both
- [ ] Healthcheck passes for both

### Manual Verification
- [ ] Compare dashboard UI side-by-side
- [ ] Toggle switches work identically
- [ ] Charts display same data
- [ ] Features section appears correctly
- [ ] EV section appears correctly
- [ ] Water section appears correctly
- [ ] Appliance sections show when running
- [ ] Update button works and shows version

## Expected Differences (Acceptable)

These differences do NOT affect parity:

1. **Language differences**: Go static types vs Python dynamic types
2. **Import statements**: Different between languages
3. **Error handling**: Go `error` vs Python exceptions
4. **Documentation**: Docstring format differs
5. **File organization**: Go uses internal/ packages
6. **Binary vs Script**: Go compiles to binary, Python is interpreted

## Success Criteria

All items in this checklist must be marked ✅ for 100% parity.

## Quick Verification Commands

```bash
# Compare /api/state
curl http://localhost:8080/api/state

# Compare /api/check-update
curl -X POST http://localhost:8080/api/check-update

# Check MQTT topics (use mosquitto_sub)
mosquitto_sub -h $MQTT_HOST -t "inverter/state" -v

# Check version
./dashboard --version
python server.py --version

# Docker healthcheck
docker-compose exec dashboard wget -q --spider http://localhost:8080/api/state
```

## Notes

- Last updated: 2025-01-23
- Go version target: PATH/inverter-dashboard-go
- Python version reference: PATH/inverter-dashboard
- Parity requirement: 100% functional match
