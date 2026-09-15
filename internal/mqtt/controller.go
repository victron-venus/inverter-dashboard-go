package mqtt

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

var controllerFields = []string{"booleans", "features", "ess_mode", "dry_run", "daily_stats", "solar_forecast", "ui_config", "dvcc_limits", "limits", "perf", "loop_interval", "grid_control_valid", "grid_control_reason", "grid_loss_state", "grid_loss_hold_seconds", "grid_loss_elapsed", "grid_loss_remaining", "grid_loss_zero_applied", "version", "uptime"}

// ApplyControllerSnapshot accepts the retained inverter-control state without
// giving it ownership of native Cerbo telemetry. A null snapshot clears flags.
func ApplyControllerSnapshot(st *state.State, data map[string]interface{}) {
	available := len(data) > 0
	st.InverterAvailable = &available
	applyControllerFields(st, data)
	flags := controllerBooleans(data["booleans"])
	st.Booleans = flags
	st.OnlyCharging, _ = flags["only_charging"].(bool)
	st.NoFeed, _ = flags["no_feed"].(bool)
	st.HouseSupport, _ = flags["house_support"].(bool)
	st.ChargeBattery, _ = flags["charge_battery"].(bool)
	st.DoNotSupplyCharger, _ = flags["do_not_supply_charger"].(bool)
	st.SetLimitToEVCharger, _ = flags["set_limit_to_ev_charger"].(bool)
	st.MinimizeCharging, _ = flags["minimize_charging"].(bool)
	if st.UIConfig == nil {
		st.UIConfig = map[string]interface{}{}
	}
	if available && st.UIConfig["header_toggles"] == nil {
		st.UIConfig["header_toggles"] = state.FormatHeaderToggles()
	}
}

func applyControllerFields(st *state.State, data map[string]interface{}) {
	clean := map[string]interface{}{}
	for _, key := range controllerFields {
		if st.TelemetryAvailable[key] || (key == "ess_mode" && st.NativeESSObserved) {
			continue
		}
		clearStateField(st, key)
		if value, ok := data[key]; ok {
			clean[key] = value
		}
	}
	// Decode each field independently: one malformed optional field must not
	// prevent valid flags or ESS telemetry from reaching the dashboard.
	for key, value := range clean {
		encoded, err := json.Marshal(map[string]interface{}{key: value})
		if err == nil {
			_ = json.Unmarshal(encoded, st)
		}
	}
}

func controllerBooleans(raw interface{}) map[string]interface{} {
	flags := map[string]interface{}{}
	if values, ok := raw.(map[string]interface{}); ok {
		for key, value := range values {
			canonical := strings.TrimPrefix(key, "input_boolean.")
			if state.IsControlFlag(canonical) {
				if flag, ok := state.ControlBool(value); ok {
					flags[canonical] = flag
				}
			} else {
				flags[key] = value
			}
		}
	}
	return flags
}

// CanControlInverter requires current transport health and observed daemon state.
func (c *Client) CanControlInverter() bool {
	if !c.IsConnected() {
		return false
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.expireOptionalTelemetry(time.Now())
	return c.state.InverterAvailable != nil && *c.state.InverterAvailable
}

const optionalTelemetryTTL = 120 * time.Second

// Expiry prevents a silent daemon from leaving active controls on a still-connected dashboard.
func (c *Client) expireOptionalTelemetry(now time.Time) bool {
	c.initCerboMaps()
	if !c.controllerLastSeen.IsZero() && now.Sub(c.controllerLastSeen) > optionalTelemetryTTL {
		if !c.cerboOwned["ess_mode"] {
			c.state.TelemetryAvailable["ess_mode"] = false
		}
		ApplyControllerSnapshot(c.state, nil)
		c.controllerLastSeen = time.Time{}
		return true
	}
	return false
}

func (c *Client) refreshControllerFreshness(now time.Time) {
	c.stateMu.Lock()
	changed := c.expireOptionalTelemetry(now)
	c.stateMu.Unlock()
	if changed {
		c.triggerHandler()
	}
}

func (c *Client) clearOptionalTelemetry() {
	c.initCerboMaps()
	for _, key := range []string{"ess_mode", "ev_power", "car_charging_power", "car_soc", "ev_charging_kw", "ev_charging_power", "ev_present", "evcharger_present", "discovered_water_ev"} {
		clearStateField(c.state, key)
		c.state.TelemetryAvailable[key] = false
	}
	ApplyControllerSnapshot(c.state, nil)
	c.controllerLastSeen = time.Time{}
}
