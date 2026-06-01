package websocket

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestStateToMap(t *testing.T) {
	s := &state.State{
		SolarTotal:   3500.0,
		GT:           150.0,
		BatterySOC:   85.5,
		TT:           1200.0,
		InverterState: "On",
		Booleans: map[string]interface{}{
			"only_charging": true,
		},
		Features: map[string]interface{}{
			"ev": true,
		},
	}

	m := stateToMap(s)
	if m == nil {
		t.Fatal("stateToMap returned nil")
	}

	if m["solar_total"] != 3500.0 {
		t.Errorf("solar_total = %v, want 3500.0", m["solar_total"])
	}
	if m["gt"] != 150.0 {
		t.Errorf("gt = %v, want 150.0", m["gt"])
	}
	if m["battery_soc"] != 85.5 {
		t.Errorf("battery_soc = %v, want 85.5", m["battery_soc"])
	}
	if m["inverter_state"] != "On" {
		t.Errorf("inverter_state = %v, want 'On'", m["inverter_state"])
	}

	booleans, ok := m["booleans"].(map[string]interface{})
	if !ok {
		t.Fatal("booleans not a map")
	}
	if booleans["only_charging"] != true {
		t.Errorf("booleans.only_charging = %v, want true", booleans["only_charging"])
	}
}

func TestMergeStatesHAConnected(t *testing.T) {
	s := &state.State{
		SolarTotal: 3500.0,
		GT:         150.0,
		Booleans: map[string]interface{}{
			"only_charging": false,
		},
	}

	overlay := homeassistant.Overlay{
		Booleans: map[string]bool{
			"only_charging": true,
		},
		HADirectConnected: true,
		AdditionalFields: map[string]interface{}{
			"water_level": 45.0,
			"car_soc":     75.0,
		},
	}

	managedKeys := []string{"only_charging", "water_level", "car_soc"}

	merged := mergeStates(s, overlay, managedKeys)

	if merged["ha_direct_connected"] != true {
		t.Error("ha_direct_connected should be true")
	}

	// Booleans should be overridden by HA
	booleans, ok := merged["booleans"].(map[string]bool)
	if !ok {
		t.Fatal("booleans not a map[string]bool")
	}
	if booleans["only_charging"] != true {
		t.Errorf("booleans.only_charging = %v, want true", booleans["only_charging"])
	}

	// Additional overlay fields should be present
	if merged["water_level"] != 45.0 {
		t.Errorf("water_level = %v, want 45.0", merged["water_level"])
	}
	if merged["car_soc"] != 75.0 {
		t.Errorf("car_soc = %v, want 75.0", merged["car_soc"])
	}

	// MQTT fields should still be present
	if merged["solar_total"] != 3500.0 {
		t.Errorf("solar_total = %v, want 3500.0", merged["solar_total"])
	}
}

func TestMergeStatesHADisconnected(t *testing.T) {
	s := &state.State{
		SolarTotal: 3500.0,
		Booleans: map[string]interface{}{
			"only_charging": true,
			"no_feed":       true,
		},
	}

	overlay := homeassistant.Overlay{
		Booleans:          map[string]bool{},
		HADirectConnected: false,
	}

	managedKeys := []string{"only_charging", "no_feed", "water_level", "car_soc"}

	merged := mergeStates(s, overlay, managedKeys)

	if merged["ha_direct_connected"] != false {
		t.Error("ha_direct_connected should be false")
	}

	// Booleans should be reset to false
	booleans, ok := merged["booleans"].(map[string]interface{})
	if !ok {
		t.Fatal("booleans not a map")
	}
	if booleans["only_charging"] != false {
		t.Errorf("booleans.only_charging = %v, want false", booleans["only_charging"])
	}
	if booleans["no_feed"] != false {
		t.Errorf("booleans.no_feed = %v, want false", booleans["no_feed"])
	}

	// Managed keys should be reset to false
	if merged["water_level"] != false {
		t.Errorf("water_level = %v, want false", merged["water_level"])
	}
	if merged["car_soc"] != false {
		t.Errorf("car_soc = %v, want false", merged["car_soc"])
	}

	// MQTT fields should still be present
	if merged["solar_total"] != 3500.0 {
		t.Errorf("solar_total = %v, want 3500.0", merged["solar_total"])
	}
}

func TestSafeFeatures(t *testing.T) {
	// Nil features should return empty map
	result := safeFeatures(nil)
	if result == nil {
		t.Fatal("safeFeatures(nil) returned nil")
	}
	if len(result) != 0 {
		t.Errorf("safeFeatures(nil) returned %d items, want 0", len(result))
	}
}

func TestGetKeys(t *testing.T) {
	m := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	keys := getKeys(m)
	if len(keys) != 3 {
		t.Errorf("getKeys returned %d keys, want 3", len(keys))
	}
	// Check all expected keys are present
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, k := range []string{"a", "b", "c"} {
		if !keySet[k] {
			t.Errorf("getKeys missing key %q", k)
		}
	}
}
