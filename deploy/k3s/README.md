# Deploy inverter-dashboard-go on k3s (node `mp`)

Pinned to Mac Pro worker via `nodeSelector: kubernetes.io/hostname: mp`.

Image: `alvit/inverter-dashboard-go:latest` (Docker Hub).

## Telemetry transport (code supports both; this deploy is IGW)

Binary policy (like inverter-desktop `connectionPolicy`):

1. `MQTT_HOST` set + broker reachable → **Cerbo MQTT** (preferred)
2. MQTT set but unreachable + gateway configured → **IGW failover** (+ ~60s MQTT probe-back)
3. No MQTT host + gateway configured → **IGW-only**
4. Neither → exit

### This k3s/mp deploy: IGW-only (unload Cerbo)

Uses native **HTTPS inverter-gateway** (`https://igw.s.2560801.xyz:9151`) via
`GET /v1/snapshot` every ~2s with Bearer `GATEWAY_API_TOKEN`.
The TLS certificate must be trusted and valid for the hostname. Pod DNS must
resolve `igw.s.2560801.xyz` to the IGW node; if cluster DNS does not forward the
LAN zone, use an installation-specific workload `hostAliases` entry while DNS is
configured. Keep the hostname in the URL so TLS identity is verified.

- ConfigMap: `GATEWAY_ENABLED=true`, `GATEWAY_URL`, poll interval
- Secret `inverter-dashboard-go-gateway`: API token
- Deployment explicitly empties the two Access environment values, overriding
  old credentials inherited from the Secret or mounted YAML.
- **No** `MQTT_HOST` / `MQTT_PORT` / `--mqtt-host` — pod does not dial Cerbo
  (IGW already owns keepalive). Set them only if you intentionally want
  MQTT-preferred dual-path on this node.

Banner ack/X: IGW `POST /v1/commands/acknowledge_all_notifications` (and
`silence_alarm`). On MQTT path the same WS actions publish Cerbo
`W/.../AcknowledgeAll` / `SilenceAlarm`.

`inverter/cmd/*` (setpoint/toggles) is not on IGW yet — telemetry-first is OK.

Both polling and commands reject all redirects, including same-origin and HTTPS
to HTTP redirects. `GATEWAY_URL` must be an absolute HTTPS URL without userinfo,
query, or fragment; existing path prefixes are supported. An HTTP URL now fails
client initialization before credentials can be sent. Configure the final HTTPS
endpoint directly rather than relying on an HTTP-to-HTTPS redirect.

For the Cloudflare-protected public endpoint `https://victron.2560801.xyz`, set a
complete Access client id/secret pair and override/remove the deployment's two
explicit empty Access environment values. Public HTTPS and the existing command
API remain supported. Any explicitly set `GATEWAY_*` environment value takes
precedence over mounted `config.yaml`, including empty credentials.

### Cerbo MQTT mode (non-k3s / LAN)

Set `MQTT_HOST`/`MQTT_PORT` (and optionally keep gateway as failover). Do **not**
run many concurrent Cerbo MQTT clients (desktop + IGW + this pod) unless slots allow.

## Ingress / DNS

Uses **Traefik on mp** (`ingressClassName: traefik-mp`, externalIP
`192.168.151.107`). See `4alvit/k3s-self-healing` → `deployments/00-traefik-mp/`.

- Host: `http://inverter-dashboard-go.mp.2560801.xyz`
- OpenWRT (one line): `address=/mp.2560801.xyz/192.168.151.107`

## Config Secret (optional)

Binary reads `/app/config.yaml` (WORKDIR). `02-secret.example.yaml` is
example-only — replace HA token out-of-band before enabling HA features:

```bash
kubectl -n inverter-dashboard-go create secret generic inverter-dashboard-go-config \
  --from-file=config.yaml=./config.yaml
```

Deployment mounts that Secret at `/app/config.yaml` (`optional: true`).

## Apply

```bash
# Review / replace 02-secret.example.yaml before prod HA use
kubectl --context k3s-heaven apply -k deploy/k3s
kubectl --context k3s-heaven -n inverter-dashboard-go get pods,ingress -o wide
# expect NODE=mp, class traefik-mp, host inverter-dashboard-go.mp.2560801.xyz
```

## Smoke

```bash
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H 'Host: inverter-dashboard-go.mp.2560801.xyz' \
  http://192.168.151.107/
curl -sS -H 'Host: inverter-dashboard-go.mp.2560801.xyz' \
  http://192.168.151.107/health
```

Do **not** confuse with Python `inverter-dashboard` (cluster Mosquitto) or
Tauri `inverter-desktop`.
