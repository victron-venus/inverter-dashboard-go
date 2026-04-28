package state

import "fmt"

// FormatESSMode returns human-readable text for ESS mode based on the data
func FormatESSMode(mode ESSMode) string {
	switch mode.ModeName {
	case "External control":
		return "External Control"
	case "Self consumption":
		return "Self Consumption"
	case "Keep batteries charged":
		return "Keep Charged"
	default:
		return mode.ModeName
	}
}

// FormatHeaderToggles returns list of toggle buttons for header
func FormatHeaderToggles() []map[string]string {
	return []map[string]string{
		{"id": "dry_run", "label": "DRY External", "entity": "input_boolean.dry_run"},
		{"id": "only_charging", "label": "ONLY CHARGING", "entity": "input_boolean.only_charging"},
		{"id": "no_feed", "label": "NO FEED", "entity": "input_boolean.no_feed"},
		{"id": "house_support", "label": "HOUSE SUPPORT", "entity": "input_boolean.house_support"},
		{"id": "charge_battery", "label": "CHARGE BATTERY", "entity": "input_boolean.charge_battery"},
		{"id": "do_not_supply_ev", "label": "DO NOT SUPPLY EV", "entity": "input_boolean.do_not_supply_ev"},
		{"id": "limit_to_ev", "label": "LIMIT TO EV", "entity": "input_boolean.limit_to_ev"},
		{"id": "minimize_charging", "label": "MINIMIZE CHARGING", "entity": "input_boolean.minimize_charging"},
	}
}

// FormatLoads returns sorted loads from state
func (s *State) FormatLoads() [][2]interface{} {
	var loads [][2]interface{}
	// Extract loads from state if available
	// This is a placeholder - actual implementation would parse state.loads
	return loads
}

// FormatSolarSources returns individual solar sources
func (s *State) FormatSolarSources() []SolarSource {
	var sources []SolarSource
	// Parse mppt_individual or other solar fields
	// Placeholder implementation
	return sources
}

// FormatBatteries returns individual battery data
func (s *State) FormatBatteries() []Battery {
	var bats []Battery
	// Parse individual battery data from state
	// Placeholder implementation
	return bats
}

// FormatDailyStats computes daily stats with money
func (s *State) FormatDailyStats() map[string]interface{} {
	return map[string]interface{}{
		"solar_kwh":   s.DailyStats.SolarKWh,
		"solar_money": s.DailyStats.SolarMoney,
		"grid_kwh":    s.DailyStats.GridKWh,
		"grid_money":  s.DailyStats.GridMoney,
		"batt_in_kwh": s.DailyStats.BattInKWh,
		"batt_out_kwh": s.DailyStats.BattOutKWh,
		"batt_net_kwh": s.DailyStats.BattNetKWh,
		"display_text": fmt.Sprintf("☀️ %.2f kWh ($%.2f) | Grid: %.2f kWh ($%.2f) | 🔋 I: %.2f kWh, O: %.2f kWh, Δ: %.2f kWh",
			s.DailyStats.SolarKWh, s.DailyStats.SolarMoney,
			s.DailyStats.GridKWh, s.DailyStats.GridMoney,
			s.DailyStats.BattInKWh, s.DailyStats.BattOutKWh, s.DailyStats.BattNetKWh),
	}
}