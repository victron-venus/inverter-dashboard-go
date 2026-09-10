package state

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSolarForecastPassthrough(t *testing.T) {
	in := []byte(`{"solar_forecast":{"date":"2026-08-23","today_kwh":12.5,"tomorrow_kwh":9.1},"battery_soc":80}`)
	var s State
	if err := json.Unmarshal(in, &s); err != nil {
		t.Fatal(err)
	}
	if s.SolarForecast == nil || s.SolarForecast.TodayKWh != 12.5 {
		t.Fatalf("forecast not decoded: %+v", s.SolarForecast)
	}
	out, _ := json.Marshal(s)
	if !strings.Contains(string(out), `"solar_forecast"`) {
		t.Fatalf("forecast not re-encoded: %s", out)
	}
	// Absent forecast must stay omitted, not serialized as null.
	var empty State
	eout, _ := json.Marshal(empty)
	if strings.Contains(string(eout), "solar_forecast") {
		t.Fatalf("empty forecast leaked into payload: %s", eout)
	}
}

func TestCloneDeepCopiesMaps(t *testing.T) {
	orig := &State{
		Booleans:   map[string]interface{}{"only_charging": true},
		Loads:      map[string]float64{"Oven": 420},
		BatterySOC: 80,
	}
	cp := orig.Clone()
	if cp == nil || cp == orig {
		t.Fatalf("Clone returned bad pointer: %p (orig %p)", cp, orig)
	}
	cp.Loads["Oven"] = 999
	cp.Booleans["only_charging"] = false
	cp.BatterySOC = 10
	if orig.Loads["Oven"] != 420 {
		t.Fatalf("mutating clone affected original Loads: %v", orig.Loads["Oven"])
	}
	if orig.Booleans["only_charging"] != true {
		t.Fatalf("mutating clone affected original Booleans: %v", orig.Booleans["only_charging"])
	}
	if orig.BatterySOC != 80 {
		t.Fatalf("mutating clone affected original BatterySOC: %v", orig.BatterySOC)
	}
}

func TestCloneNil(t *testing.T) {
	var s *State
	if s.Clone() != nil {
		t.Fatal("Clone(nil) should return nil")
	}
}
