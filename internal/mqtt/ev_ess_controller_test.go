package mqtt

import (
	"encoding/json"
	"testing"
	"time"
)

func controllerMessage(c *Client, payload string) {
	c.onStateMessage(nil, &fakeMessage{topic: "inverter/state", payload: []byte(payload)})
}

func TestEVAutoDiscoveryUsesMeasurementsAndKeepsUnits(t *testing.T) {
	c := NewClient("", 0)
	send(c, "ev/1", "ProductName", "Metadata only")
	send(c, "ev/812", "Soc", 0)
	send(c, "ev/812", "Ac/Power", 3200)
	send(c, "ev/812", "CustomName", "Vehicle")
	send(c, "evcharger/77", "Ac/L1/Power", 1000)
	send(c, "evcharger/77", "Ac/L2/Power", 2200)
	st := c.GetState()
	if st.CarSOC != 0 || !st.TelemetryAvailable["car_soc"] || st.EVPower != 3200 || st.CarChargingPower != 3200 || st.EVChargingPower != 3200 || st.EVChargingKW != 3.2 {
		t.Fatalf("auto EV readings/units: %+v", st)
	}
	if !st.EVPresent || !st.EVChargerPresent || len(st.DiscoveredWaterEV) != 3 || st.DiscoveredWaterEV[1].Name != "Vehicle" {
		t.Fatalf("missing discovery: %+v", st.DiscoveredWaterEV)
	}
	controllerMessage(c, `{"booleans":{"only_charging":false},"car_soc":90,"ev_power":9999}`)
	if st = c.GetState(); st.CarSOC != 0 || st.EVPower != 3200 {
		t.Fatal("controller replaced native zero/EV readings")
	}
	send(c, "ev/812", "Connected", 0)
	if st = c.GetState(); st.TelemetryAvailable["car_soc"] || st.TelemetryAvailable["ev_power"] || len(st.DiscoveredWaterEV) != 2 {
		t.Fatalf("disconnected EV retained values: %+v", st)
	}
}

func TestExplicitEVZeroNeverFallsBackToAnotherVehicle(t *testing.T) {
	c := NewClient("", 0)
	c.SetEVConfig(0, 0)
	send(c, "ev/0", "Soc", 40)
	send(c, "evcharger/0", "Ac/Power", 0)
	send(c, "ev/2", "Soc", 90)
	send(c, "evcharger/2", "Ac/Power", 7000)
	if st := c.GetState(); st.CarSOC != 40 || st.EVChargingKW != 0 || !st.TelemetryAvailable["ev_charging_kw"] {
		t.Fatal("explicit zero instance ignored")
	}
	c.onCerboLiveMessage(nil, &fakeMessage{topic: "N/p1/ev/0", payload: nil})
	send(c, "evcharger/0", "Ac/Power", nil)
	if st := c.GetState(); st.TelemetryAvailable["car_soc"] || st.TelemetryAvailable["ev_charging_kw"] {
		t.Fatal("missing configured instance silently selected another device")
	}
}

func TestEVChargerSOCAndRemovedInventory(t *testing.T) {
	c := NewClient("", 0)
	send(c, "evcharger/71", "Soc", 46)
	send(c, "evcharger/71", "Ac/Power", 0)
	if st := c.GetState(); st.CarSOC != 46 || !st.EVChargerPresent || !st.TelemetryAvailable["ev_charging_kw"] {
		t.Fatal("legacy evcharger SOC/zero not discovered")
	}
	c.onCerboLiveMessage(nil, &fakeMessage{topic: "N/p1/evcharger/71", payload: nil})
	if st := c.GetState(); st.EVChargerPresent || len(st.DiscoveredWaterEV) != 0 || st.TelemetryAvailable["car_soc"] || st.TelemetryAvailable["ev_charging_kw"] {
		t.Fatal("removed EV service stayed available")
	}
	send(c, "evcharger/71", "Soc", 101)
	if c.GetState().TelemetryAvailable["car_soc"] {
		t.Fatal("out-of-range SOC accepted")
	}
}

