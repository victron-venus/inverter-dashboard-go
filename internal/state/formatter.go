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
		{"id": "only_charging", "label": "ONLY CHARGING", "entity": "only_charging"},
		{"id": "no_feed", "label": "NO FEED", "entity": "no_feed"},
		{"id": "house_support", "label": "HOUSE SUPPORT", "entity": "house_support"},
		{"id": "charge_battery", "label": "CHARGE BATTERY", "entity": "charge_battery"},
		{"id": "do_not_supply_charger", "label": "DO NOT SUPPLY EV", "entity": "do_not_supply_charger"},
		{"id": "set_limit_to_ev_charger", "label": "LIMIT TO EV", "entity": "set_limit_to_ev_charger"},
		{"id": "minimize_charging", "label": "MINIMIZE CHARGING", "entity": "minimize_charging"},
	}
}

// FormatDailyStats computes daily stats with money
func (s *State) FormatDailyStats() map[string]interface{} {
	return map[string]interface{}{
		"solar_kwh":    s.DailyStats.SolarKWh,
		"solar_money":  s.DailyStats.SolarMoney,
		"grid_kwh":     s.DailyStats.GridKWh,
		"grid_money":   s.DailyStats.GridMoney,
		"batt_in_kwh":  s.DailyStats.BattInKWh,
		"batt_out_kwh": s.DailyStats.BattOutKWh,
		"batt_net_kwh": s.DailyStats.BattNetKWh,
		"display_text": fmt.Sprintf("☀️ %.2f kWh ($%.2f) | Grid: %.2f kWh ($%.2f) | 🔋 I: %.2f kWh, O: %.2f kWh, Δ: %.2f kWh",
			s.DailyStats.SolarKWh, s.DailyStats.SolarMoney,
			s.DailyStats.GridKWh, s.DailyStats.GridMoney,
			s.DailyStats.BattInKWh, s.DailyStats.BattOutKWh, s.DailyStats.BattNetKWh),
	}
}
