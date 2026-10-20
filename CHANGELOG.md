# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.9.15] - 2026-10-20

### Fixed

- Recover gateway polling after transport failures, including interrupted
  HTTP/2 response bodies, without replaying commands or interrupting active
  requests. Keep the configured delay between recovery attempts.
- Wait for an in-progress gateway override callback before retiring its source.
  Preserve the original MQTT subscription session through controller and native
  notification decoding, portal discovery and deferred subscriptions, and fence
  state updates against source retirement.
- Reject malformed Home Assistant state responses and recover on the next valid
  poll. Keep nonfinite readings out of JSON, reject invalid numeric control
  bounds consistently in displayed sliders and command authorization, and
  constrain appliance durations to valid nonnegative, representable values.

### Changed

- Import the verified official Vue 2.2.11-beta.1 interface without rebuilding.
  Its WebSocket recovery no longer lets successful HTTP fallback polling hide a
  silent command connection. Keep the existing reconnect interval and command
  gates; no commands are replayed.
- Align the local lint hook with the Go 1.27-compatible golangci-lint 2.14 used by
  CI, retaining its standard checks. Use the reviewed Go 1.27.2 Alpine container
  base and current pinned CI workflows.

### Upgrade

Preserve local configuration, credentials, notification storage and persistent
volumes when upgrading. No configuration or data migration is required. Retain
the previous immutable image or package for rollback, and verify fresh telemetry
and configured Home controls before accepting the deployment.

### Security

Delayed callbacks from retired sources cannot renew notification state or
control eligibility. Malformed and nonfinite HA responses do not grant control
permission. Authentication, TLS verification and command deadlines remain
required. The local scan found no affected symbols or imported packages;
GO-2026-5932 is a module-only finding in unused deprecated OpenPGP packages.
No scanner exclusion is added. The synthetic regression tests do not establish
physical-device acceptance or delivery of operating-system notifications.

## [1.9.14] - 2026-10-18

### Fixed

- Import the verified Vue 2.2.10 dashboard release, which limits tariff cost intervals to 1–366 inclusive calendar dates across daylight-saving transitions and leap years.

### Changed

- Route Dependabot metadata approval through the trusted default branch while preserving the existing author and review policy.

### Upgrade

Use the documented upgrade procedure and preserve local configuration and data. No data migration is required. Keep the previous immutable release available for rollback.

### Security

The frontend import verifies the published source, manifest, receipt and archive bytes. Existing authentication and release-validation requirements remain in effect; publication does not establish physical-device acceptance.

## [1.9.13] - 2026-10-17

### Changed

- Embed the verified official Vue 2.2.9 stable distribution, with interface text in
  English, Russian, Ukrainian, German, French and Dutch. Localize dates, times,
  retained application errors and theme labels while keeping provider-supplied
  messages intact.
- Update the SHA-pinned shared CI and release toolkit used by the reviewed
  dependency maintenance changes.
