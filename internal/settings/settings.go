// Package settings persists runtime-editable dashboard settings
// (dashboard_settings.json, mirroring the Python dashboard's store).
// Secrets stay in env/config.yaml by design.
package settings

import (
	"encoding/json"
	"os"
	"sync"
)

const settingsFile = "dashboard_settings.json"

// ALLOWED_KEYS with types; anything else is rejected.
var allowed = map[string]string{
	"camera_topic":      "string",
	"show_ev":           "bool",
	"show_washer":       "bool",
	"show_dryer":        "bool",
	"show_dishwasher":   "bool",
	"show_home_section": "bool",
	"show_ha_covers":    "bool",
	"show_ha_media":     "bool",
	"show_ha_scenes":    "bool",
	"show_ha_weather":   "bool",
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
	data, err := os.ReadFile(settingsFile)
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
		"camera_topic":      cameraEnv,
		"mqtt_host":         envDefaults["MQTT_HOST"],
		"mqtt_port":         envDefaults["MQTT_PORT"],
		"ha_url":            "",
		"ha_token":          "",
		"show_ev":           true,
		"show_washer":       true,
		"show_dryer":        true,
		"show_dishwasher":   true,
		"show_home_section": true,
		"show_ha_covers":    true,
		"show_ha_media":     true,
		"show_ha_scenes":    true,
		"show_ha_weather":   true,
	}
}

func typeOK(key string, v interface{}) bool {
	switch allowed[key] {
	case "string":
		_, ok := v.(string) // empty allowed: clears the setting (e.g. disable camera)
		return ok
	case "int":
		switch v.(type) {
		case int, float64: // JSON numbers decode as float64
			return true
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
	for k, v := range clean {
		current[k] = v
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err == nil {
		tmp := settingsFile + ".tmp"
		if werr := os.WriteFile(tmp, data, 0o644); werr == nil {
			_ = os.Rename(tmp, settingsFile)
		}
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
