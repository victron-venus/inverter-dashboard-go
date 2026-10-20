# Operator guide

[README](../README.md) · [Configuration](configuration.md) · [Troubleshooting](troubleshooting.md)

## Bring up a source deliberately

Choose an explicit release/platform and verify its published identity before installing. Preserve the old executable or image and compatible private data for rollback. Review your source, portal, native instances, HA entity inventory, authentication and storage paths before starting a new service. The application is a control surface as well as a display.

Start from a directory with the intended `config.yaml`. A read-only configuration file is fine; the settings path and enabled push store must be writable by the service account. Keep secrets out of command history, screenshots, support bundles and repository files. Use your service manager's protected environment/configuration mechanism.

Read-only startup checks are sufficient for routine installation validation: confirm the running version, HTTP health, selected source, recent observed timestamps and expected configured labels. Do not “test connectivity” by changing a mode, opening a valve, toggling a Home switch or acknowledging an alarm.

## Containers and persistence

The [Dockerfile](../Dockerfile) runs as a non-root user with `/app` as its working directory. Mount the YAML file at `/app/config.yaml`. A mount at `/app/config/config.yaml` needs an explicit working-directory change to be read. The historical [Compose example](../docker-compose.yml) is not a complete production recipe: review its image identity, configuration destination, network exposure and persistence before using it.

For a reviewed deployment, configure these independently:

- Read-only `config.yaml` and private credential environment.
- `INVERTER_DASHBOARD_SETTINGS_FILE=/var/lib/inverter-dashboard/settings/dashboard_settings.json` on writable persistent storage.
- If push is enabled, `WEB_PUSH_DATA_DIR=/var/lib/inverter-dashboard/notifications` on persistent storage with the private modes described in [Web Push](web-push.md).
- One writer per push directory, with a replacement strategy such as Kubernetes `Recreate`. Do not solve a lock error by removing a live lock or starting another replica against the same data.
- Verified image identity, suitable ownership, HTTPS ingress and preserved external Host header.

A read-only Secret/ConfigMap cannot also hold mutable settings. An `emptyDir` or a container filesystem loses those settings when the pod/container is replaced. Do not copy private stores into images. Before a storage-path change, stop the writer and transfer existing data privately with ownership/mode preservation; verify it before discarding the old location.

The [k3s example guide](../deploy/k3s/README.md) describes repository manifests. Actual site deployment policy and credentials remain local. No manifest should include someone's private Home inventory as a universal default.

## What “available” means

A connected socket or HTTP 200 does not prove all measurements are current. Telemetry fields have their own validity; controller display availability and command authorization have different lifetimes. A configured HA connection flag is not proof that every entity was successfully observed.

Command gates use current source ownership and fresh observations, generally within 30 seconds. A retained MQTT value can populate the display without authorizing a command. Unknown, invalid, disconnected or retired-source observations do not become fresh because another field updated. After a source switch, wait for new observations rather than repeatedly pressing a disabled control.

The MQTT read-only refresh runs every 20 seconds, including explicit reads for the selected pump/valve `Mode` leaves. This keeps static modes observable without relying on a value change. It does not grant permission to queue a stale command for later execution.

## Control families

**Controller and ESS.** Supported controller switches and the six ESS choices require the corresponding live capability. Freshness, supported values and DRY restrictions are checked again at dispatch. A request ID scopes results/errors to the action. Check resulting authoritative state; an HTTP/WebSocket send is not a physical acknowledgement.

**Persistent setpoint override.** This differs from a legacy one-shot setpoint. The controller maintains the override; stopping it uses an explicit null value. Set and clear are independent of DRY under the controller protocol. The backend uses a total five-second deadline for dispatch/confirmation, requires matching request ID and exact value without a controller error, and does not automatically resend after a source change. Do not describe DRY as a way to neutralize this feature.

**Tariffs.** Controller tariff changes require a current writable capability, fresh status and the current revision. Local browser tariff preferences and controller tariff persistence are separate. The controller remains authoritative; missing tariff or energy history does not become a fabricated complete day.

**Water.** Configured native instances and fresh valid `Mode` observations gate supported water commands. Invalid updates revoke prior authority. Gateway mode can send supported, capability-gated water commands; it is not categorically display-only.

**Home Assistant.** Only configured targets are allowed. Direct controls must be enabled and the relevant observation must be fresh. Disabled/removed Home entities cannot fall through to a generic MQTT toggle. HA service success confirms the request was accepted by HA; it does not establish the physical device's final state. Covers/scenes/media/number controls may have physical consequences too.

## Alarms and source time

Native events retain original occurrence timestamps through MQTT, gateway snapshots and reconnects. The UI can show an old warning because it is still relevant; reconnect does not change it to a new event. Missing/invalid time is unknown. Exact local timestamp/timezone and relative age help distinguish source time from receipt time.

Resolution, acknowledgement and silence are distinct. An inactive but unacknowledged alarm may remain visible. Local presentation changes do not imply acknowledgement, and acknowledgement/silence controls can send actual source commands. Verify their scope before using them; do not use them as read-only troubleshooting steps.

Web Push has an independent lifecycle and a five-minute event lifetime, startup priming and persistent duplicate suppression. A historic on-screen alarm may correctly produce no new system notification. See [the event diagram](assets/notification-lifecycle.svg) and [delivery policy](web-push.md).

## Authentication and network boundary

Set `DASHBOARD_SECRET` before network exposure. The application has a shared-secret model, not per-user roles or an independent viewer account. Browser command connections check same-origin Host rules. Settings mutations require JSON and reject cross-site browser origins. Push mutations additionally require an explicit HTTPS Origin. Ingress must preserve Host; arbitrary forwarded Host/Proto headers are not a substitute.

The application exposes health, metrics and designated static resources without that shared-secret check. Restrict those at ingress when appropriate. Protect MQTT access and HA/gateway credentials separately. Use valid trusted certificates; disabling certificate verification is not an operational fix.

## Update and recovery

Treat application version, embedded UI version and private data as separate identities. The frontend receipt records what is actually in a Go build; source-only translation changes do not update existing binaries. Container updates should replace verified images, not run the optional self-updater inside a container.

When an update fails, preserve its identity and sanitized diagnostics, and restore a compatible known-good artifact through your normal deployment procedure. Preserve settings and push identity across replacement. Do not clear browser/server subscriptions merely because a provider accepted a message without an OS banner: first check permission, OS policy and bounded delivery outcomes.
