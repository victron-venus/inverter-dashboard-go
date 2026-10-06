# Browser system notifications

Web Push is opt-in and works while dashboard tabs are closed. The browser must
support PushManager and service workers, have notification permission, and use
HTTPS. The shared Vue settings panel registers the current browser with this
server; the server uses standard encrypted Web Push and a persistent VAPID key.
A test response means **queued**, not confirmed delivery by the operating system.

## Server configuration

- `WEB_PUSH_ENABLED=true` enables the service; it is disabled by default.
- `WEB_PUSH_DATA_DIR` must be an absolute dedicated directory on durable storage.
  For example, mount the volume at `/var/lib/inverter-dashboard` and use
  `/var/lib/inverter-dashboard/notifications` as the child directory.
- `WEB_PUSH_SUBJECT` defaults to the HTTPS GitHub project URL. A replacement must
  be an HTTPS contact URL.

Linux and macOS support the private persistent store. Windows builds retain the
ordinary dashboard but report persistent Web Push unavailable. The store requires
mode 0700 for its directory and mode 0600 for regular files, rejects symlinks and
hard-linked files, and holds an exclusive process lock. Use a single writer and
Recreate deployment strategy. Keep the directory on durable storage across image
updates: it contains subscription capabilities, encryption material, pending
notifications, and deduplication records. Do not publish it or include it in logs.
Corrupt, inaccessible, or previously initialized but missing state fails closed;
it never silently generates a replacement application key.

## Events and replay rules

Native warnings and alarms use the original Victron/controller timestamp.
Unknown timestamps are never replaced with receipt time. Startup, reconnect, and
source changes silently prime the existing notification list. MQTT's initial
10-second hydration window is also silent; a late timestamp for an unknown
baseline event stays silent. Events must belong to the current source epoch,
be no older than five minutes, and be no more than 30 seconds in the future.
The server keeps the latest 4096 distinct event keys across restarts.

Synthetic notifications cover charger power crossing 10 W, configured native
pump/valve transitions, and measured native battery SoC crossing 20% downward.
Each requires two fresh observations of the same selected device, no more than
30 seconds apart. Retained, missing, disconnected, or changed-source observations
clear the baseline. Battery SoC uses the native system reading, then the selected
native battery, or a sole native battery. Voltage estimates and near-zero grid
watts never produce battery/grid notifications. Telemetry processing runs even
with no WebSocket clients.

The sender allows 64 subscriptions, 1024 pending deliveries, 3072-byte payloads,
and two concurrent requests. It uses remaining source-event lifetime as provider
TTL, at most 300 seconds. Only transport failures, 429, and 5xx retry, up to three
attempts. 404/410 removes the subscription. Disabling a category or deleting a
subscription removes matching queued work and cancels matching in-flight
requests; source changes similarly cancel the retired epoch. A request already
accepted by a provider cannot be recalled. A process crash after provider
acceptance can cause an at-least-once retry; stable event keys let the service
worker deduplicate those deliveries.

## HTTP and outbound security

The notification API inherits `DASHBOARD_SECRET` authentication. New browser API
requests use an Authorization header. Mutations additionally require JSON and an
explicit HTTPS Origin whose authority exactly matches the request Host; forwarded
Host/Proto headers are not trusted for this check. Ingress must preserve Host.
Missing/null/foreign origins and cross-site Fetch metadata are rejected. All API
responses use `Cache-Control: no-store`; no endpoint inventory or private key is
returned. Request bodies are bounded to 8192 bytes. The exact root paths `/notifications-sw.js`, `/manifest.webmanifest`, and
`/notification-icon.svg` are public static resources with no-cache and their
respective JavaScript, manifest, and SVG media types. The worker has explicit
root scope permission and does not cache application assets.

Supported push hosts are exactly `fcm.googleapis.com`,
`updates.push.services.mozilla.com`, and valid ASCII DNS subdomains of
`push.apple.com` or `notify.windows.com`, all HTTPS on port 443. Unknown providers
are explicitly unsupported. Userinfo, fragments, IP literals, malformed names,
private/special/mixed DNS results, redirects, and environment proxies are refused.
The sender resolves once and dials a validated public address while preserving
TLS hostname verification. Responses and errors never echo endpoint capabilities
or provider bodies. Test notifications require a registered endpoint and are
limited to one per endpoint per minute and ten per minute globally.

Each send attempt records its notification kind, attempt number, HTTP status
(zero when no response arrived), and a fixed error category in the application
log. Endpoint URLs, keys, subscription identifiers, payloads, raw errors and
provider response contents are excluded. A 2xx status proves provider acceptance;
it does not prove that the browser displayed an OS notification. Completed
outcomes are not added to the persistent subscription store.

## Offline verification

`internal/push` tests use temporary private stores and fake HTTP clients; they
never send to real providers or devices. `NewService` and `RegisterRoutes` can be
used with a test Gin router plus the normal auth middleware, without constructing
MQTT, gateway, or Home Assistant clients. Omit `Service.Start` when exercising only
the API. Unit and HTTP contract checks do not demonstrate native OS delivery;
that requires a real browser opt-in and an explicit delivery test.
