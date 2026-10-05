package config

import (
	"os"
	"testing"
)

func TestGetEnvDefault(t *testing.T) {
	// Test with env set
	t.Setenv("TEST_KEY", "test_value")

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
	t.Setenv("TEST_INT_KEY", "8080")

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
	t.Setenv("INVALID_INT_KEY", "not-a-number")

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
  appliance_entities:
    dishwasher_running: "binary_sensor.dishwasher_running"
  vue_sensors:
    "Garage": "sensor.garage_power"
`
	t.Chdir(t.TempDir())
	tmpFile := "config.yaml"
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Explicit YAML settings override environment defaults.
	t.Setenv("MQTT_HOST", "from-env")

	// Load the isolated YAML fixture rather than a developer's local config.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Basic checks
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
	if cfg.MQTT.Host != "192.168.1.100" {
		t.Errorf("MQTT.Host = %q, want YAML value", cfg.MQTT.Host)
	}
	if cfg.Web.Port != 9090 {
		t.Errorf("Web.Port = %d, want 9090", cfg.Web.Port)
	}
	if cfg.HomeAssistant == nil || cfg.HomeAssistant.URL != "http://ha.local:8123" {
		t.Fatalf("Home Assistant YAML config was not loaded: %+v", cfg.HomeAssistant)
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
	}

	result := convertMapToEntitySlice(input)

	if len(result) != 2 {
		t.Errorf("Expected 2 entities, got %d", len(result))
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
	}

	result := convertMapToBooleanEntitySlice(input)

	if len(result) != 2 {
		t.Errorf("Expected 2 entities, got %d", len(result))
	}

	for _, e := range result {
		switch e.Key {
		case "only_charging":
			if e.Entity != "input_boolean.only_charging" {
				t.Errorf("only_charging entity = %q", e.Entity)
			}
		case "no_feed":
			if e.Entity != "input_boolean.no_feed" {
				t.Errorf("no_feed entity = %q", e.Entity)
			}
			if e.Order != 2 {
				t.Errorf("no_feed order = %d, want %d", e.Order, 2)
			}
		}
	}
}

func TestGatewayConfigFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_ENABLED", "true")
	t.Setenv("GATEWAY_URL", "https://victron.example")
	t.Setenv("GATEWAY_ACCESS_CLIENT_ID", "id.access")
	t.Setenv("GATEWAY_ACCESS_CLIENT_SECRET", "secret")
	t.Setenv("GATEWAY_API_TOKEN", "tok")
	t.Setenv("MQTT_HOST", "")

	// Avoid picking up a local config.yaml MQTT host during this test.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "cfg-igw-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GatewayConfigured() {
		t.Fatalf("expected gateway configured: %+v", cfg.Gateway)
	}
	if cfg.MQTTConfigured() {
		t.Fatalf("expected mqtt skipped when GATEWAY_ENABLED, host=%q", cfg.MQTT.Host)
	}
	if cfg.Gateway.PollIntervalSec != 2 {
		t.Fatalf("poll interval = %d", cfg.Gateway.PollIntervalSec)
	}
}
