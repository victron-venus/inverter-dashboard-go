package mqtt

import (
	"encoding/json"
	"testing"
)

func cerboMsg(topic string, value interface{}) *fakeMessage {
	payload, _ := json.Marshal(map[string]interface{}{"value": value})
	return &fakeMessage{topic: topic, payload: payload}
}

func TestVoltageSOCMatchesDesktop(t *testing.T) {
	if VoltageSOC(40.0) != 0 {
		t.Errorf("40V => %v, want 0", VoltageSOC(40.0))
	}
	if VoltageSOC(54.4) != 100 {
		t.Errorf("54.4V => %v, want 100", VoltageSOC(54.4))
	}
	if VoltageSOC(47.2) != 50 {
		t.Errorf("47.2V => %v, want 50", VoltageSOC(47.2))
	}
}

func TestSlimInverterStateDoesNotClearCerboLoads(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	if !c.handleACLoad("N/p1/acload/81/Ac/Power", cerboMsg("", 420).payload) {
		t.Fatal("acload power not applied")
	}
	if !c.handleACLoad("N/p1/acload/81/CustomName", mustJSON("Oven")) {
		t.Fatal("acload name not applied")
	}
	c.stateMu.Unlock()

	if c.GetState().Loads["Oven"] != 420 {
		t.Fatalf("loads Oven = %v, want 420", c.GetState().Loads["Oven"])
	}

	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{
		"daily_stats": map[string]interface{}{"solar_kwh": 12.5},
		"version":     "9.9.9",
		"loads":       map[string]interface{}{},
		"gt":          0.0,
	})
	c.stateMu.Unlock()

	st := c.GetState()
	if st.Loads["Oven"] != 420 {
		t.Errorf("loads wiped: %v", st.Loads)
	}
	if st.DailyStats.SolarKWh != 12.5 {
		t.Errorf("daily_stats.solar_kwh = %v, want 12.5", st.DailyStats.SolarKWh)
	}
	if st.Version != "9.9.9" {
		t.Errorf("version = %v, want 9.9.9", st.Version)
	}
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(map[string]interface{}{"value": v})
	return b
}

func TestSystemcalcGridAndConsumption(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.handleCerboDevice("N/p1/system/0/Ac/Grid/L1/Power", mustJSON(100))
	c.handleCerboDevice("N/p1/system/0/Ac/Grid/L2/Power", mustJSON(50))
	c.handleCerboDevice("N/p1/system/0/Ac/Consumption/L1/Power", mustJSON(200))
	c.handleCerboDevice("N/p1/system/0/Ac/Consumption/L2/Power", mustJSON(80))
	c.stateMu.Unlock()

	st := c.GetState()
	if st.G1 != 100 || st.G2 != 50 || st.GT != 150 || st.TT != 280 {
		t.Fatalf("grid/cons = g1=%v g2=%v gt=%v tt=%v", st.G1, st.G2, st.GT, st.TT)
	}

	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{
		"daily_stats": map[string]interface{}{"batt_out_kwh": 3.1},
		"gt":          0.0,
		"tt":          0.0,
	})
	c.stateMu.Unlock()

	st = c.GetState()
	if st.GT != 150 || st.TT != 280 {
		t.Errorf("slim wiped Cerbo grid/cons: gt=%v tt=%v", st.GT, st.TT)
	}
	if st.DailyStats.BattOutKWh != 3.1 {
		t.Errorf("batt_out_kwh = %v, want 3.1", st.DailyStats.BattOutKWh)
	}
}

func TestBatteryShuntVoltageSOC(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.handleCerboDevice("N/p1/battery/512/ProductName", mustJSON("SmartShunt 500A"))
	c.handleCerboDevice("N/p1/battery/512/Dc/0/Voltage", mustJSON(47.2))
	c.handleCerboDevice("N/p1/battery/512/Dc/0/Current", mustJSON(-12.5))
	c.handleCerboDevice("N/p1/battery/512/Dc/0/Power", mustJSON(-590))
	c.stateMu.Unlock()

	st := c.GetState()
	if st.BatterySOC != 50 {
		t.Errorf("battery_soc = %v, want 50", st.BatterySOC)
	}
	if st.BatteryVoltage != 47.2 || st.BatteryCurrent != -12.5 || st.BatteryPower != -590 {
		t.Errorf("battery V/I/P = %v %v %v", st.BatteryVoltage, st.BatteryCurrent, st.BatteryPower)
	}
	if len(st.Batteries) != 1 || st.Batteries[0].State != "Discharging" {
		t.Errorf("batteries = %+v", st.Batteries)
	}
}

func TestSolarchargerAndPVMakeSolarTotal(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.handleCerboDevice("N/p1/solarcharger/1/Yield/Power", mustJSON(300))
	c.stateMu.Unlock()

	c.onPvInverterMessage(nil, cerboMsg("N/p1/pvinverter/369/Ac/Power", 200))

	st := c.GetState()
	if st.MpptTotal != 300 {
		t.Errorf("mppt_total = %v, want 300", st.MpptTotal)
	}
	if st.SolarTotal != 500 {
		t.Errorf("solar_total = %v, want 500", st.SolarTotal)
	}
}

func TestVebusSetpointAndMode(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.handleCerboDevice("N/p1/vebus/276/Hub4/L1/AcPowerSetpoint", mustJSON(-500))
	c.handleCerboDevice("N/p1/vebus/276/State", mustJSON(9))
	c.stateMu.Unlock()

	st := c.GetState()
	if st.Setpoint != -500 {
		t.Errorf("setpoint = %v, want -500", st.Setpoint)
	}
	if st.InverterState != "Inverting" {
		t.Errorf("inverter_state = %q, want Inverting", st.InverterState)
	}
}

func TestACLoadPowerUpdatesExistingEntry(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.handleACLoad("N/p1/acload/81/Ac/Power", mustJSON(100))
	c.handleACLoad("N/p1/acload/81/CustomName", mustJSON("Oven"))
	c.handleACLoad("N/p1/acload/81/Ac/Power", mustJSON(455))
	c.stateMu.Unlock()
	if c.GetState().Loads["Oven"] != 455 {
		t.Errorf("Oven = %v, want 455", c.GetState().Loads["Oven"])
	}
}

func TestPortalDiscoveryUpdatesID(t *testing.T) {
	c := NewClient("localhost", 1883)
	if c.PortalID() != "" {
		t.Fatalf("portal should start empty, got %q", c.PortalID())
	}
	// onPortalMessage will try to subscribe; without a connected client it may log warnings.
	// We only assert the ID is stored when message arrives — skip Subscribe side effects
	// by setting portal via the message handler path with nil-safe client check.
	c.onPortalMessage(nil, &fakeMessage{topic: "inverter/portal", payload: []byte("b827ebea1ece")})
	if c.PortalID() != "b827ebea1ece" {
		t.Errorf("portal = %q, want b827ebea1ece", c.PortalID())
	}
}
