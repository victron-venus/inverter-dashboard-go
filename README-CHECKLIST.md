# Maintainer verification checklist

This is a checklist for a specific change, not a declaration of complete parity with Python or Desktop. Record the tested commit, environment, checks and limitations in each pull request. Automated fixtures do not establish physical-device safety or successful OS notification display.

## Configuration and installation

- [ ] The documented Go version agrees with `go.mod`; build commands compile `.`.
- [ ] The configuration example contains supported keys and reserved example domains, with no credentials or private entity inventory.
- [ ] Working-directory lookup, field-specific precedence and restart requirements are explained.
- [ ] Home labels, ordering and enabled state come from configuration. An empty list stays empty; unavailable state is not fabricated as off.
- [ ] Settings writes report real failures and preserve unrelated settings. Persistent paths survive the proposed deployment lifecycle.
- [ ] Web Push identity, subscriptions and preferences survive updates; no second writer shares the store.

## Source and command contracts

- [ ] MQTT/IGW selection and reconnect tests prevent retired callbacks from authorizing commands or generating fresh events.
- [ ] Retained, missing, invalid or stale telemetry cannot grant fresh control permission.
- [ ] HA controls require configured targets and fresh observations; bounded number values respect observed metadata.
- [ ] Request IDs and command errors remain correlated. Transport acceptance is not presented as physical completion.
- [ ] ESS, water, override, tariff and alarm paths retain their own capability and acknowledgement rules. DRY is not described as a universal lock.
- [ ] No test sends real commands, acknowledges live alarms or changes external configuration.

## Notifications and UI

- [ ] Original event time survives replay; invalid/missing time remains unknown.
- [ ] Startup priming, slot reuse, source changes, synthetic baselines and persistent deduplication have relevant regression coverage.
- [ ] Queue/provider acceptance is distinguished from browser and OS presentation.
- [ ] Embedded frontend inventory and hashes exactly match the official artifact and its receipt; no local bundle edits or normalization.
- [ ] Required root service-worker, manifest and icon resources retain their exact bytes, content types and cache headers.
- [ ] Documentation languages are not used as evidence that an older embedded UI contains new translations.

## Review and validation

- [ ] Follow [CONTRIBUTING.md](CONTRIBUTING.md), run relevant tests and pre-commit hooks, and record any unavailable checks honestly.
- [ ] For release candidates, use the full checks declared by [release policy](.release-policy.json), including [scripts/ci.sh](scripts/ci.sh).
- [ ] Check links, code examples and accessible SVG titles/descriptions when documentation changes.
- [ ] Review actual CI results, review bodies and unresolved threads for the exact proposed head. Do not suppress checks to make a change pass.
- [ ] Follow [RELEASING.md](RELEASING.md) for signed source, saved plans, artifact verification and separate deployment authorization.

See [docs/development.md](docs/development.md) for source locations and [docs/troubleshooting.md](docs/troubleshooting.md) for read-only operational checks.
