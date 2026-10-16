# Cryptography and TLS boundaries

The Go standard library implements TLS and X.509 verification. Web Push uses the
MIT-licensed `github.com/SherClockHolmes/webpush-go` dependency pinned in `go.mod`
and `go.sum`. The project does not define a new encryption algorithm.

## Outbound TLS

[internal/tlsclient](../internal/tlsclient) configures TLS 1.2 or newer and leaves
normal certificate-chain and hostname verification enabled. Its additional
`VerifyConnection` check requires at least one complete, normally verified chain
with RSA keys of 2048 bits or more, ECDSA keys of 224 bits or more, or Ed25519 keys.
It checks every certificate in that chain, including the trust anchor. It also
runs on resumed connections. It does not treat unrelated certificates sent by a
server as trust paths, and does not reject a valid strong path merely because a
second path contains a weaker CA.

This policy applies to Web Push delivery, gateway requests, Home Assistant HTTPS
requests, version/update downloads and HTTPS OTLP trace export. Proxy selection, request deadlines,
redirect restrictions and the Web Push provider/DNS allowlist retain their
existing behavior. HTTP connections do not become TLS connections automatically;
local MQTT uses the configured TCP connection. Use the documented isolated
network and authenticated HTTPS deployment boundaries.

Before upgrading, replace RSA-1024 or other undersized certificates in an affected
server's full chain. The client fails the connection instead of bypassing trust
or hostname verification. No global HTTP transport or system trust store is
modified. Web Push logs only a bounded `tls` failure category, without exposing
subscription URLs or certificate data.

Tests use ephemeral keys and local in-memory TLS connections. They cover TLS 1.2
and 1.3; RSA, ECDSA and Ed25519; weak leaf/intermediate/root keys; a strong alternate
chain; rejection of a resumed weak session; and normal rejection of an untrusted
issuer or incorrect hostname. Local HTTP behavior is tested separately.

## Inbound HTTPS

When `--ssl-cert` and `--ssl-key` enable the built-in HTTPS listener, the server
loads the matching certificate/key pair and checks every certificate supplied in
the chain before opening a listener. RSA keys must have at least 2048 bits, ECDSA
keys at least 224 bits, and Ed25519 keys their standard length. The loaded pair is
retained for serving: changing a file after validation cannot replace it with an
unchecked pair. TLS 1.2 is the minimum protocol version; normal client trust and
hostname verification remain client responsibilities. This checks key strength,
not whether a certificate was issued by a client's trusted authority.

Replace undersized server or intermediate certificates before upgrading. A
malformed, mismatched or undersized pair now causes startup to fail. When both TLS flags are supplied, `startServer` opens only HTTPS on the
configured web port; it does not open an additional HTTP listener. Omitting either
flag selects the separate plain HTTP compatibility mode. External reverse proxy
TLS configuration is not controlled by this policy.

## Web Push

[push/store.go](../internal/push/store.go) obtains VAPID keys from the pinned
Web Push library. Version 1.4.0 uses P-256 and `crypto/rand.Reader` for key
generation. Message encryption uses ephemeral P-256 ECDH, HKDF-SHA-256 and
AES-128-GCM, with cryptographically random salt. Subscription public keys are
validated as P-256 points before delivery. Persisted private/public key pairs are
validated on load; damaged state is rejected instead of silently replacing the
application key. Local state uses a private directory and files.

Random queue identifiers use `crypto/rand.Read`. Their randomness is not derived
from timestamps or the MQTT client identifier. These standard-library random
sources use the operating system's cryptographically secure generator.

## OTLP and credential boundaries

The OTLP exporter in [tracing.go](../internal/tracing/tracing.go) applies the same
peer-key policy to HTTPS and checks the complete configured mTLS client chain
before export. Generic `OTEL_EXPORTER_OTLP_CERTIFICATE`, `CLIENT_CERTIFICATE` and
`CLIENT_KEY` settings are read first; their `OTEL_EXPORTER_OTLP_TRACES_` equivalents
then replace valid generic settings. A certificate and key must come from the
same prefix. Whitespace, invalid-file fallback and the library's diagnostics
retain the pinned OpenTelemetry 1.47 behavior. With no custom CA, normal system
roots remain in effect. Timeout, headers, compression, proxy and retry options
remain managed by the exporter.

Explicit HTTP/Insecure configuration retains its previous behavior; this change
does not silently upgrade those connections. Endpoint and Insecure environment
precedence is covered by comparison with the pinned library. Use HTTPS when
traces or exporter credentials cross an untrusted network. Replace undersized
server/CA or mTLS client certificates before upgrading.

The following complete direct-binary profile combines these boundaries without
relying on a third-party TLS terminator.

The server's inbound bearer secret is operator-configured; the application does
not create user accounts or a password-verifier database. Authentication uses constant-time comparison in `internal/auth/auth.go`. Missing
or incorrect secrets are rejected when configured. Query-string credentials are
supported for browser/WebSocket compatibility and can appear in history or proxy
logs; prefer an Authorization header, redact request URLs at ingress and never
share credential-bearing links. The empty-secret compatibility mode is unprotected.
Private settings use restricted filesystem permissions; bearer credentials are
recoverable secrets, not hashed human-password verifiers.

## Complete direct-binary profile

Use the verified Go binary directly with its built-in HTTPS listener (`--ssl-cert`
and `--ssl-key`) and a nonempty `DASHBOARD_SECRET` generated from at least 32 OS-random
bytes. Use `--port 8443` (or an explicitly chosen port); the application binds all
interfaces, so firewall that port to intended clients. Both TLS flags are required;
HTTPS replaces HTTP on that port. Do not expose a separate HTTP proxy listener. Use the
IGW-only configuration (`GATEWAY_ENABLED=true`, an `https://` gateway URL and
its independent bearer token; leave `MQTT_HOST` empty). This is a functional
telemetry and supported-command deployment; the configuration and tests in
`internal/gateway` cover it. The optional direct MQTT transport is plaintext and
requires a separately trusted network if enabled.

Home Assistant is optional; leave its URL/token unset or use a trusted HTTPS
endpoint with the same certificate policy. Leave OTLP export unset or use HTTPS
with the validated CA/mTLS configuration above. Web Push, when enabled, keeps
HTTPS endpoint validation and uses P-256/HKDF-SHA-256/AES-128-GCM. Do not enable
`InsecureSkipVerify`, an HTTP trace endpoint or external TLS termination and
assume the direct-binary certificate policy still applies.

Acquire source/artifacts through GitHub HTTPS and verify published SHA-256
checksums/provenance. The release CLI's verified stricter transport is described
in [release-tls-profile.md](release-tls-profile.md). Optional registry promotion,
SSH deployment and site-managed reverse proxies require their own operator
qualification; they are not required to build and run this complete profile.
All maintained source and delivery helpers remain in the review and scan scope.

The project uses SHA-1 only for Git object identity in release-control checks;
source/artifact integrity evidence uses SHA-256. The Git object identifier is
not treated as a modern standalone signature or password hash.
