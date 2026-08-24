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
}

var (
	mu        sync.RWMutex
	current   map[string]interface{}
	cameraEnv string // CAMERA_TOPIC env default, set at startup
)

// Init seeds defaults (camera topic from env) and loads any persisted file.
func Init(cameraTopicFromEnv string) {
	cameraEnv = cameraTopicFromEnv
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

type errUnknown struct{ key string }

func (e errUnknown) Error() string { return "unknown setting: " + e.key }

type errType struct{ key string }

func (e errType) Error() string { return "setting " + e.key + " has wrong type" }
