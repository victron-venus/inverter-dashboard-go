package homeassistant

import (
	"testing"
)

func TestParseSwitchEntitiesWithStringArrays(t *testing.T) {
	input := map[string]interface{}{
		"home_recliner": []string{"switch.recliner_recliner", "Recliner"},
		"home_garage":   []string{"switch.garage_opener_l", "Garage"},
	}

	result := parseSwitchEntities(input)

	if len(result) != 2 {
		t.Errorf("Expected 2 buttons, got %d", len(result))
	}

	recliner, ok := result["home_recliner"]
	if !ok {
		t.Fatal("home_recliner not found in result")
	}
	if recliner.Entity != "switch.recliner_recliner" {
		t.Errorf("Expected entity 'switch.recliner_recliner', got %s", recliner.Entity)
	}
	if recliner.Label != "Recliner" {
		t.Errorf("Expected label 'Recliner', got %s", recliner.Label)
	}
}

func TestIsOn(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"on", true},
		{"ON", true},
		{"On", true},
		{"true", true},
		{"yes", true},
		{"1", true},
		{"off", false},
		{"OFF", false},
		{"false", false},
		{"unavailable", false},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isOn(tt.input)
		if got != tt.want {
			t.Errorf("isOn(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseStateToSeconds(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"unavailable", 0},
		{"unknown", 0},
		{"None", 0},
		{"0", 0},
		{"3600", 3600},
		{"3661", 3661},
		{"01:00:00", 3600},
		{"01:01:01", 3661},
		{"59:59", 3599},
		{"05:30", 330},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseStateToSeconds(tt.input)
		if got != tt.want {
			t.Errorf("parseStateToSeconds(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseApplianceField(t *testing.T) {
	tests := []struct {
		key      string
		entityID string
		raw      string
		want     interface{}
	}{
		// Boolean domain
		{"dishwasher_running", "binary_sensor.dishwasher_running", "on", true},
		{"dishwasher_running", "binary_sensor.dishwasher_running", "off", false},
		// Sensor with _power suffix -> bool
		{"washer_power", "sensor.washer_power", "1500", true},
		{"washer_power", "sensor.washer_power", "0", false},
		{"washer_power", "sensor.washer_power", "0.5", false},
		// Sensor with _time suffix -> seconds
		{"washer_time", "sensor.washer_time", "1800", 1800},
		{"washer_time", "sensor.washer_time", "01:30:00", 5400},
		{"dryer_time", "sensor.dryer_time", "00:45:00", 2700},
		// Unavailable -> 0
		{"washer_time", "sensor.washer_time", "unavailable", 0},
		// Sensor with _duration suffix
		{"dishwasher_duration", "sensor.dishwasher_duration", "3600", 3600},
	}
	for _, tt := range tests {
		got := parseApplianceField(tt.key, tt.entityID, tt.raw)
		if got != tt.want {
			t.Errorf("parseApplianceField(%q, %q, %q) = %v (%T), want %v (%T)", tt.key, tt.entityID, tt.raw, got, got, tt.want, tt.want)
		}
	}
}

func TestGenerateDefaultLabel(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"only_charging", "ONLY CHARGING"},
		{"home_recliner", "RECLINER"},
		{"home_garage", "GARAGE"},
		{"no_feed", "NO FEED"},
		{"simple", "SIMPLE"},
	}
	for _, tt := range tests {
		got := generateDefaultLabel(tt.key)
		if got != tt.want {
			t.Errorf("generateDefaultLabel(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
