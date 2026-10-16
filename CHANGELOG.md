# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
