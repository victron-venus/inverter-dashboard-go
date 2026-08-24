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
