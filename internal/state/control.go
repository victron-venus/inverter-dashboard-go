package state

import "strings"

// IsControlFlag recognizes only the seven inverter-control flags and their
// documented input_boolean aliases. Other HA entities retain HA ownership.
func IsControlFlag(key string) bool {
	key = strings.TrimPrefix(strings.TrimSpace(key), "input_boolean.")
	switch key {
	case "only_charging", "no_feed", "house_support", "charge_battery", "do_not_supply_charger", "set_limit_to_ev_charger", "minimize_charging":
		return true
	}
	return false
}

func ControlBool(value interface{}) (bool, bool) {
	switch value := value.(type) {
	case bool:
		return value, true
	case float64:
		return value == 1, value == 0 || value == 1
	case int:
		return value == 1, value == 0 || value == 1
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "on", "1":
			return true, true
		case "false", "off", "0":
			return false, true
		}
	}
	return false, false
}
