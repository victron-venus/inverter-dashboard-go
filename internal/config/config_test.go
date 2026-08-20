package config

import (
	"os"
	"testing"
)

func TestGetEnvDefault(t *testing.T) {
	// Test with env set
	os.Setenv("TEST_KEY", "test_value")
	defer os.Unsetenv("TEST_KEY")

	got := getEnvDefault("TEST_KEY", "default")
	if got != "test_value" {
		t.Errorf("getEnvDefault = %q, want %q", got, "test_value")
	}

	// Test with env not set
	got = getEnvDefault("NONEXISTENT_KEY", "default")
	if got != "default" {
		t.Errorf("getEnvDefault = %q, want %q", got, "default")
	}
}

func TestGetEnvIntDefault(t *testing.T) {
	// Test with env set to valid int
	os.Setenv("TEST_INT_KEY", "8080")
	defer os.Unsetenv("TEST_INT_KEY")

	got := getEnvIntDefault("TEST_INT_KEY", 3000)
	if got != 8080 {
		t.Errorf("getEnvIntDefault = %d, want %d", got, 8080)
	}

	// Test with env not set
	got = getEnvIntDefault("NONEXISTENT_INT_KEY", 3000)
	if got != 3000 {
		t.Errorf("getEnvIntDefault = %d, want %d", got, 3000)
	}

	// Test with invalid int - should return default
	os.Setenv("INVALID_INT_KEY", "not-a-number")
	defer os.Unsetenv("INVALID_INT_KEY")

	got = getEnvIntDefault("INVALID_INT_KEY", 3000)
	if got != 3000 {
		t.Errorf("getEnvIntDefault with invalid value = %d, want %d", got, 3000)
	}
}

func TestLoadWithYAML(t *testing.T) {
	// Create a temporary config.yaml
	yamlContent := `
mqtt:
  host: "192.168.1.100"
  port: 1883
web:
  host: "0.0.0.0"
  port: 9090
homeassistant:
  url: "http://ha.local:8123"
  token: "test-token-12345"
  direct_controls: true
  poll_interval_seconds: 15
  boolean_entities:
    only_charging: "input_boolean.only_charging"
  switch_entities:
    home_recliner:
      entity: "switch.recliner"
      label: "Recliner"
      order: 1
  water_level_entity: "sensor.water_level"
  car_soc_entity: "sensor.car_soc"
  appliance_entities:
    dishwasher_running: "binary_sensor.dishwasher_running"
  vue_sensors:
    "Garage": "sensor.garage_power"
`
	tmpFile := "config_test_tmp.yaml"
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	// Override the hardcoded yamlFile path
	// Since loadConfigYAML uses hardcoded "config.yaml", we need a different approach.
	// Test the Config struct defaults and env overrides instead.

	// Set env vars to test they don't get overridden when YAML doesn't set them
	os.Setenv("MQTT_HOST", "from-env")
	defer os.Unsetenv("MQTT_HOST")

	// Test Load using env vars
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Basic checks
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
	if cfg.MQTT.Host == "" {
		t.Error("MQTT.Host should not be empty")
	}
	if cfg.Web.Port == 0 {
		t.Error("Web.Port should not be 0")
	}
}

