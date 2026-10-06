// Package settings persists runtime-editable dashboard settings
// (dashboard_settings.json, mirroring the Python dashboard's store).
// Secrets stay in env/config.yaml by design.
package settings

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
)

const settingsFile = "dashboard_settings.json"

func storagePath() string {
	if explicit := os.Getenv("INVERTER_DASHBOARD_SETTINGS_FILE"); explicit != "" {
		return explicit
	}
	return settingsFile
}

// ALLOWED_KEYS with types; anything else is rejected.
var allowed = map[string]string{
	"camera_topic":          "string",
	"show_ev":               "bool",
	"show_washer":           "bool",
	"show_dryer":            "bool",
	"show_dishwasher":       "bool",
	"show_home_section":     "bool",
	"show_ha_covers":        "bool",
	"show_ha_media":         "bool",
	"show_ha_scenes":        "bool",
	"show_ha_weather":       "bool",
	"show_daily_stats":      "bool",
	"show_header_toggles":   "bool",
	"show_batteries":        "bool",
	"show_solar_production": "bool",
	"show_active_loads":     "bool",
	"show_ha_sensors":       "bool",
	"show_ha_numbers":       "bool",
	// Connection overrides (applied at startup by main.go; restart required).
	// Note: mqtt_username/mqtt_password intentionally absent — the go client
	// connects anonymously (Cerbo broker); add when broker auth lands.
	"mqtt_host": "string",
	"mqtt_port": "int",
	"ha_url":    "string",
	"ha_token":  "string",
}

// SecretKeys are masked in API responses.
var SecretKeys = []string{"ha_token"}

var (
	mu          sync.RWMutex
	current     map[string]interface{}
	cameraEnv   string                 // CAMERA_TOPIC env default, set at startup
	envDefaults map[string]interface{} // pre-file MQTT defaults for override comparison
)

// Init seeds defaults (camera topic from env) and loads any persisted file.
func Init(cameraTopicFromEnv string, mqttHost string, mqttPort int) {
	cameraEnv = cameraTopicFromEnv
	envDefaults = map[string]interface{}{
		"MQTT_HOST": mqttHost,
		"MQTT_PORT": mqttPort,
	}
	mu.Lock()
	defer mu.Unlock()
	current = defaults()
	data, err := os.ReadFile(storagePath())
	if err != nil {
		return
	}
	var stored map[string]interface{}
	if json.Unmarshal(data, &stored) != nil {
		return
	}
	for k := range allowed {
		if v, ok := stored[k]; ok && typeOK(k, v) {
			current[k] = v
		}
	}
}

func defaults() map[string]interface{} {
	return map[string]interface{}{
		"camera_topic":          cameraEnv,
		"mqtt_host":             envDefaults["MQTT_HOST"],
		"mqtt_port":             envDefaults["MQTT_PORT"],
		"ha_url":                "",
		"ha_token":              "",
		"show_ev":               true,
		"show_washer":           true,
		"show_dryer":            true,
		"show_dishwasher":       true,
		"show_home_section":     true,
		"show_ha_covers":        true,
		"show_ha_media":         true,
		"show_ha_scenes":        true,
		"show_ha_weather":       true,
		"show_daily_stats":      true,
		"show_header_toggles":   true,
		"show_batteries":        true,
		"show_solar_production": true,
		"show_active_loads":     true,
		"show_ha_sensors":       true,
		"show_ha_numbers":       true,
	}
}

func typeOK(key string, v interface{}) bool {
	switch allowed[key] {
	case "string":
		_, ok := v.(string) // empty allowed: clears the setting (e.g. disable camera)
		return ok
	case "int":
		switch n := v.(type) {
		case int:
			return n >= 1 && n <= 65535
		case float64:
			return !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n >= 1 && n <= 65535
		}
		return false
	case "bool":
		_, ok := v.(bool)
		return ok
	}
	return false
}

// Get returns a copy of the current settings.
func Get() map[string]interface{} {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]interface{}, len(current))
	for k, v := range current {
		out[k] = v
	}
	return out
}

// Apply validates the patch against the allowlist, merge-writes the file,
// and hot-applies. Unknown keys or wrong types return an error naming the key.
func Apply(patch map[string]interface{}) error {
	return applyWithReplacer(patch, replaceDurably)
}

func applyWithReplacer(patch map[string]interface{}, replace func(string, string) (bool, error)) error {
	mu.Lock()
	defer mu.Unlock()
	clean := make(map[string]interface{}, len(patch))
	for k, v := range patch {
		if _, ok := allowed[k]; !ok {
			return errUnknown{k}
		}
		if !typeOK(k, v) {
			return errType{k}
		}
		clean[k] = v
	}
	next := make(map[string]interface{}, len(current)+len(clean))
	for k, v := range current {
		next[k] = v
	}
	for k, v := range clean {
		// The public settings DTO uses a placeholder, never an actual replacement credential.
		if k == "ha_token" && v == "***" {
			continue
		}
		next[k] = v
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dashboard settings: %w", err)
	}
	target := storagePath()
	parent := filepath.Dir(target)
	if os.Getenv("INVERTER_DASHBOARD_SETTINGS_FILE") != "" {
		if err := os.MkdirAll(parent, 0700); err != nil {
			return fmt.Errorf("create settings directory: %w", err)
		}
	}
	file, err := os.CreateTemp(parent, ".dashboard-settings-*")
	if err != nil {
		return fmt.Errorf("create dashboard settings: %w", err)
	}
	tmp := file.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write dashboard settings: %w", err)
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync dashboard settings: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close dashboard settings: %w", err)
	}
	replaced, err := replace(tmp, target)
	if replaced {
		// A directory flush can fail after replacement. Keep subsequent reads and
		// merge patches consistent with the visible file, while reporting that
		// its durability was not confirmed.
		current = next
	}
	if err != nil {
		return fmt.Errorf("replace dashboard settings: %w", err)
	}
	return nil
}

// Overrides returns stored connection values that differ from the
// env-derived defaults (empty values ignored). main.go applies them to the
// MQTT/HA clients at startup — restart required.
func Overrides() map[string]interface{} {
	mu.RLock()
	defer mu.RUnlock()
	out := map[string]interface{}{}
	for _, k := range []string{"mqtt_host", "mqtt_port", "ha_url", "ha_token"} {
		v, ok := current[k]
		if !ok || v == "" || v == nil {
			continue
		}
		if def, ok2 := envDefaults[defaultKey(k)]; ok2 && v == def {
			continue
		}
		out[k] = v
	}
	return out
}

// Masked returns a copy of current settings with secrets masked ("***").
func Masked() map[string]interface{} {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]interface{}, len(current))
	for k, v := range current {
		out[k] = v
	}
	for _, k := range SecretKeys {
		if s, _ := out[k].(string); s != "" {
			out[k] = "***"
		} else {
			out[k] = ""
		}
	}
	return out
}

func defaultKey(setting string) string {
	switch setting {
	case "mqtt_host":
		return "MQTT_HOST"
	case "mqtt_port":
		return "MQTT_PORT"
	}
	return "\x00" // never matches → no default comparison
}

type errUnknown struct{ key string }

func (e errUnknown) Error() string { return "unknown setting: " + e.key }

type errType struct{ key string }

func (e errType) Error() string { return "setting " + e.key + " has wrong type" }
