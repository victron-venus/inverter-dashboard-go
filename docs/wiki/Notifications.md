# Notifications and alarms

Native alarms use the source's original occurrence time. A reconnect does not turn an old warning into “just now.” Missing or invalid timestamps remain unknown. Relative age updates without new telemetry, and the exact local timestamp/timezone can be inspected in the shared interface.

Resolution, acknowledgement, silence and local presentation are different operations. Acknowledgement or silence can send real commands; they are not read-only troubleshooting steps. An inactive but unacknowledged alarm may still appear.

![Source time, silent priming, eligibility, private queue and delivery stages](https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main/docs/assets/notification-lifecycle.svg)

## Browser system notifications

Web Push is optional and requires HTTPS, browser support and user/OS permission. Enable it with `WEB_PUSH_ENABLED=true` and an absolute, durable `WEB_PUSH_DATA_DIR`. The Linux/macOS store uses private permissions and one process lock; Windows servers report this feature unavailable while retaining ordinary dashboard functionality.

Startup and reconnect prime history silently. Native push events require known source time and eligible severity; synthetic device events require fresh observations of the same selected device. Near-zero grid power does not prove grid loss, and voltage estimates do not qualify as native battery SoC.

Limits include 64 subscriptions, 1024 pending deliveries, 3072-byte payloads, two concurrent requests and three attempts at most. Event lifetime is at most five minutes. The latest 4096 event keys are retained for duplicate suppression. Test notifications are limited to one per endpoint per minute and ten per minute globally.

API 202 means queued; provider 2xx means accepted by the provider; an OS banner is separate evidence. Preserve keys and subscriptions during updates. Do not clear the store to debug delivery. The [full Web Push guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/web-push.md) documents supported providers, endpoint validation, cancellation and safe log fields.