func TestESSSettingsAndControllerOwnership(t *testing.T) {
	c := NewClient("", 0)
	controllerMessage(c, `{"booleans":{"only_charging":false,"input_boolean.no_feed":"on"},"ess_mode":{"mode_name":"External control","is_external":true}}`)
	if st := c.GetState(); st.ESSMode.ModeName != "External control" || st.Booleans["only_charging"] != false || st.Booleans["no_feed"] != true || st.UIConfig["header_toggles"] == nil {
		t.Fatal("controller ESS/flag metadata missing")
	}
	send(c, "settings/0", "Settings/CGwacs/Hub4Mode", 1)
	send(c, "settings/0", "Settings/CGwacs/BatteryLife/State", 9)
	controllerMessage(c, `{"booleans":{"only_charging":true},"ess_mode":{"mode_name":"External control","is_external":true}}`)
	if st := c.GetState(); st.ESSMode.IsExternal || st.ESSMode.ModeName != "Keep batteries charged" || st.Booleans["no_feed"] != nil {
		t.Fatal("native ESS lost priority or removed flag remained")
	}
	send(c, "settings/0", "Settings/CGwacs/Hub4Mode", nil)
	if st := c.GetState(); st.ESSMode.ModeName != "" || st.TelemetryAvailable["ess_mode"] {
		t.Fatal("null ESS setting retained stale active mode")
	}
}

func TestControllerExpiryAndDisconnectClearFlags(t *testing.T) {
	c := NewClient("", 0)
	controllerMessage(c, `{"booleans":{"only_charging":true},"ess_mode":{"is_external":true,"mode_name":"External control"}}`)
	c.stateMu.Lock()
	c.controllerLastSeen = time.Now().Add(-121 * time.Second)
	c.stateMu.Unlock()
	st := c.GetState()
	if *st.InverterAvailable || len(st.Booleans) != 0 || st.OnlyCharging || st.ESSMode.ModeName != "" {
		t.Fatal("expired controller state remained active")
	}
	controllerMessage(c, `{"booleans":{"only_charging":true}}`)
	c.invalidateCerbo()
	if st = c.GetState(); *st.InverterAvailable || len(st.Booleans) != 0 {
		t.Fatal("disconnect retained controller flags")
	}
	if _, err := json.Marshal(st); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyRetainedControllerMessageClearsImmediately(t *testing.T) {
	c := NewClient("", 0)
	controllerMessage(c, `{"booleans":{"only_charging":true},"ess_mode":{"mode_name":"External control","is_external":true}}`)
	controllerMessage(c, "")
	if st := c.GetState(); st.InverterAvailable == nil || *st.InverterAvailable || len(st.Booleans) != 0 || st.ESSMode.ModeName != "" {
		t.Fatal("retained deletion left controller state active")
	}
}

func TestIdleControllerExpiryBroadcasts(t *testing.T) {
	c := NewClient("", 0)
	controllerMessage(c, `{"booleans":{"only_charging":true}}`)
	updated := make(chan bool, 1)
	c.SetMessageHandler(func() { updated <- *c.GetState().InverterAvailable })
	c.refreshControllerFreshness(time.Now().Add(121 * time.Second))
	select {
	case available := <-updated:
		if available {
			t.Fatal("idle expiry broadcast retained available controller")
		}
	case <-time.After(time.Second):
		t.Fatal("idle controller expiry did not reach existing clients")
	}
}

func TestDaemonEVMirrorsNeverSupplyNativeReadingsAtStartup(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{
		"car_soc": 99.0, "ev_power": 7400.0, "car_charging_power": 7400.0,
		"ev_charging_kw": 7.4, "ev_charging_power": 7400.0, "ev_present": true,
		"evcharger_present": true, "discovered_water_ev": []interface{}{map[string]interface{}{"kind": "ev", "instance": 22}},
		"booleans":  map[string]interface{}{"only_charging": true},
		"ui_config": "malformed optional metadata",
	})
	c.stateMu.Unlock()
	st := c.GetState()
	if st.Booleans["only_charging"] != true {
		t.Fatal("malformed metadata suppressed valid flags")
	}
	for _, key := range []string{"car_soc", "ev_power", "car_charging_power", "ev_charging_kw", "ev_charging_power"} {
		if st.TelemetryAvailable[key] {
			t.Errorf("daemon mirror became native telemetry: %s", key)
		}
	}
	if len(st.DiscoveredWaterEV) != 0 {
		t.Fatal("daemon supplied native device inventory")
	}
}
