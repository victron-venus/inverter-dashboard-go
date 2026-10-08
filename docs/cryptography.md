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

## Remaining review boundaries

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

These outbound TLS tests do not establish project-wide OpenSSF cryptographic
compliance by themselves.

The server's inbound bearer secret is operator-configured; the application does
not create user accounts or a password-verifier database. The separate review of
that credential boundary, external TLS termination and deployment configuration
must not be inferred from these outbound TLS tests.
