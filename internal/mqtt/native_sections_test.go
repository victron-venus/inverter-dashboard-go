package mqtt

import (
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"testing"
	"time"
)

func TestNativeSectionsNeverUseDaemonMirrors(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"loads": map[string]interface{}{"Oven": 999}, "water_level": 99, "pump_mode": 1, "water_valve": true, "battery_soc": 99})
	c.stateMu.Unlock()
	st := c.GetState()
	for _, key := range []string{"loads", "water_level", "pump_mode", "water_valve", "battery_soc"} {
		if st.TelemetryAvailable[key] {
			t.Fatalf("daemon owns native %s", key)
		}
	}
	if len(st.Loads) != 0 || st.WaterLevel != 0 || st.PumpMode != 0 || st.WaterValve || st.BatterySOC != 0 {
		t.Fatal("daemon mirror leaked into native state")
	}
	if c.TransportStatus()["telemetry"].(map[string]interface{})["quality"] != "unknown" {
		t.Fatal("daemon refreshed native telemetry")
	}
}

func TestNativeLoadNamesDoNotChangeIdentity(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "acload/81", "CustomName", "Kitchen")
	send(c, "acload/82", "CustomName", "Kitchen")
	send(c, "acload/81", "Ac/Power", 420)
	send(c, "acload/82", "Ac/Power", -250)
	st := c.GetState()
	if len(st.Loads) != 2 || st.Loads["81"] != 420 || st.Loads["82"] != -250 || st.LoadNames["82"] != "Kitchen" {
		t.Fatal("duplicate display names merged or generation lost")
	}
	send(c, "acload/82", "CustomName", "Solar")
	if c.GetState().Loads["82"] != -250 || len(c.GetState().Loads) != 2 {
		t.Fatal("rename changed identity")
	}
	send(c, "acload/81", "Connected", 0)
	if len(c.GetState().Loads) != 1 {
		t.Fatal("disconnected load retained")
	}
}

func TestGatewayWaterUsesCapabilityReadbackAndConfiguredInstance(t *testing.T) {
	c := NewClient("", 0)
	c.SetWaterConfig("", 21, 0, 8)
	c.EnableGatewayMode()
	c.SetGatewayConnected(true)
	calls := 0
	c.SetGatewayPublisher(func(action string, payload interface{}) error {
		calls++
		p := payload.(map[string]interface{})
		if action != "water_mode" || p["instance"] != 0 || p["mode"] != 1 {
			t.Fatalf("wrong native water route: %s %v", action, p)
		}
		return nil
	})
	st := &state.State{TelemetryAvailable: map[string]bool{"pump_mode": true}, PumpMode: 2}
	c.ApplyState(st)
	if c.CanControlWater() || c.SetWaterMode("pump", 1) == nil {
		t.Fatal("older gateway accepted water")
	}
	st.GatewayCapabilities = map[string]bool{"water_mode": true}
	c.ApplyState(st)
	if !c.CanControlWaterDevice("pump") || c.CanControlWaterDevice("valve") {
		t.Fatal("per-device availability incorrect")
	}
	if err := c.SetWaterMode("pump", 1); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || c.GetState().PumpMode != 2 {
		t.Fatal("command predicted readback")
	}
	c.SetGatewayConnected(false)
	if c.SetWaterMode("pump", 1) == nil || c.CanControlWater() {
		t.Fatal("disconnected water accepted")
	}
	c.SetGatewayConnected(true)
	c.ApplyState(&state.State{GatewayCapabilities: map[string]bool{"water_mode": true}})
	if c.SetWaterMode("pump", 1) == nil {
		t.Fatal("removed device accepted")
	}
}

func TestTransportStatusDistinguishesIGWAndFreshness(t *testing.T) {
	c := NewClient("", 0)
	c.EnableGatewayMode()
	c.SetGatewayConnected(true)
	c.ApplyState(&state.State{})
	status := c.TransportStatus()
	if status["data_source"] != "igw" || status["mqtt_connected"] != false || status["native_connected"] != true || status["gateway_connected"] != true {
		t.Fatal("IGW mislabeled MQTT")
	}
	if status["telemetry"].(map[string]interface{})["quality"] != "live" {
		t.Fatal("new snapshot not live")
	}
	c.stateMu.Lock()
	c.nativeLastSeen = time.Now().Add(-121 * time.Second)
	c.stateMu.Unlock()
	if c.TransportStatus()["telemetry"].(map[string]interface{})["quality"] != "stale" {
		t.Fatal("old snapshot not stale")
	}
	c.SetGatewayConnected(false)
	if c.TransportStatus()["native_connected"] != false {
		t.Fatal("failed IGW poll still connected")
	}
}

func TestWaterRejectsInvalidModesAndPreservesPercent(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	send(c, "tank/21", "Level", .5)
	send(c, "pump/1", "Mode", 0)
	if c.GetState().WaterLevel != .5 || !c.GetState().TelemetryAvailable["water_pump_mode"] {
		t.Fatal("water percent/zero mode lost")
	}
	for _, mode := range []float64{-1, .5, 3} {
		send(c, "pump/1", "Mode", mode)
		if c.GetState().TelemetryAvailable["pump_mode"] || c.GetState().TelemetryAvailable["water_pump_mode"] {
			t.Fatalf("invalid mode accepted: %v", mode)
		}
	}
}

func TestExplicitUnknownConnectionInvalidatesNativeDevices(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	c.client = &recordingBroker{}
	send(c, "pump/1", "Mode", 0)
	if !c.CanControlWaterDevice("pump") {
		t.Fatal("legacy device without Connected rejected")
	}
	send(c, "pump/1", "Connected", nil)
	if c.CanControlWaterDevice("pump") || c.SetWaterMode("pump", 1) == nil {
		t.Fatal("unknown connection retained water command availability")
	}
	send(c, "pump/1", "Connected", 1)
	if !c.CanControlWaterDevice("pump") {
		t.Fatal("reconnected device unavailable")
	}
}
