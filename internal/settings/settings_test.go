package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestDefaultsAndRoundtrip(t *testing.T) {
	withTempDir(t)
	Init("frigate/+/events", "Cerbo", 1883)

	got := Get()
	if got["camera_topic"] != "frigate/+/events" || got["show_ev"] != true {
		t.Fatalf("unexpected defaults: %v", got)
	}

	if err := Apply(map[string]interface{}{"show_ev": false}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if Get()["show_ev"] != false {
		t.Fatal("hot-apply failed")
	}

	// Reload from disk in a fresh Init — persisted values win over defaults
	Init("", "Cerbo", 1883)
	if Get()["show_ev"] != false || Get()["camera_topic"] != "frigate/+/events" {
		t.Fatalf("persistence roundtrip failed: %v", Get())
	}
}

func TestApplyRejectsUnknownAndWrongType(t *testing.T) {
	withTempDir(t)
	Init("", "Cerbo", 1883)

	if err := Apply(map[string]interface{}{"mqtt_password": "x"}); err == nil {
		t.Fatal("unknown key must be rejected")
	}
	if err := Apply(map[string]interface{}{"show_ev": "yes"}); err == nil {
		t.Fatal("wrong type must be rejected")
	}
	if Get()["show_ev"] != true {
		t.Fatal("rejected patches must not apply")
	}
}

func TestEmptyStringClearsSetting(t *testing.T) {
	withTempDir(t)
	Init("cam", "Cerbo", 1883)

	if err := Apply(map[string]interface{}{"camera_topic": ""}); err != nil {
		t.Fatalf("empty string must be allowed: %v", err)
	}
	if Get()["camera_topic"] != "" {
		t.Fatal("empty string must clear the topic")
	}
}

func TestCorruptFileIgnored(t *testing.T) {
	withTempDir(t)
	if err := os.WriteFile(settingsFile, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	Init("", "Cerbo", 1883)
	if Get()["show_ev"] != true {
		t.Fatal("corrupt file must fall back to defaults")
	}
	_ = filepath.Join
}

func TestConnectionOverridesAndMasking(t *testing.T) {
	withTempDir(t)
	Init("", "Cerbo", 1883)

	if err := Apply(map[string]interface{}{"ha_token": "tok", "mqtt_host": "broker.local", "mqtt_port": float64(8883)}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	ov := Overrides()
	if ov["ha_token"] != "tok" || ov["mqtt_host"] != "broker.local" || ov["mqtt_port"].(float64) != 8883 {
		t.Fatalf("unexpected overrides: %v", ov)
	}
	// env-default values are not overrides
	if err := Apply(map[string]interface{}{"mqtt_host": "Cerbo"}); err != nil {
		t.Fatalf("apply default: %v", err)
	}
	if _, ok := Overrides()["mqtt_host"]; ok {
		t.Fatal("env-equal value must not be an override")
	}

	masked := Masked()
	if masked["ha_token"] != "***" {
		t.Fatalf("token not masked: %v", masked["ha_token"])
	}
	if Get()["ha_token"] != "tok" {
		t.Fatal("Get must stay unmasked")
	}
}

func TestPortFloatAndIntAccepted(t *testing.T) {
	withTempDir(t)
	Init("", "Cerbo", 1883)
	if err := Apply(map[string]interface{}{"mqtt_port": float64(1884)}); err != nil {
		t.Fatalf("float64 port rejected: %v", err)
	}
	if Get()["mqtt_port"].(float64) != 1884 {
		t.Fatal("port not applied")
	}
	if err := Apply(map[string]interface{}{"mqtt_port": "x"}); err == nil {
		t.Fatal("string port must be rejected")
	}
}
