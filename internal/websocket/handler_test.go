package websocket

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockha"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

// --- existing tests ---

func TestStateToMap(t *testing.T) { /* unchanged */
	s := &state.State{
		SolarTotal:    3500.0,
		GT:            150.0,
		BatterySOC:    85.5,
		TT:            1200.0,
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

func TestMergeStatesHAConnected(t *testing.T) { /* unchanged */
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

	booleans, ok := merged["booleans"].(map[string]bool)
	if !ok {
		t.Fatal("booleans not a map[string]bool")
	}
	if booleans["only_charging"] != true {
		t.Errorf("booleans.only_charging = %v, want true", booleans["only_charging"])
	}

	if merged["water_level"] != 45.0 {
		t.Errorf("water_level = %v, want 45.0", merged["water_level"])
	}
	if merged["car_soc"] != 75.0 {
		t.Errorf("car_soc = %v, want 75.0", merged["car_soc"])
	}

	if merged["solar_total"] != 3500.0 {
		t.Errorf("solar_total = %v, want 3500.0", merged["solar_total"])
	}
}

func TestMergeStatesHADisconnected(t *testing.T) { /* unchanged */
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

	if merged["water_level"] != false {
		t.Errorf("water_level = %v, want false", merged["water_level"])
	}
	if merged["car_soc"] != false {
		t.Errorf("car_soc = %v, want false", merged["car_soc"])
	}

	if merged["solar_total"] != 3500.0 {
		t.Errorf("solar_total = %v, want 3500.0", merged["solar_total"])
	}
}

func TestSafeFeatures(t *testing.T) {
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

// --- handleMessage tests ---

func TestHandleMessage_UnknownAction(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	msg := Message{Action: "bogus"}
	err := handleMessage(msg, nil, nil)
	if err == nil {
		t.Fatal("handleMessage unknown action: want error, got nil")
	}
	if err.Error() != "unknown action: bogus" {
		t.Errorf("error = %q, want %q", err.Error(), "unknown action: bogus")
	}
}

func TestHandleMessage_SetSettings_Nil(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	msg := Message{Action: "set_settings", Settings: nil}
	err := handleMessage(msg, nil, nil)
	if err == nil {
		t.Fatal("handleMessage set_settings nil: want error, got nil")
	}
	if err.Error() != "settings required" {
		t.Errorf("error = %q, want %q", err.Error(), "settings required")
	}
}

func TestHandleMessage_PublishCommand(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	msg := Message{Action: "dry_run"}
	err := handleMessage(msg, mc, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 {
		t.Fatalf("len=%d, want 1", len(pub))
	}
	if pub[0].Action != "dry_run" {
		t.Errorf("action=%q, want dry_run", pub[0].Action)
	}
}

func TestHandleMessage_PublishCommandSetpoint(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	msg := Message{Action: "setpoint", Value: 42.0}
	err := handleMessage(msg, mc, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 {
		t.Fatalf("len=%d, want 1", len(pub))
	}
	if pub[0].Action != "setpoint" {
		t.Errorf("action=%q, want setpoint", pub[0].Action)
	}
	if pub[0].Payload["value"] != 42.0 {
		t.Errorf("value=%v, want 42.0", pub[0].Payload["value"])
	}
}

func TestHandleMessage_PublishCommandLimits(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	msg := Message{Action: "limits", Min: 10.0, Max: 50.0}
	err := handleMessage(msg, mc, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 {
		t.Fatalf("len=%d, want 1", len(pub))
	}
	if pub[0].Action != "limits" {
		t.Errorf("action=%q, want limits", pub[0].Action)
	}
	if pub[0].Payload["min"] != 10.0 {
		t.Errorf("min=%v, want 10.0", pub[0].Payload["min"])
	}
	if pub[0].Payload["max"] != 50.0 {
		t.Errorf("max=%v, want 50.0", pub[0].Payload["max"])
	}
}

func TestHandleMessage_EssMode(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	msg := Message{Action: "ess_mode"}
	err := handleMessage(msg, mc, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 || pub[0].Action != "ess_mode" {
		t.Errorf("ess_mode not published")
	}
}

func TestHandleMessage_LoopInterval(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	msg := Message{Action: "loop_interval", Interval: 1.5}
	err := handleMessage(msg, mc, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 {
		t.Fatalf("len=%d, want 1", len(pub))
	}
	if pub[0].Action != "loop_interval" {
		t.Errorf("action=%q, want loop_interval", pub[0].Action)
	}
	if pub[0].Payload["interval"] != 1.5 {
		t.Errorf("interval=%v, want 1.5", pub[0].Payload["interval"])
	}
}

func TestHandleMessage_LoopIntervalZero(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	msg := Message{Action: "loop_interval", Interval: 0}
	err := handleMessage(msg, mc, nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if pub[0].Payload["interval"] != 0.33 {
		t.Errorf("interval=%v, want 0.33 (default)", pub[0].Payload["interval"])
	}
}

// --- handleToggle tests ---

func TestHandleToggle_EmptyEntity(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	err := handleToggle("", nil, nil)
	if err == nil {
		t.Fatal("handleToggle empty entity: want error, got nil")
	}
	if err.Error() != "entity ID required for toggle" {
		t.Errorf("error = %q, want %q", err.Error(), "entity ID required for toggle")
	}
}

func TestHandleToggle_FallsBackToMQTT(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	hc := mockha.NewClient()
	// HA not direct mode → falls back to MQTT
	hc.SetDirectMode(false)

	err := handleToggle("light.foo", mc, hc)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 || pub[0].Action != "toggle" {
		t.Errorf("toggle not published via MQTT")
	}
	if pub[0].Payload["entity"] != "light.foo" {
		t.Errorf("entity=%v, want light.foo", pub[0].Payload["entity"])
	}
}

func TestHandleToggle_HADirectMode(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	hc := mockha.NewClient()
	hc.SetDirectMode(true)
	hc.SetToggleAllowed("button.foo", true)

	err := handleToggle("button.foo", mc, hc)
	if err != nil {
		t.Fatal(err)
	}
	// MQTT should NOT be called in HA direct mode
	if len(mc.Published()) != 0 {
		t.Errorf("MQTT publish called in HA direct mode: %v", mc.Published())
	}
}

func TestHandleToggle_HANotToggleAllowed(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	hc := mockha.NewClient()
	hc.SetDirectMode(true)
	// Not in toggleAllowed map → falls back to MQTT
	hc.SetToggleAllowed("light.foo", false)

	err := handleToggle("light.foo", mc, hc)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 || pub[0].Action != "toggle" {
		t.Errorf("toggle not published via MQTT fallback")
	}
}

// --- handlePress tests ---

func TestHandlePress_EmptyEntity(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	err := handlePress("", nil, nil)
	if err == nil {
		t.Fatal("handlePress empty entity: want error, got nil")
	}
	if err.Error() != "entity ID required for press" {
		t.Errorf("error = %q, want %q", err.Error(), "entity ID required for press")
	}
}

func TestHandlePress_FallsBackToMQTT(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	hc := mockha.NewClient()
	hc.SetDirectMode(false) // not direct mode

	err := handlePress("button.foo", mc, hc)
	if err != nil {
		t.Fatal(err)
	}
	pub := mc.Published()
	if len(pub) != 1 || pub[0].Action != "press" {
		t.Errorf("press not published via MQTT")
	}
	if pub[0].Payload["entity"] != "button.foo" {
		t.Errorf("entity=%v, want button.foo", pub[0].Payload["entity"])
	}
}

func TestHandlePress_HADirectMode(t *testing.T) {
	resetClientsForTest()
	defer resetClientsForTest()

	mc := mockmqtt.NewClient()
	hc := mockha.NewClient()
	hc.SetDirectMode(true)
	hc.SetToggleAllowed("button.foo", true)

	err := handlePress("button.foo", mc, hc)
	if err != nil {
		t.Fatal(err)
	}
	// PressButton should be called
	if hc.PressButtonCalled() != "button.foo" {
		t.Errorf("PressButtonCalled=%q, want button.foo", hc.PressButtonCalled())
	}
	// MQTT should NOT be called
	if len(mc.Published()) != 0 {
		t.Errorf("MQTT publish called in HA direct mode")
	}
}