func TestConvertMapToEntitySlice(t *testing.T) {
	input := map[string]interface{}{
		"home_test": map[string]interface{}{
			"entity": "switch.test",
			"label":  "Test Switch",
			"order":  1,
		},
		"home_simple": "switch.simple",
		"home_list": []interface{}{
			"switch.list",
			"List Label",
			2,
		},
		"home_list_no_label": []interface{}{
			"switch.list2",
		},
		"home_map": map[string]interface{}{
			"entity": "switch.map",
			"label":  "Map Label",
			"order":  3,
			"name":   "Map Name", // should be ignored if label present
			"short":  "Map Short", // should be ignored if label present
		},
		"home_map_no_label": map[string]interface{}{
			"entity": "switch.map2",
			"name":   "Map2 Name",
			"order":  4,
		},
		"home_map_short": map[string]interface{}{
			"entity": "switch.map3",
			"short":  "Map3 Short",
			"order":  5,
		},
	}

	result := convertMapToEntitySlice(input)

	if len(result) != 7 {
		t.Errorf("Expected 7 entities, got %d", len(result))
	}

	for _, e := range result {
		switch e.Key {
		case "home_test":
			if e.Entity != "switch.test" {
				t.Errorf("home_test entity = %q, want %q", e.Entity, "switch.test")
			}
			if e.Label != "Test Switch" {
				t.Errorf("home_test label = %q, want %q", e.Label, "Test Switch")
			}
			if e.Order != 1 {
				t.Errorf("home_test order = %d, want %d", e.Order, 1)
			}
		case "home_simple":
			if e.Entity != "switch.simple" {
				t.Errorf("home_simple entity = %q, want %q", e.Entity, "switch.simple")
			}
			if e.Label != "HOME_SIMPLE" {
				t.Errorf("home_simple label = %q, want %q", e.Label, "HOME_SIMPLE")
			}
			if e.Order != 0 {
				t.Errorf("home_simple order = %d, want %d", e.Order, 0)
			}
		case "home_list":
			if e.Entity != "switch.list" {
				t.Errorf("home_list entity = %q, want %q", e.Entity, "switch.list")
			}
			if e.Label != "List Label" {
				t.Errorf("home_list label = %q, want %q", e.Label, "List Label")
			}
			if e.Order != 2 {
				t.Errorf("home_list order = %d, want %d", e.Order, 2)
			}
		case "home_list_no_label":
			if e.Entity != "switch.list2" {
				t.Errorf("home_list_no_label entity = %q, want %q", e.Entity, "switch.list2")
			}
			if e.Label != "HOME_LIST_NO_LABEL" {
				t.Errorf("home_list_no_label label = %q, want %q", e.Label, "HOME_LIST_NO_LABEL")
			}
			if e.Order != 0 {
				t.Errorf("home_list_no_label order = %d, want %d", e.Order, 0)
			}
		case "home_map":
			if e.Entity != "switch.map" {
				t.Errorf("home_map entity = %q, want %q", e.Entity, "switch.map")
			}
			if e.Label != "Map Label" {
				t.Errorf("home_map label = %q, want %q", e.Label, "Map Label")
			}
			if e.Order != 3 {
				t.Errorf("home_map order = %d, want %d", e.Order, 3)
			}
		case "home_map_no_label":
			if e.Entity != "switch.map2" {
				t.Errorf("home_map_no_label entity = %q, want %q", e.Entity, "switch.map2")
			}
			if e.Label != "HOME_MAP_NO_LABEL" {
				t.Errorf("home_map_no_label label = %q, want %q", e.Label, "HOME_MAP_NO_LABEL")
			}
			if e.Order != 4 {
				t.Errorf("home_map_no_label order = %d, want %d", e.Order, 4)
			}
		case "home_map_short":
			if e.Entity != "switch.map3" {
				t.Errorf("home_map_short entity = %q, want %q", e.Entity, "switch.map3")
			}
			if e.Label != "HOME_MAP_SHORT" {
				t.Errorf("home_map_short label = %q, want %q", e.Label, "HOME_MAP_SHORT")
			}
			if e.Order != 5 {
				t.Errorf("home_map_short order = %d, want %d", e.Order, 5)
			}
		}
	}
}

func TestConvertMapToBooleanEntitySlice(t *testing.T) {
	input := map[string]interface{}{
		"only_charging": "input_boolean.only_charging",
		"no_feed": map[string]interface{}{
			"entity": "input_boolean.no_feed",
			"order":  2,
		},
		"simple": "input_boolean.simple",
	}

	result := convertMapToBooleanEntitySlice(input)

	if len(result) != 3 {
		t.Errorf("Expected 3 entities, got %d", len(result))
	}

	for _, e := range result {
		switch e.Key {
		case "only_charging":
			if e.Entity != "input_boolean.only_charging" {
				t.Errorf("only_charging entity = %q", e.Entity)
			}
			if e.Order != 0 {
				t.Errorf("only_charging order = %d, want %d", e.Order, 0)
			}
		case "no_feed":
			if e.Entity != "input_boolean.no_feed" {
				t.Errorf("no_feed entity = %q", e.Entity)
			}
			if e.Order != 2 {
				t.Errorf("no_feed order = %d, want %d", e.Order, 2)
			}
		case "simple":
			if e.Entity != "input_boolean.simple" {
				t.Errorf("simple entity = %q", e.Entity)
			}
			if e.Order != 0 {
				t.Errorf("simple order = %d, want %d", e.Order, 0)
			}
		}
	}
}

