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
	Init("frigate/+/events")

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
	Init("")
	if Get()["show_ev"] != false || Get()["camera_topic"] != "frigate/+/events" {
		t.Fatalf("persistence roundtrip failed: %v", Get())
	}
}

func TestApplyRejectsUnknownAndWrongType(t *testing.T) {
	withTempDir(t)
	Init("")

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
	Init("cam")

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
	Init("")
	if Get()["show_ev"] != true {
		t.Fatal("corrupt file must fall back to defaults")
	}
	_ = filepath.Join
}
