# Inverter Dashboard Go

This wiki describes the Go server with its embedded shared Vue interface. It receives native Cerbo MQTT or HTTPS inverter-gateway telemetry and optionally integrates explicitly configured Home Assistant entities. It is a control surface, not only a chart viewer.

Start with the [installation and language guides](https://github.com/victron-venus/inverter-dashboard-go/blob/main/README.md). Node.js is not needed to run a released binary. Select an explicit release/platform, verify its published identity, and configure your own source and authentication before starting.

![Sources, server, browser and private storage](https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main/docs/assets/architecture.svg)

## Guides

- [Configuration](Configuration.md): file location, precedence, source selection and Home inventory.
- [Notifications](Notifications.md): original event time, startup priming, private Web Push and delivery limits.
- [Troubleshooting](Troubleshooting.md): read-only diagnosis and useful sanitized reports.
- [Development](Development.md): source ownership, tests, official frontend adoption and release policy.

## Essential boundaries

An unset `DASHBOARD_SECRET` disables application authentication. Retained or stale telemetry does not authorize a new control command. DRY is not a global lock: persistent setpoint override follows its own controller protocol. A queued notification is not proof of OS display. Settings and push identity need persistent private storage, not a read-only configuration mount or temporary container filesystem.

The versioned [operator guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/operations.md) provides the full details. This wiki's Markdown source is maintained in `docs/wiki`; repository changes are published to the wiki separately by a maintainer.
