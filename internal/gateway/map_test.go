package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) *Snapshot {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return &snap
}

func TestSnapshotToStateCoreTiles(t *testing.T) {
	st := SnapshotToState(loadFixture(t, "snapshot_core.json"), MapOptions{})
	if st.GT != 15.0 {
		t.Errorf("gt = %v, want 15", st.GT)
	}
	if st.TT != 150.0 {
		t.Errorf("tt = %v, want 150", st.TT)
	}
	if st.Setpoint != -2100.0 {
		t.Errorf("setpoint = %v, want -2100", st.Setpoint)
	}
	if st.InverterState != "Bulk" {
		t.Errorf("inverter_state = %q, want Bulk", st.InverterState)
	}
	if st.BatteryPower != 1258.4 {
		t.Errorf("battery_power = %v, want 1258.4 (shunt)", st.BatteryPower)
	}
	if st.BatteryCurrent != 23.5 {
		t.Errorf("battery_current = %v, want 23.5", st.BatteryCurrent)
	}
	if st.BatterySOC == 0 {
		t.Errorf("battery_soc empty")
	}
	if len(st.Batteries) != 2 {
		t.Errorf("batteries len = %d, want 2", len(st.Batteries))
	}
	if st.Batteries[0].TimeToGo != "40h 48m" {
		t.Errorf("time_to_go = %q, want 40h 48m", st.Batteries[0].TimeToGo)
	}
}

func TestSnapshotToStateSolarTotal(t *testing.T) {
	st := SnapshotToState(loadFixture(t, "snapshot_core.json"), MapOptions{})
	wantMPPT := 403.57 + 594.33 + 455.39
	if st.MpptTotal != wantMPPT {
		t.Errorf("mppt_total = %v, want %v", st.MpptTotal, wantMPPT)
	}
	wantSolar := wantMPPT + 235.0 + 278.0
	if st.SolarTotal != wantSolar {
		t.Errorf("solar_total = %v, want %v", st.SolarTotal, wantSolar)
	}
	if len(st.MPPTChargers) != 3 {
		t.Errorf("mppt_chargers = %d, want 3", len(st.MPPTChargers))
	}
	if len(st.PvInverters) != 2 {
		t.Errorf("pv_inverters = %d, want 2", len(st.PvInverters))
	}
}

func TestSnapshotToStateWaterEVLoads(t *testing.T) {
	st := SnapshotToState(loadFixture(t, "snapshot_core.json"), MapOptions{
		TankInstance: 21, PumpInstance: 1, ValveInstance: 2,
		EVInstance: 22, EVChargerInstance: 40,
	})
	if st.WaterLevel != 91.0 {
		t.Errorf("water_level = %v, want 91", st.WaterLevel)
	}
	if st.EVChargingKW != 3.2 {
		t.Errorf("ev_charging_kw = %v, want 3.2", st.EVChargingKW)
	}
	if st.EVPower != 7.2 {
		t.Errorf("ev_power = %v, want 7.2", st.EVPower)
	}
	if st.Loads["Oven"] != 420.0 {
		t.Errorf("loads Oven = %v, want 420", st.Loads["Oven"])
	}
}

func TestBankPrefersShuntOverSystem(t *testing.T) {
	st := SnapshotToState(loadFixture(t, "snapshot_core.json"), MapOptions{})
	if st.BatteryCurrent == 25.2 {
		t.Fatal("bank current followed system aggregate instead of shunt")
	}
	if st.BatteryCurrent != 23.5 || st.BatteryPower != 1258.4 {
		t.Fatalf("bank V/I/P not from shunt: I=%v P=%v", st.BatteryCurrent, st.BatteryPower)
	}
}

func TestCommandWhitelist(t *testing.T) {
	if !IsWhitelistedCommand("silence_alarm") {
		t.Fatal("silence_alarm should be whitelisted")
	}
	if IsWhitelistedCommand("setpoint") {
		t.Fatal("setpoint must not be whitelisted")
	}
	if _, ok := MapDashboardAction("setpoint"); ok {
		t.Fatal("setpoint has no IGW mapping")
	}
	if name, ok := MapDashboardAction("acknowledge_all_notifications"); !ok || name != "acknowledge_all_notifications" {
		t.Fatalf("ack map = %q %v", name, ok)
	}
}

func TestHidesTimeToGoWhenIdle(t *testing.T) {
	raw := []byte(`{
	  "battery": {
	    "289/ProductName": "SmartShunt 500A/50mV",
	    "289/Dc/0/Current": 0.1,
	    "289/TimeToGo": 146880.0
	  }
	}`)
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	st := SnapshotToState(&snap, MapOptions{})
	if len(st.Batteries) != 1 {
		t.Fatalf("batteries = %d", len(st.Batteries))
	}
	if st.Batteries[0].State != "Idle" {
		t.Errorf("state = %q", st.Batteries[0].State)
	}
	if st.Batteries[0].TimeToGo != "" {
		t.Errorf("time_to_go should be empty when idle, got %q", st.Batteries[0].TimeToGo)
	}
}

func TestSnapshotToStatePlatformNotifications(t *testing.T) {
	st := SnapshotToState(loadFixture(t, "snapshot_platform_notifs.json"), MapOptions{})
	if len(st.Notifications) != 1 {
		t.Fatalf("notifications = %+v, want 1 active (acked skipped)", st.Notifications)
	}
	n := st.Notifications[0]
	if n.ID != "victron-platform-0-3" {
		t.Errorf("id = %q", n.ID)
	}
	if n.Level != "alarm" {
		t.Errorf("level = %q, want alarm", n.Level)
	}
	if n.Title != "High cell voltage" {
		t.Errorf("title = %q", n.Title)
	}
	if n.Body != "Lynx Smart BMS" {
		t.Errorf("body = %q", n.Body)
	}
}

func TestSnapshotToStateAlarmFallbackWhenNoPlatform(t *testing.T) {
	snap := loadFixture(t, "snapshot_platform_notifs.json")
	snap.Platform = nil
	st := SnapshotToState(snap, MapOptions{})
	if len(st.Notifications) < 1 {
		t.Fatalf("expected alarm fallback banners, got %+v", st.Notifications)
	}
	found := false
	for _, n := range st.Notifications {
		if n.Level == "alarm" || n.Level == "warning" {
			found = true
			if !strings.HasPrefix(n.ID, "victron-") {
				t.Errorf("id = %q", n.ID)
			}
		}
	}
	if !found {
		t.Fatalf("no alarm/warning in %+v", st.Notifications)
	}
}

func TestMapDashboardActionAliases(t *testing.T) {
	if name, ok := MapDashboardAction("dismiss_banner"); !ok || name != "acknowledge_all_notifications" {
		t.Fatalf("dismiss_banner -> %q ok=%v", name, ok)
	}
	if name, ok := MapDashboardAction("acknowledge_victron_banner"); !ok || name != "acknowledge_all_notifications" {
		t.Fatalf("acknowledge_victron_banner -> %q ok=%v", name, ok)
	}
}
