# Troubleshooting

Use read-only checks first. Do not toggle equipment or acknowledge a real alarm just to establish connectivity. The [full troubleshooting guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/troubleshooting.md) covers these cases in detail.

## Page unavailable

Check the service version/status, bind address, port, DNS/VPN route and certificate. The example binds loopback; remote access requires an appropriate proxy/network setup. `/health` proves HTTP service health, not current inverter data. A 401 means authentication failed; a 403 on a browser mutation may indicate an Origin/Host mismatch. Preserve the external Host at the proxy instead of disabling checks.

## Data visible, controls unavailable

Check source selection, portal and native instance configuration, and actual observation timestamps. Retained or old observations do not grant new command permission. A recent unrelated field or a connection icon does not prove that a specific control is fresh. Wait for a valid current-source observation rather than repeatedly retrying a command.

## Home missing or disabled

Empty configuration intentionally yields no Home controls. Verify the intended file was loaded, preserve your existing list, and inspect enabled flags, HA credentials, target availability and recent successful entity observations. Unknown is not off. Appliance visibility additionally requires an active state.

## Settings or notifications

For lost settings, inspect the configured path, ownership and persistent volume. Read-only mounts cannot accept changes, and temporary filesystems do not survive replacement. Do not publish the private settings file.

For push, check HTTPS/browser/OS permissions, category preferences, store health and bounded sender outcomes. A queued test or provider acceptance is not OS display. Do not delete the identity store or repeatedly send tests as a first response.

## Reporting

Provide package/source version, embedded Vue version, platform, transport, sanitized status/error category, reproduction and expected behavior. Remove credentials, push endpoints/keys, private hosts and personal device inventories. Send confidential security reports through [SECURITY.md](https://github.com/victron-venus/inverter-dashboard-go/blob/main/SECURITY.md).
