# Documentation index

These documents cover running, configuring and verifying the Go dashboard
server, its embedded Vue interface, browser acceptance and release process.

## Get started

| Document | Description |
| --- | --- |
| [Process and results guide](process-and-results.md) | From a source change to a verified dashboard, and what each stage proves. |
| [Configuration reference](configuration.md) | The supported example file, options and field-specific precedence. |
| [Operator guide](operations.md) | Choosing a release, installing it and containers with persistence. |
| [Troubleshooting without operating equipment](troubleshooting.md) | Read-only first checks for version, configuration and connectivity. |

## Operate

| Document | Description |
| --- | --- |
| [Connection recovery and invalid source data](runtime-recovery.md) | Keeping transport health separate from data validity and ownership. |
| [Browser system notifications](web-push.md) | Opt-in Web Push, its storage requirements and delivery limits. |
| [Cryptography and TLS boundaries](cryptography.md) | Which TLS, X.509 and Web Push primitives the server relies on. |
| [Roadmap through October 2027](roadmap.md) | Priorities for compatibility, interfaces and reproducible validation. |

## Verify

| Document | Description |
| --- | --- |
| [Embedded frontend browser acceptance](browser-compatibility.md) | The shared TypeScript/browser checks in the required quality gate. |
| [Native browser, PWA and notification acceptance](native-browser-acceptance.md) | The separate desktop engine, PWA, Safari and installed-app evidence lanes. |
| [Packaged browser qualification network](PACKAGED-BROWSER-NETWORK.md) | The isolated Docker bridge used by qualification containers. |
| [Security review: 16 October 2026](security-review-20261016.md) | Review findings at a specific source commit and their scope. |
| [OpenSSF Best Practices evidence](openssf-evidence.md) | Evidence index reviewed against the Passing criteria. |
| [Screenshot artwork](images/README.md) | How the README screenshot is captured and regenerated. |

## Develop

| Document | Description |
| --- | --- |
| [Developer guide](development.md) | Go toolchain versions, local builds, tests and shared UI translations. |
| [CI orchestration](CI-ORCHESTRATION.md) | How the quality gate and release pipeline divide validation. |
| [Release version inputs](release-versioning.md) | The `versioning` fields that identify a release. |

## Wiki source

| Page | Description |
| --- | --- |
| [Inverter Dashboard Go](wiki/Home.md) | Wiki landing page for the Go server and embedded interface. |
| [Configuration](wiki/Configuration.md) | Wiki summary pointing at the canonical configuration reference. |
| [Notifications](wiki/Notifications.md) | Alarm occurrence times and missing or invalid timestamps. |
| [Troubleshooting](wiki/Troubleshooting.md) | Wiki summary of read-only troubleshooting steps. |
| [Development](wiki/Development.md) | Wiki summary pointing at the developer guide and policies. |
| [Process and results](wiki/Process-and-results.md) | Evidence per layer: source, frontend, release and running dashboard. |
| [Sidebar](wiki/_Sidebar.md) | Wiki navigation sidebar. |

Shared repository policy: [release-workflow.md](release-workflow.md), [runner-preflight.md](runner-preflight.md), [release-read-safety.md](release-read-safety.md), [release-tls-profile.md](release-tls-profile.md), [VERSIONING.md](VERSIONING.md), [rc-fuzzing.md](rc-fuzzing.md) and [fuzzing.md](fuzzing.md) are generated or shared boilerplate maintained in [victron-venus/venus-os-ci-toolkit](https://github.com/victron-venus/venus-os-ci-toolkit).
