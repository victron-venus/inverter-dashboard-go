# Configuration

The canonical reference is [docs/configuration.md](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/configuration.md), with a safe [config.yaml.example](https://github.com/victron-venus/inverter-dashboard-go/blob/main/config.yaml.example).

The server reads `config.yaml` from its working directory. There is no `--config` flag. The container works in `/app`, so mount the file at `/app/config.yaml` unless changing the working directory explicitly. Review the example and supply real values before starting; its domains are placeholders.

## Sources and precedence

Configure MQTT host/port and an explicit Cerbo portal ID, or configure an HTTPS gateway. An empty YAML MQTT host plus enabled IGW selects gateway-only operation. With both configured, MQTT is preferred and IGW provides failover. Source changes invalidate old control permission.

Precedence is field-specific: ordinary YAML MQTT/web values override their defaults; explicit `GATEWAY_*` environment variables override the gateway YAML block. CLI overrides follow, and saved MQTT/HA connection settings apply last at startup. Connection changes need a restart.

## Your Home inventory

Use a `homeassistant.switch_entities` mapping keyed by stable names, with an entity ID, label, order and enabled flag for each entry. An empty mapping (`{}`) means no Home buttons. Preserve the installation's existing entries and credentials during updates. Do not copy another site's personal device IDs or infer that an unknown entity is off. HA controls need configured targets and fresh successful observations.

## Private persistence

Set `INVERTER_DASHBOARD_SETTINGS_FILE` to a persistent writable file path. It can contain a saved HA token, even though API responses mask that value. New settings directories use private permissions; existing parents must be protected by the operator.

If Web Push is enabled, give `WEB_PUSH_DATA_DIR` its own absolute private directory on persistent storage, with exactly one writer. Do not share that store between Python and Go installations. Preserve identity and subscriptions when replacing the application. See [Notifications](Notifications.md) and the [operator guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/operations.md).
