# Development

Read the [developer guide](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/development.md), [contribution policy](https://github.com/victron-venus/inverter-dashboard-go/blob/main/CONTRIBUTING.md) and [verification checklist](https://github.com/victron-venus/inverter-dashboard-go/blob/main/README-CHECKLIST.md).

Use the Go toolchain in `go.mod`. Build the whole root package with `go build .`; `go run main.go` omits other package files. Node.js is not a runtime dependency. The historical npm wrappers are not the authoritative build or release interface.

Focused tests should use synthetic data and local fixtures. Preserve source/session fences, per-field freshness, strict command validation, correlated outcomes, original event time and private storage behavior. No automated documentation check should contact an inverter, issue a physical command or send a real push notification.

`bash scripts/ci.sh` runs the declared local quality checks, including lint, vulnerability checks and race-enabled tests. Run relevant package regressions while developing and the configured pre-commit hooks before review. Existing CI, browser acceptance and fuzzing workflows remain authoritative; do not create duplicate pipelines or weaken their gates.

## Shared frontend

UI code and translations live in [inverter-dashboard-vue](https://github.com/victron-venus/inverter-dashboard-vue). The Go repository embeds an official verified artifact and records its exact source/file hashes in `internal/html/vue-ui-source.json`. Preserve bytes and inventory; never hand-edit or reformat the generated bundle. Newly translated source is not automatically present in an older released Go binary.

## Docs and releases

Keep localized READMEs aligned on setup, security, storage and known limitations. SVG diagrams must be self-contained and accessible. Files in `docs/wiki` are reviewed publication sources; changing them does not itself update GitHub's wiki.

Follow the [release strategy](https://github.com/victron-venus/inverter-dashboard-go/blob/main/RELEASING.md) and [runbook](https://github.com/victron-venus/inverter-dashboard-go/blob/main/docs/release-workflow.md). Source review, CI, official artifacts, registry import and deployment are distinct steps. Preserve failed evidence; do not reuse a tag or bypass checks.
