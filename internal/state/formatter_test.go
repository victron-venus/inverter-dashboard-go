package state

import (
	"testing"
)

func TestFormatESSMode(t *testing.T) {
	tests := []struct {
		mode ESSMode
		want string
	}{
		{ESSMode{ModeName: "External control"}, "External Control"},
		{ESSMode{ModeName: "Self consumption"}, "Self Consumption"},
		{ESSMode{ModeName: "Keep batteries charged"}, "Keep Charged"},
		{ESSMode{ModeName: "Optimized"}, "Optimized"},
		{ESSMode{ModeName: ""}, ""},
	}

	for _, tt := range tests {
		got := FormatESSMode(tt.mode)
		if got != tt.want {
			t.Errorf("FormatESSMode(%+v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestFormatHeaderToggles(t *testing.T) {
	toggles := FormatHeaderToggles()
	if len(toggles) != 8 {
		t.Errorf("Expected 8 toggles, got %d", len(toggles))
	}

	// Check first toggle
	if toggles[0]["id"] != "dry_run" {
		t.Errorf("First toggle id = %q, want %q", toggles[0]["id"], "dry_run")
	}

	// Check all toggles have required fields
	for i, toggle := range toggles {
		if toggle["id"] == "" {
			t.Errorf("Toggle %d has empty id", i)
		}
		if toggle["label"] == "" {
			t.Errorf("Toggle %d has empty label", i)
		}
		if toggle["entity"] == "" {
			t.Errorf("Toggle %d has empty entity", i)
		}
	}
}

func TestFormatDailyStats(t *testing.T) {
	s := &State{
		DailyStats: DailyStats{
			SolarKWh:   25.5,
			SolarMoney: 5.10,
			GridKWh:    10.2,
			GridMoney:  3.16,
			BattInKWh:  15.0,
			BattOutKWh: 12.0,
			BattNetKWh: 3.0,
		},
	}

	stats := s.FormatDailyStats()
	if stats == nil {
		t.Fatal("FormatDailyStats returned nil")
	}

	if stats["solar_kwh"] != 25.5 {
		t.Errorf("solar_kwh = %v, want 25.5", stats["solar_kwh"])
	}
	if stats["batt_net_kwh"] != 3.0 {
		t.Errorf("batt_net_kwh = %v, want 3.0", stats["batt_net_kwh"])
	}
}
