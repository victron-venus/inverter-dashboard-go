# Deploy inverter-dashboard-go on k3s (node `mp`)

Pinned to Mac Pro worker via `nodeSelector: kubernetes.io/hostname: mp`.

Image: `alvit/inverter-dashboard-go:latest` (Docker Hub).

## Telemetry: IGW-only (default in these manifests)

Uses **HTTPS inverter-gateway** (`https://victron.2560801.xyz`) via
`GET /v1/snapshot` every ~2s with Cloudflare Access service-token headers +
Bearer `GATEWAY_API_TOKEN`.

- ConfigMap: `GATEWAY_ENABLED=true`, `GATEWAY_URL`, poll interval
- Secret `inverter-dashboard-go-gateway`: Access client id/secret + API token
- **No** `MQTT_HOST` / `MQTT_PORT` / `--mqtt-host` — this pod must not open a
  third Cerbo MQTT client (IGW already owns keepalive)

Commands: IGW whitelist only (`silence_alarm`, `acknowledge_all_notifications`).
`inverter/cmd/*` (setpoint/toggles) is not on IGW yet — telemetry-first is OK.

### Legacy Cerbo MQTT mode

Set `MQTT_HOST`/`MQTT_PORT` (and drop `GATEWAY_ENABLED`) if you intentionally
want a direct Cerbo broker client. Do **not** also run IGW + desktop + this
pod against Cerbo at once.

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
