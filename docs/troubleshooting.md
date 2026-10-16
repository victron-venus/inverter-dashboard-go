# Troubleshooting without operating equipment

[README](../README.md) · [Configuration](configuration.md) · [Operations](operations.md)

Begin with version, configuration location, source connectivity and freshness. Avoid commands to equipment, live alarm acknowledgements, raw state dumps and unredacted configuration/log exports. Configuration and state can disclose credentials, entity identities and private network details.

## The page does not open

Check the listening host/port, service status, DNS route and HTTPS certificate. The example binds `127.0.0.1`, while an unconfigured binary defaults to all interfaces. VPN routes can direct a valid hostname to an unreachable network even when the service is healthy. Compare an authorized health check through the intended route; do not disable TLS verification or change cluster/network policy as a first guess.

For a local HTTP installation, the binary's `healthcheck` subcommand performs `GET /health` on `127.0.0.1`, using `WEB_PORT` or 8080. A custom `--port` or native-TLS setup needs a correspondingly configured probe. `/health` proves HTTP service health, not fresh inverter or HA data.

A 401 after enabling `DASHBOARD_SECRET` is an authentication issue, not an MQTT issue. Use the supported authenticated entry/API path and keep tokens out of shared URLs and logs. A 403 on browser mutations can mean an Origin/Host mismatch; correct the reverse proxy to preserve the public Host.

## The page opens, but readings or controls are unavailable

Confirm the selected source and original observation times. A quiet MQTT broker needs an explicit `cerbo.portal_id` to bootstrap reads. Check your portal and native instance selection against your own installation. Do not assume default tank/pump/valve instances match every site.

For IGW, check its configured HTTPS URL, certificate trust, credentials and enabled flag. If both transports are configured, MQTT remains preferred; gateway-only mode needs an explicitly empty YAML MQTT host or an environment-only configuration without conflicting YAML. Follow [configuration precedence](configuration.md#configuration-precedence).

Retained, malformed or old observations can still leave some display context while controls remain disabled. Waiting for a current supported observation is correct behavior. A transport badge or recent update to another field does not authorize all actions. Do not remove freshness checks to enable a button.

## Home buttons are absent or all disabled

An empty `switch_entities` mapping (`{}`) intentionally produces no Home buttons. Confirm that the process reads the intended `config.yaml`, then inspect only your own `switch_entities` entries: stable unique keys, actual entity IDs, labels, order and enabled flags. The public example is not your device list.

For disabled buttons, distinguish unknown state from off. Check HA URL/token validity, selected entity availability, `direct_controls` and recent successful observations. A configuration-present HA flag is not a successful entity read. Preserve existing credentials and inventory during updates; don't replace them wholesale to make the UI look populated.

Appliances appear only when enabled for display and actively running. A shared power meter is not necessarily an individual appliance measurement. Configure the correct readings rather than assuming any nonzero household power proves a washer/dryer is running.

## A setting did not survive restart

Check `INVERTER_DASHBOARD_SETTINGS_FILE`, the working directory, service-account permissions and durable volume mapping. A read-only Secret mount cannot accept settings writes; an ephemeral directory is lost on replacement. A reported save failure is real even if a failed durability check happened after file replacement. Re-read current settings before retrying and preserve the existing file.

MQTT/HA connection overrides require restart; visibility preferences do not. The API masks saved HA tokens. Sending the displayed mask back should preserve the real token, not replace it. Never attach the private settings file to an issue.

## An alarm looks old, or no system notification appears

Read the exact occurrence time/timezone. Reconnects preserve the original source time, and unknown timestamps stay unknown. An inactive unacknowledged alarm may remain relevant. Closing/hiding local UI is not proof of acknowledgement or resolution.

Web Push deliberately suppresses startup/reconnect history and expired events. An old on-screen alarm is not evidence that a new push should be emitted. Check server push availability, browser HTTPS support, permission, per-browser category preferences and OS notification settings. Windows servers do not currently support the private persistent push store.

For an explicitly requested test, distinguish three outcomes: API 202 means queued; a sender log with provider 2xx means provider acceptance; a visible OS notification is separate evidence. Inspect only bounded delivery fields (kind, attempt, status, fixed error category); do not expose endpoints, keys, payloads or raw provider bodies. Test limits are one per endpoint per minute and ten per minute globally. Repeated test clicks are not a substitute for diagnosing the first attempt.

Store-lock, permissions or corruption errors require preserving the private store and inspecting ownership/single-writer deployment. Deleting it changes the push identity and can invalidate subscriptions. See [Web Push](web-push.md) for supported provider hosts and network restrictions.

## UI text or charts differ from source screenshots

Check [the embedded frontend receipt](../internal/html/vue-ui-source.json). The source repository can be ahead of the released UI inside this binary. The presence of date-library locale files does not prove all UI labels are translated. Browser-local preferences, timezone, actual device data and supported controller capabilities also affect the display.

Missing grid-energy history or coverage is not zero usage. Verify upstream history availability and timezone/day boundaries before comparing daily totals. Do not manufacture complete intervals in the dashboard to fill a gap.

## Report a reproducible problem

Include the Go package version/source, embedded Vue version, OS/architecture, selected transport, sanitized error category/status, exact steps and expected result. State which checks were read-only and which dependencies were mocked. Redact all tokens, cookies, push endpoints/keys, private hosts and personal entity inventories. Report confidential security issues via [SECURITY.md](../SECURITY.md), not a public issue.
