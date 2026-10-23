# Developer guide

[README](../README.md) · [Contributing](../CONTRIBUTING.md) · [Verification checklist](../README-CHECKLIST.md)

## Build the server, not an npm wrapper

Use Go 1.27.3, selected by [go.mod](../go.mod), with the Go 1.26.10 module baseline preserved. From the repository root:

```sh
go mod download
go build -trimpath -ldflags "-X main.Version=$(cat VERSION)" -o inverter-dashboard .
go test ./internal/config ./internal/homeassistant
```

The root package spans multiple files; `go run main.go` is not equivalent to `go run .`. Building and testing do not require a live inverter. Running the server does load the working-directory configuration and can contact real services, so use explicit isolated fixtures for integration work.

## Source map

- [main.go](../main.go): startup, HTTP routes, transport selection, polling and lifecycle.
- [internal/config](../internal/config): environment/YAML parsing and configured entity definitions.
- [internal/mqtt](../internal/mqtt): native subscriptions, retained/live distinctions, portal/device selection and command dispatch.
- [internal/gateway](../internal/gateway): HTTPS snapshots, supported commands and source lifecycle.
- [internal/state](../internal/state): telemetry model, strict controller status and command validation.
- [internal/homeassistant](../internal/homeassistant): configured entity polling, overlay and bounded actions.
- [internal/websocket](../internal/websocket): browser messages, correlated outcomes and payload composition.
- [internal/settings](../internal/settings): private atomic settings persistence and masking.
- [internal/push](../internal/push): private store, subscription API, event processing, safe provider transport and bounded delivery.
- [internal/auth](../internal/auth): shared-secret middleware.
- [internal/html](../internal/html): exact embedded frontend payload and provenance.
- [contracts](../contracts): shared API/data fixtures; root and package `*_test.go` files cover unit and integration behavior.

The [architecture diagram](assets/architecture.svg) shows these boundaries. Native telemetry and server-owned capability fields must remain authoritative over optional HA overlays and browser input.

## Invariants to preserve

Source generations and MQTT subscription sessions are distinct. Reject callbacks from retired sessions; don't relabel a delayed old callback with the current generation. Recheck source/capability ownership immediately at dispatch and after awaited work. Hold ownership locks only through the nonblocking dispatch decision, not across an unbounded network wait.

Retained data can support display but cannot renew command authority. Missing/invalid water modes invalidate only the matching configured instance's authority. Status, freshness and acknowledgement serials must be read atomically where a command needs all of them. Override acknowledgement requires the exact request ID/value and no error within one total deadline.

Preserve original event timestamps and wire precision. A receiving timestamp is not an occurrence time. First snapshots/reconnects prime push history silently; invalid/unknown samples reset synthetic baselines. A native grid outage is not inferred from low watts, and a voltage-derived estimate is not native SoC.

Persist private data before claiming successful durable storage. Reconcile hot settings state if replacement succeeded but the directory durability check failed, while still reporting the failure. Don't erase a previously initialized push identity when data is missing or corrupt. Public API/logging must not expose credentials, push capabilities or raw provider errors.

## Checks

The full local policy entry point is [scripts/ci.sh](../scripts/ci.sh):

```sh
bash scripts/ci.sh
```

It downloads modules, runs golangci-lint and govulncheck, runs race-enabled Go tests with coverage, and checks the declared coverage threshold. Install the versions required by the checked-in manifests/workflows. Use focused package tests during development, then complete checks appropriate to the change. Run the configured hooks, for example:

```sh
pre-commit run --files README.md docs/configuration.md
```

Do not weaken checks or regenerate unrelated dependencies to make documentation pass. Pure documentation changes need link/example validation, accessible SVG validation and hooks, not physical commands. For regressions, exercise the rejected input and failure paths rather than only copying implementation structure.

[Fuzzing guidance](fuzzing.md) distinguishes ordinary seed replay from the release-candidate mutation workflow. [Browser acceptance](../.github/workflows) is governed by its existing workflows; do not create a competing release or acceptance pipeline for a documentation edit.

## Shared UI and translations

Edit Vue/TypeScript, styles and UI translations in [inverter-dashboard-vue](https://github.com/victron-venus/inverter-dashboard-vue), under that repository's tests and lockfile. This Go repository does not independently build the shared frontend during normal operation.

Adopt a verified official SPA artifact without changing its bytes, filenames or inventory. Update `internal/html/vue-ui-source.json` with the source commit, release/run/receipt metadata and exact file hashes. Run existing embedding/HTTP tests, including all three public root resources: service worker, manifest and notification icon. Do not format minified generated bundles or substitute a local build for an official artifact.

Localized READMEs describe the same server behavior and should stay aligned when configuration, security or persistence changes. Keep environment names and code identifiers unchanged in translations. A documentation translation is not evidence that the embedded UI has already received a new locale.

## Documentation and wiki maintenance

Keep examples synthetic and runnable only after deliberate local configuration. Prefer links to canonical configuration, security and release policy instead of duplicating commands that will drift. SVGs in `docs/assets` are self-contained XML with accessible titles/descriptions and no external images, scripts or fonts.

The Markdown files in `docs/wiki` are publication sources. In-repository changes do not publish the GitHub wiki automatically; the maintainer reviews and publishes them separately. Their canonical-document links use repository URLs so they remain useful after publication.

## Release work

Use [RELEASING.md](../RELEASING.md), [.release-policy.json](../.release-policy.json) and [the release runbook](release-workflow.md). Source version changes use reviewed pull requests; saved release plans bind exact versions and source. CI success, artifact publication, registry import and deployment are separate events. Never create a replacement tag or bypass a failed artifact check to complete a release.
