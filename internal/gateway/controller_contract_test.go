package gateway

import (
	"encoding/json"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
)

func TestIGWControllerNativeEVAndExplicitClear(t *testing.T) {
	var snap Snapshot
	if err := json.Unmarshal([]byte(`{"inverter":{"booleans":{"only_charging":false},"ui_config":{"header_toggles":[]},"ev_power":999,"ess_mode":{"mode_name":"Old mode"}},"ev":{"812/Soc":0,"812/Ac/Power":3200},"evcharger":{"71/Ac/Power":0},"settings":{"0/Settings/CGwacs/Hub4Mode":3}}`), &snap); err != nil {
		t.Fatal(err)
	}
	st := SnapshotToState(&snap, MapOptions{})
	if !*st.InverterAvailable || st.Booleans["only_charging"] != false || st.EVPower != 3200 || st.CarSOC != 0 || !st.TelemetryAvailable["car_soc"] || !st.ESSMode.IsExternal || st.EVChargingPower != 0 || !st.TelemetryAvailable["ev_charging_kw"] {
		t.Fatalf("IGW contract: %+v", st)
	}
	if len(st.UIConfig["header_toggles"].([]interface{})) != 0 {
		t.Fatal("explicit empty header configuration replaced")
	}
	c := mqtt.NewClient("", 0)
	c.EnableGatewayMode()
	c.SetGatewayConnected(true)
	c.ApplyState(st)
	if !c.CanControlInverter() {
		t.Fatal("IGW-only controller incorrectly requires MQTT")
	}
	if err := json.Unmarshal([]byte(`{"inverter":null}`), &snap); err != nil {
		t.Fatal(err)
	}
	c.ApplyState(SnapshotToState(&snap, MapOptions{}))
	st = c.GetState()
	if *st.InverterAvailable || c.CanControlInverter() || len(st.Booleans) != 0 || st.ESSMode.ModeName != "" || st.EVPresent || st.TelemetryAvailable["car_soc"] {
		t.Fatalf("explicit null/empty snapshot retained stale state: %+v", st)
	}
}

func TestIGWLegacyOmissionAndConfiguredZero(t *testing.T) {
	var legacy Snapshot
	if err := json.Unmarshal([]byte(`{"ev":{"0/Soc":35,"1/Soc":90},"evcharger":{"0/Ac/Power":0,"2/Ac/Power":7000}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	st := SnapshotToState(&legacy, MapOptions{EVInstance: 0, EVChargerInstance: 0, EVInstancesConfigured: true})
	if legacy.InverterPresent || st.InverterAvailable != nil || st.CarSOC != 35 || st.EVChargingKW != 0 {
		t.Fatalf("legacy omission/explicit instance0 broken: %+v", st)
	}
	st = SnapshotToState(&legacy, MapOptions{EVInstance: 999, EVChargerInstance: 999, EVInstancesConfigured: true})
	if st.TelemetryAvailable["car_soc"] || st.TelemetryAvailable["ev_charging_kw"] {
		t.Fatal("IGW explicit pin silently changed devices")
	}
}

func TestIGWControllerCommandValidation(t *testing.T) {
	for _, name := range []string{"toggle", "dry_run", "ess_mode"} {
		if mapped, ok := MapDashboardAction(name); !ok || mapped != name {
			t.Fatalf("missing mapping %s", name)
		}
	}
	body, err := normalizeControllerCommand("toggle", map[string]interface{}{"entity": "input_boolean.only_charging", "state": false})
	if err != nil || body.(map[string]interface{})["state"] != "off" || body.(map[string]interface{})["entity"] != "only_charging" {
		t.Fatal("flag normalization failed")
	}
	for _, tc := range []struct {
		name string
		body map[string]interface{}
	}{
		{"toggle", map[string]interface{}{"entity": "switch.only_charging", "state": true}},
		{"toggle", map[string]interface{}{"entity": "only_charging"}},
		{"toggle", map[string]interface{}{"entity": "only_charging", "state": 2}},
		{"dry_run", map[string]interface{}{"value": "true"}},
		{"ess_mode", map[string]interface{}{"value": true}},
	} {
		if _, err := normalizeControllerCommand(tc.name, tc.body); err == nil {
			t.Fatalf("accepted invalid %s %v", tc.name, tc.body)
		}
	}
}

func TestNativeUnavailableESSDoesNotReviveControllerMirror(t *testing.T) {
	for _, settings := range []string{`{"0/Settings/CGwacs/Hub4Mode":null}`, `{"0/Settings/CGwacs/Hub4Mode":1,"0/Settings/CGwacs/BatteryLife/State":null}`} {
		var snap Snapshot
		body := `{"inverter":{"ess_mode":{"mode_name":"External control","is_external":true}},"settings":` + settings + `}`
		if err := json.Unmarshal([]byte(body), &snap); err != nil {
			t.Fatal(err)
		}
		st := SnapshotToState(&snap, MapOptions{})
		if st.ESSMode.ModeName != "" || st.ESSMode.IsExternal || st.TelemetryAvailable["ess_mode"] {
			t.Fatalf("unavailable native ESS revived daemon copy: %+v", st.ESSMode)
		}
	}
}