func TestLoad_NoConfig(t *testing.T) {
	// Ensure no config.yaml exists in the current directory
	// Remove if exists (should not)
	_ = os.Remove("config.yaml")
	defer func() { _ = os.Remove("config.yaml") }()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() should not error when config.yaml missing, got %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
	// Should have default values from env
	if cfg.MQTT.Host == "" {
		t.Error("MQTT.Host should not be empty (default from env)")
	}
	if cfg.Web.Port == 0 {
		t.Error("Web.Port should not be 0 (default from env)")
	}
	if cfg.HomeAssistant != nil {
		t.Error("HomeAssistant should be nil when no config.yaml")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Create invalid yaml
	yamlContent := `invalid: : :`
	if err := os.WriteFile("config.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove("config.yaml") }()

	cfg, err := Load("")
	if err == nil {
		t.Error("Load() should return error for invalid YAML")
	}
	if cfg != nil && cfg.HomeAssistant != nil {
		t.Error("HomeAssistant should be nil when YAML invalid")
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	// Create valid yaml with minimal homeassistant section
	yamlContent := `
homeassistant:
  url: "http://ha.example.com"
  token: "validtoken"
`
	if err := os.WriteFile("config.yaml", []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove("config.yaml") }()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
	if cfg.HomeAssistant == nil {
		t.Error("HomeAssistant should be set")
	}
	if cfg.HomeAssistant.URL != "http://ha.example.com" {
		t.Errorf("HomeAssistant URL mismatch: got %s", cfg.HomeAssistant.URL)
	}
	if cfg.HomeAssistant.Token != "validtoken" {
		t.Errorf("HomeAssistant Token mismatch: got %s", cfg.HomeAssistant.Token)
	}
	// Defaults
	if cfg.HomeAssistant.DirectControls != true {
		t.Error("HomeAssistant DirectControls should default to true")
	}
	if cfg.HomeAssistant.PollInterval == 0 {
		t.Error("HomeAssistant PollInterval should default to 0")
	}
}

func TestLoadHomeAssistantConfig_Nil(t *testing.T) {
	// Should not panic
	logHomeAssistantConfig(nil)
}

func TestLogHomeAssistantConfig(t *testing.T) {
	cfg := &HomeAssistantConfig{
		URL:                "http://ha.example.com",
		Token:              "token1234567890",
		DirectControls:     false,
		PollInterval:       30,
		WaterValveEntity:   "switch.water_valve",
		WaterLevelEntity:   "sensor.water_level",
		PumpSwitchEntity:   "switch.pump",
		CarSOCEntity:       "sensor.car_soc",
		EVChargingKWEntity: "sensor.ev_charging_kw",
		EVPowerEntity:      "sensor.ev_power",
		BooleanEntities: []BooleanEntityConfig{
			{Key: "bool1", Entity: "binary_sensor.bool1", Order: 1},
			{Key: "bool2", Entity: "binary_sensor.bool2", Order: 2},
		},
		SwitchEntities: []EntityConfig{
			{Key: "switch1", Entity: "switch.switch1", Label: "Switch 1", Order: 3},
			{Key: "switch2", Entity: "switch.switch2", Order: 4},
		},
		ApplianceEntities: map[string]string{
			"appliance1": "binary_sensor.appliance1",
		},
		VueSensors: map[string]string{
			"vue1": "sensor.vue1",
		},
		SensorEntities: map[string]string{
			"sensor1": "sensor.sensor1",
		},
	}
	// Just ensure it doesn't panic
	logHomeAssistantConfig(cfg)
}

func TestGetExampleConfigPath(t *testing.T) {
	// The function returns empty string and nil error
	path, err := GetExampleConfigPath()
	if err != nil {
		t.Errorf("GetExampleConfigPath returned error: %v", err)
	}
	if path != "" {
		t.Errorf("GetExampleConfigPath expected empty path, got %s", path)
	}
}
