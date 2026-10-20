# Security review: 16 October 2026

## GO-2026-5932 applicability

At source `1a1531efb766bd9459b5a0c6341a3715fda0fe84`, `go.mod` requires
`golang.org/x/crypto v0.57.0`. [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)
covers the deprecated `golang.org/x/crypto/openpgp` package family, not all
packages in the module. The current dashboard does not import that family.
The [exact-source CI scan](https://github.com/victron-venus/inverter-dashboard-go/actions/runs/38937850166/job/117102242767)
reported zero affected symbols and zero affected imported packages, alongside
one advisory in a required module. A local repeat with Go 1.26.9 agreed.

Scorecard alert 76 is a module-level signal. It does not demonstrate a reachable
OpenPGP defect in this application. No dependency version is presented as fixing
this advisory, which lists no fixed OpenPGP version. Keep both govulncheck and
Scorecard enabled; reassess if imports or the advisory change.

## Scope and outcomes

Reviewed maintained Go HTTP/WebSocket/MQTT handlers, embedded browser source,
private state, TLS clients/listener and Python/Shell release/deployment helpers.
The [cryptographic profile](cryptography.md) documents bearer authentication,
certificate validation, supported direct deployment and optional transport limits.
Race-enabled Go tests exercise real in-memory/local TLS handshakes and rejected
inputs. Browser and container integration checks are separate CI jobs.

A full-history Gitleaks review covered 551 commits and reported one historical
README curl example with an explicit YOUR_TOKEN placeholder. It is not a credential.
This finding does not justify excluding README files or weakening secret scans.
No unremediated live secret was established by this scan.

The public issue archive contains no non-PR bug/enhancement reports in the
reviewed interval; the maintainer confirmed no outside reports. Release metadata
for all 111 published versions included human-readable bodies; current release
notes additionally have source-bound Upgrade/Security sections. This is a dated
review, not a guarantee that future changes will preserve every property.

## Remaining assurance limits

Measured Go coverage is not whole-maintained-source coverage. No Silver 80%/90%
claim is made for all source, browser and delivery code. A backup maintainer and
one-week access restoration remain unestablished; see [governance](../GOVERNANCE.md).
This review did not operate an inverter, deploy infrastructure or qualify real
site credentials.
