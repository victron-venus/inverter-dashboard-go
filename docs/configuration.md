# Configuration reference

[README](../README.md) · [Operations](operations.md) · [Troubleshooting](troubleshooting.md)

The supported example is [config.yaml.example](../config.yaml.example). Copy it to `config.yaml` in the server's working directory, protect it with mode 0600, and edit it before starting. There is no `--config` flag or automatic search of a `config/` subdirectory. The container's working directory is `/app` by default.

Do not replace a working installation's file with the example during an upgrade. In particular, preserve your own Home entity inventory, ordering, labels, disabled entries and credentials. Examples are not a list of devices to adopt.

## Configuration precedence

The order is field-specific:

1. Built-in defaults and environment variables initialize configuration.
2. `config.yaml` is loaded. Present MQTT fields override environment values; valid web/Cerbo fields also override their defaults. Home Assistant configuration comes from this file.
3. For the gateway block, explicitly set `GATEWAY_*` environment variables take precedence over YAML. Explicitly empty Access credential variables also override YAML credentials.
4. Non-empty/positive CLI overrides apply: `--mqtt-host`, `--mqtt-port`, `--port`. `--ssl-cert` and `--ssl-key` enable native TLS together.
5. Persisted MQTT and HA connection settings are applied after loading. Non-empty stored host/URL/token and positive MQTT port override earlier values at startup.

Connection settings need a process restart. UI visibility settings are read for subsequent state payloads. An empty saved connection value stops overriding the underlying configuration; it does not erase credentials from `config.yaml`.

An absent or invalid YAML file logs a warning and leaves environment/default configuration in effect. Unknown YAML fields are not a validation error. Do not assume that a misspelled key took effect. The historical default MQTT host is installation-specific; **always set your own source explicitly**. An empty `MQTT_HOST` environment string alone is not a reliable way to disable that default. Use `mqtt.host: ""` in YAML, or gateway-only environment configuration as described below.

## Telemetry sources

For native MQTT, set `mqtt.host`, `mqtt.port` (normally 1883), and `cerbo.portal_id`. These correspond to `MQTT_HOST`, `MQTT_PORT` and `CERBO_PORTAL_ID` before YAML/CLI overrides. The current native client uses the configured TCP broker; do not assume that HA or gateway credentials authenticate MQTT. Place that connection on an appropriate trusted network.

With no portal ID, the client can discover a portal from native traffic. A quiet broker cannot be discovered until such traffic arrives. Configuring the portal allows the initial read request immediately. Native subscriptions and commands remain scoped to the selected portal. Read-only keepalive and selected water `Mode` refresh requests run every 20 seconds.

For IGW, configure:

- `GATEWAY_ENABLED` / `gateway.enabled` — default false.
- `GATEWAY_URL` / `gateway.url` — HTTPS endpoint for inverter-gateway.
- `GATEWAY_API_TOKEN` / `gateway.api_token` — optional bearer credential required by your gateway policy.
- `GATEWAY_ACCESS_CLIENT_ID` and `GATEWAY_ACCESS_CLIENT_SECRET` / matching YAML keys — optional Cloudflare Access pair.
- `GATEWAY_POLL_INTERVAL_SECONDS` / `gateway.poll_interval_seconds` — default 2 seconds.

Gateway-only YAML uses `mqtt: {host: "", port: 0}` with an enabled, configured gateway. Environment-only gateway mode defaults MQTT to disabled when `GATEWAY_ENABLED=true` and no explicit MQTT host is supplied; do not leave a conflicting MQTT block in YAML. When both sources are configured, MQTT is preferred, IGW provides failover, and MQTT recovery is probed approximately every 60 seconds. TLS hostname/certificate verification remains enabled and redirects are rejected. A transport switch invalidates the old command/event generation.

## Native water and EV selection

The `cerbo` block selects native services:

- `tank_instance` / `WATER_TANK_INSTANCE`: default 21.
- `pump_instance` / `WATER_PUMP_INSTANCE`: default 1.
- `valve_instance` / `WATER_VALVE_INSTANCE`: default 2.
- `ev_instance` / `EV_INSTANCE` and `evcharger_instance` / `EVCHARGER_INSTANCE`: default -1 for discovery; zero or another nonnegative value pins an exact instance.

Match these to your installed services. Water and EV telemetry come from the native device services; setting arbitrary HA switches does not create equivalent native controls. Unsupported or stale controls must remain unavailable.

## Home Assistant and Home buttons

