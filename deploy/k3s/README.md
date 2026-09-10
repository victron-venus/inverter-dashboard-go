# Deploy inverter-dashboard-go on k3s (node `mp`)

Pinned to Mac Pro worker via `nodeSelector: kubernetes.io/hostname: mp`.

Image: `alvit/inverter-dashboard-go:latest` (Docker Hub).

## MQTT

Uses **Cerbo LAN MQTT** `192.168.160.150:1883` (args + ConfigMap).

ConfigMap sets `CERBO_PORTAL_ID=b827ebea1ece` for keepalive + water/EV/alarms.
Live tiles (grid/battery/solar/loads) come from Cerbo MQTT wildcards; slim `inverter/state` only supplies daemon extras.

Do **not** point this deploy at in-cluster Mosquitto
(`mosquitto.homeassistant.svc.cluster.local`) — the Python
`inverter-dashboard` path already flaps on that broker.

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
