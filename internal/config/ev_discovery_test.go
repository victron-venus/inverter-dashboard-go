package config

import (
	"os"
	"testing"
)

func TestEVDefaultsAutoAndExplicitZeroPersists(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("EV_INSTANCE", "")
	t.Setenv("EVCHARGER_INSTANCE", "")
	t.Setenv("GATEWAY_ENABLED", "true")
	t.Setenv("MQTT_HOST", "")
	t.Setenv("MQTT_PORT", "0")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cerbo.EVInstance != -1 || cfg.Cerbo.EVChargerInstance != -1 || cfg.MQTT.Host != "" || cfg.MQTT.Port != 0 {
		t.Fatal("auto discovery changed IGW-only connection defaults")
	}
	if err := os.WriteFile("config.yaml", []byte("mqtt:\n  host: ''\n  port: 0\ncerbo:\n  ev_instance: 0\n  evcharger_instance: 71\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cerbo.EVInstance != 0 || cfg.Cerbo.EVChargerInstance != 71 || cfg.MQTT.Host != "" || cfg.MQTT.Port != 0 {
		t.Fatalf("configured instances or IGW-only mode changed: %+v", cfg.Cerbo)
	}
}
