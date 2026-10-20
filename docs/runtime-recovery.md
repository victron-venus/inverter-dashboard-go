# Connection recovery and invalid source data

The dashboard keeps transport health separate from the validity and ownership of
individual readings. A successful reconnect does not make an old event new, and
HTTP success alone does not validate an entity's payload.

## IGW polling and commands

Gateway requests retain their total HTTP deadline. After a transport failure,
including a timeout while reading the response body, the poller closes idle
connections and waits the configured poll interval before its next attempt. It
drains idle connections again before that attempt so a late HTTP/2 stream
cancellation cannot leave the next poll using a failed idle connection.

HTTP application errors and malformed JSON mark the gateway disconnected but do
not force an otherwise working connection to be replaced. Recovery does not
interrupt active requests or replay commands. An override command retains its
single original deadline and must receive the matching acknowledgement.

When switching away from IGW, `Stop` cancels requests and waits for both poll
callbacks and an in-progress override acknowledgement callback to finish.
Callbacks must not call `Stop` synchronously. A result that reaches the state
callback after cancellation is rejected.

## MQTT notifications

Every subscription callback keeps the MQTT session token captured when the
subscription was installed. That token is passed through the controller and
native notification parsing paths, rather than recaptured after decoding.
Immediately before updating state, the callback checks the session, push
generation and active source. MQTT notifications cannot replace IGW state while
gateway mode is active. Session retirement and push-generation changes take the
same exclusive source lock, so a state callback already in progress finishes
before retirement, including when it was waiting for the state lock.

The notification's timestamp remains the source event timestamp. Reconnecting,
decoding or receiving a retained message does not replace it with the receipt
time. Existing behavior for missing or invalid source time is unchanged.

## Home Assistant responses

A state response must contain a string `state`. A JSON `null`, list, missing
state or incorrectly typed state does not renew connection health or control
freshness. The next valid poll can recover normally. Valid `unknown` and
`unavailable` strings still show that HA responded, while the corresponding
entity remains unavailable.

Nonfinite sensor values become `null`; nonfinite appliance power values become
`false`; invalid durations retain the existing `0` representation. Filtered
numeric displays never inject NaN or infinity into the dashboard JSON. Explicit
invalid number bounds cannot acquire default control permissions.

Durations accept finite nonnegative numeric seconds, truncating the fractional
part, or `MM:SS` / `HH:MM:SS` with unsigned ASCII integer components. Non-leading
components must be below 60. Values exceeding JavaScript's exact integer range
(`2^53 - 1`) or the native Go integer range are rejected. Outer whitespace is
ignored; malformed, negative and overflowing values become `0`.

## Regression checks

Run `go test -race ./internal/gateway ./internal/mqtt ./internal/homeassistant`.
The tests cover real local HTTP/2 connection recovery, active command isolation,
callbacks held across source switches, malformed HA responses followed by
recovery, JSON serialization and duration boundaries. They do not send commands
to physical equipment or establish native OS notification delivery.
