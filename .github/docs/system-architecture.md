# System Architecture

## Data Flow

```mermaid
flowchart LR
    subgraph Venus["Venus OS"]
        MQTT["MQTT Broker"]
        INV["inverter-control"]
    end

    subgraph Dashboard["inverter-dashboard-go"]
        WS["WebSocket"]
        API["HA Client\n(optional)"]
    end

    subgraph Client["Browser Clients"]
        UI["Single Page App"]
    end

    INV -->|"inverter/state"| MQTT
    MQTT -->|"subscribe"| WS
    WS -->|"push"| UI
    WS -.->|"poll HA"| API

    style WS fill:#00ADD8,color:#fff
```

## Binary Deployment

```mermaid
flowchart TB
    subgraph Build["CI/CD"]
        GOPROXY["Go Proxy"]
        BINARY["Darwin/Linux\nBinaries"]
    end

    subgraph Deploy["Deployment"]
        DOCKER["Docker Hub\nalvit/inverter-dashboard-go"]
        SYNO["Synology NAS\nDocker"]
    end

    subgraph Runtime["Runtime"]
        MQTT["MQTT Broker"]
        WS["WebSocket Server"]
    end

    GOPROXY --> BINARY --> DOCKER --> SYNO
    SYNO --> WS
    WS --> MQTT

    style WS fill:#00ADD8,color:#fff
    style BINARY fill:#00ADD8,color:#fff
    style DOCKER fill:#2496ED,color:#fff
```

## Runbook: Troubleshooting

### Docker Container Not Starting

**Symptoms:**
- Container exits immediately
- "exec format error"

**Actions:**
```bash
# Check architecture
uname -m
# Should match: amd64 or arm64

# Try specific tag
docker pull alvit/inverter-dashboard-go:amd64
docker run alvit/inverter-dashboard-go:amd64
```

### WebSocket Connection Issues

**Symptoms:**
- Dashboard shows connecting...
- WebSocket errors in browser console

**Actions:**
```bash
# Check MQTT connectivity from container
docker exec inverter-dashboard-go nc -zv MQTT_HOST 1883

# Verify binary is listening
docker exec inverter-dashboard-go ss -tlnp
```

### Out of Memory on NAS

**Symptoms:**
- Container OOM killed
- NAS alerts

**Actions:**
```bash
# Limit memory
docker run --memory=256m alvit/inverter-dashboard-go:latest

# Or in compose
services:
  dashboard:
    image: alvit/inverter-dashboard-go:latest
    mem_limit: 256m
```

---

## Related Documentation

- [inverter-control System Architecture](../inverter-control/.github/docs/system-architecture.md)
- [ADR-002: MQTT Bridge Architecture](../inverter-control/.github/docs/adr-001-grid-zero-architecture.md)
