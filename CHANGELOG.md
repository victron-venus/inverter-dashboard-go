# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.9.11] - Development line

### Changed
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

- Publish reviewed release notes from the exact source commit used to build each candidate, preserving build provenance.
- Document contribution checks, confidential security reporting and the project-specific trust boundaries.

### Upgrade

Water and EV values use their native Cerbo MQTT services. Configure the cerbo portal and service instances described above; old Home Assistant water and EV entity settings no longer populate those values. Retain existing unrelated configuration and verify source freshness before enabling controls.

### Security

Private vulnerability reporting and response policy are documented in SECURITY.md. This maintenance update strengthens release evidence and review instructions; it does not replace deployment authentication, network isolation or independent equipment safeguards. No new project CVE is announced by these changes.
