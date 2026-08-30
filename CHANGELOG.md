# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
