package homeassistant

import "strings"

// IsControlFlag recognizes controller-owned keys and their legacy HA aliases.
func IsControlFlag(key string) bool {
	parts := strings.Split(strings.TrimSpace(key), ".")
	switch parts[len(parts)-1] {
	case "only_charging", "no_feed", "house_support", "charge_battery",
		"do_not_supply_charger", "set_limit_to_ev_charger", "minimize_charging":
		return true
	}
	return false
}

// IsMQTTOwnedKey protects Cerbo telemetry and controller state from old HA mirrors.
// Home appliances and unrelated HA sensors retain their independent ownership.
func IsMQTTOwnedKey(key string) bool {
	if IsControlFlag(key) {
		return true
	}
	switch key {
	case "g1", "g2", "g3", "gt", "t1", "t2", "t3", "tt", "bv", "bc", "bp",
		"battery_soc", "battery_voltage", "battery_current", "battery_power", "battery_socs", "batteries",
		"solar_total", "pv_total", "mppt_total", "mppt_data", "mppt_individual", "mppt_chargers",
		"pv_inverter_total", "pv_inverter_individual", "pv_inverter_powers", "pv_inverters",
		"loads", "load_names", "setpoint", "inverter_state",
		"ev_power", "ev_charging_kw", "car_soc", "car_charging_power", "ev_charging_power",
		"ev_present", "evcharger_present", "water_level", "water_valve", "pump_switch",
		"water_pump_mode", "pump_mode", "water_valve_mode", "discovered_water_ev",
		"booleans", "features", "ess_mode", "dry_run", "daily_stats", "solar_forecast",
		"limits", "loop_interval", "filtered_gt", "dvcc_limits", "perf", "ui_config",
		"version", "uptime", "grid_control_valid", "grid_control_reason":
		return true
	}
	return false
}

func haOwnsField(key, entity string) bool {
	return !IsMQTTOwnedKey(key) && !IsControlFlag(entity)
}