- Select Go 1.27.2 for CI, release builds and bounded fuzzing while preserving
  the Go 1.26.9 module baseline and existing dependency versions. This includes
  the upstream fix for spurious fuzz-timeout failures ([golang/go#75804](https://github.com/golang/go/issues/75804)).

### Fixed

- Wait for the loaded dashboard's expected telemetry and connected transport in
  browser acceptance checks, instead of waiting for background network traffic
  to stop. Public pages must expose their read-only marker and expected telemetry.
- Keep all eleven application scenarios and existing time limits. Add eight
  isolated browser controls that reject missing JavaScript, unavailable or
  incorrect telemetry and missing public-mode identification, while allowing
  noncritical requests to remain active, including when telemetry is requested
  before the noncritical request arrives.
- Run readiness controls sequentially in fresh browser contexts so deliberately
  pending requests cannot exhaust another control's shared connection pool.
  Preserve per-control traffic diagnostics on failure.
- Preserve immutable published frontend assets during pre-commit formatting and
  verify their release inventory through Git checkout instead. Whitespace checks
  continue to cover handwritten sources and documentation.

### Upgrade

Use the documented installation or container update procedure for the selected
release channel and retain the previous immutable release for rollback. Preserve local configuration, credentials
and existing Kubernetes persistent volumes; no configuration-format migration is
introduced. Reload the browser after updating to load the packaged interface.
Existing control permissions, connection settings and notification permissions
remain in effect. Verify the chosen language and live data on the deployment
before accepting it for normal use.

### Security

Import frontend files only from an explicit official release after verifying its
archive, file inventory and source-bound release metadata; do not substitute a
local frontend build. Synthetic browser-control pages are selected from fixed
HTML rather than interpolating request parameters, and unknown modes return 404.
These release and browser checks do not establish hardware acceptance or delivery
of operating-system notifications. Keep the deployment's authentication, network
isolation and independent equipment safeguards in place.

## [1.9.12] - Development line

### Changed

- Document the supported direct HTTPS/IGW cryptographic profile, advisory applicability, governance and one-year roadmap for the OpenSSF assessment.

- Qualify this release with a bounded RC fuzz campaign, offline target execution, and deterministic seed replay checks.
- Bind release evidence to the exact source and toolkit revision, validate seed corpora, and retain engine execution counts and diagnostic artifacts.
- Document the fuzzing scope, resource limits, and the distinction between synthetic checks and target acceptance.

### Upgrade

Use the existing installation or update procedure. This release changes validation
and release tooling; existing user configuration and data formats are preserved.
Keep the previous immutable release for rollback. Test the exact RC package on the
intended target before accepting a stable promotion.

### Security

Fuzz targets and deterministic preflight commands run offline on synthetic
inputs. Setup and dependency installation may use the runner network. The
campaign rejects missing or invalid seed corpora and incomplete execution
evidence. These checks do not establish untested hardware, integrations,
credentials, or deployment behavior.

## [1.9.11] - Development line

### Fixed

- Reject non-finite, overflowing and sub-nanosecond Home Assistant polling
  intervals from YAML. Invalid values now use the existing 12-second default
  instead of producing a zero ticker duration or stalling polling.

### Changed

- Add bounded native Go fuzz qualification for public release candidates, with
  source-bound evidence, synthetic seeds and documented reproduction. PRs, betas
  and nightlies retain ordinary tests without mutation campaigns.
- **Water system migrated from Home Assistant to dbus-pump via Cerbo MQTT** (no HA):
  - New `cerbo:` config section (`portal_id`, `tank_instance`, `pump_instance`,
    `valve_instance`; env: `CERBO_PORTAL_ID`, `WATER_*_INSTANCE`) subscribes to
    `N/<portal>/tank/<N>/Level` and `N/<portal>/pump/<N>/State`
  - `water_valve_entity` / `water_level_entity` / `pump_switch_entity` removed from
    the HomeAssistant config; HA no longer polls or overlays any water state
  - Removed `[BROADCAST DEBUG]` per-key logging from the websocket broadcaster
- **EV data migrated from Home Assistant to dbus-ev / dbus-evcharger via Cerbo MQTT** (no HA):
  - Extended `cerbo:` config section with `ev_instance` (default 22) and
    `evcharger_instance` (default 40); env `EV_INSTANCE` / `EVCHARGER_INSTANCE`
  - Subscribes to `N/<portal>/ev/<i>/Soc`, `N/<portal>/ev/<i>/Ac/Power` (W → kW),
    `N/<portal>/evcharger/<i>/Ac/Power` (W → kW); car stays on `ev` per dbus-ev's
    bus-name contract (not `evcharger`)
  - `car_soc_entity` / `ev_charging_kw_entity` / `ev_power_entity` removed from the
    HomeAssistant config; HA no longer polls or overlays any EV state

### Maintenance

- Replace outdated setup and npm guidance with current Go configuration,
  command-safety, persistence and notification documentation. Add complete
  Russian, Ukrainian, German, French and Dutch READMEs, accessible architecture
  diagrams and reviewed wiki publication sources. Embedded Vue assets are unchanged.
- Publish reviewed release notes from the exact source commit used to build each candidate, preserving build provenance.
- Document contribution checks, confidential security reporting and the project-specific trust boundaries.

### Upgrade

Water and EV values use their native Cerbo MQTT services. Configure the cerbo portal and service instances described above; old Home Assistant water and EV entity settings no longer populate those values. Retain existing unrelated configuration and verify source freshness before enabling controls.

### Security

Outbound Web Push, gateway, Home Assistant HTTPS and update downloads now require a normally verified certificate chain whose RSA keys are at least 2048 bits, ECDSA keys at least 224 bits, or valid Ed25519 keys. Replace weaker server or CA certificates before upgrading; hostname and trust verification remain required. HTTPS OTLP export uses the same peer-key checks and rejects undersized keys in the configured mTLS client chain. Its CA/client-certificate environment precedence and explicit HTTP behavior are preserved.

Private vulnerability reporting and response policy are documented in SECURITY.md. This maintenance update strengthens release evidence and review instructions; it does not replace deployment authentication, network isolation or independent equipment safeguards. No new project CVE is announced by these changes.