`homeassistant.url` and `homeassistant.token` configure the direct REST integration. `poll_interval_seconds` defaults to 12. Non-finite, nonpositive, too-small or overflowing intervals fall back to 12 seconds. `direct_controls` defaults to true when omitted; set it false if this integration should only provide observations. This flag is not a global read-only mode for the inverter.

Use an explicit ordered `switch_entities` list:

```yaml
homeassistant:
  url: "https://ha.example.net"
  token: "" # configure privately before use
  direct_controls: false
  poll_interval_seconds: 12
  switch_entities:
    - key: home_example
      entity: switch.example
      label: Example
      order: 10
      enabled: true
```

`key` identifies the state field; keep it stable and unique. The configured label is shown without stripping words. Entries sort by `order`; equal orders in an explicit list retain declaration order. Mapping syntax is also supported, but an explicit list makes ordering easier to review. Disabled entries are excluded from Home display, polling and toggle permission. An empty list creates no Home buttons. Unknown/unavailable state must not be treated as a known off value. Home controls require current successful HA observations, not merely a configured URL/token or a connection badge.

Additional optional fields:

- `boolean_entities`: legacy mapping for named controller flags. Use it only for an existing installation that needs that mapping; it is not an automatic Home inventory.
- `appliance_entities`: map recognized appliance readings such as `washer_time`, `washer_power`, `dryer_time`, `dryer_power`, `dishwasher_running`, `dishwasher_duration`, `dishwasher_power` to your sensors. Appliance display requires both the visibility preference and an active/running state.
- `vue_sensors`: display label to entity-ID mapping for additional readings.
- `filtered_entities.covers`, `media_players`, `scenes`, `numbers`, `sensors`: explicit lists; `filtered_entities.weather`: one entity ID. Rich actions remain constrained to configured targets and fresh observations. Number actions also honor observed minimum/maximum metadata.

Never add another user's personal entity IDs to a default file. Changing labels is different from changing the underlying entity or native device instance.

## HTTP, authentication and storage

`web.host` defaults to `0.0.0.0`; the example deliberately uses `127.0.0.1`. `web.port` / `WEB_PORT` defaults to 8080. `--port` is a final startup override. Configure ingress and firewall rules for the intended audience.

`DASHBOARD_SECRET` enables shared-secret authentication. With it set, requests can use `Authorization: Bearer …`; the dashboard/WebSocket entry path also supports a token query parameter. Query credentials may appear in browser history or proxy access logs, so prefer headers for API clients and protect/redact logs. An unset secret disables this authentication. There is no role-based user database.

`INVERTER_DASHBOARD_SETTINGS_FILE` sets an explicit settings-file path. Otherwise the file is `dashboard_settings.json` in the working directory. New explicit parent directories are created with mode 0700 and writes use private temporary files, atomic replacement and durability checks. The file may contain a saved HA token: treat it as a secret, even though API responses mask that token. Existing parent directories must already have suitable ownership and protection.

Visibility keys include `show_ev`, `show_washer`, `show_dryer`, `show_dishwasher`, `show_home_section`, `show_ha_covers`, `show_ha_media`, `show_ha_scenes`, `show_ha_weather`, `show_daily_stats`, `show_header_toggles`, `show_batteries`, `show_solar_production`, `show_active_loads`, `show_ha_sensors`, `show_ha_numbers`. `CAMERA_TOPIC` supplies the initial camera topic; a saved preference can override it.

`SELF_UPDATE_ENABLED` is false unless explicitly set to `true`. Keep it disabled in immutable container deployments and replace the verified image instead. `LOG_FORMAT=text` selects human-readable structured logging; otherwise the structured logger uses JSON. A YAML `logging` or `ui` block does not configure this Go loader.

## Web Push

`WEB_PUSH_ENABLED` defaults to false. Enabling it also requires an absolute, dedicated `WEB_PUSH_DATA_DIR` on durable storage. The default `WEB_PUSH_SUBJECT` is the HTTPS project contact URL; a replacement must be a valid HTTPS contact URL.

Use separate children, for example `/var/lib/inverter-dashboard/settings/` and `/var/lib/inverter-dashboard/notifications/`, for settings and the private push store. Do not share one push store between multiple processes, Python and Go servers, or independent installations. Do not delete it to troubleshoot a delivery problem: it contains the application identity and subscription capabilities. See [Web Push](web-push.md) for platform limits, filesystem permissions and delivery policy.
