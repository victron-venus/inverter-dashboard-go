package homeassistant

import (
	"strconv"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

// FilteredEntityConfig aliases the config-package type so client code and
// tests can reference it without importing config directly.
type FilteredEntityConfig = config.FilteredEntityConfig

// filteredEmpty reports whether nothing is configured.
func filteredEmpty(f *FilteredEntityConfig) bool {
	return f == nil || (len(f.Covers) == 0 && len(f.MediaPlayers) == 0 &&
		len(f.Scenes) == 0 && len(f.Numbers) == 0 && len(f.Sensors) == 0 && f.Weather == "")
}

// filteredAll lists every entity id referenced by the config (deduped).
func filteredAll(f *FilteredEntityConfig) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ids []string) {
		for _, id := range ids {
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	add(f.Covers)
	add(f.MediaPlayers)
	add(f.Scenes)
	add(f.Numbers)
	add(f.Sensors)
	if f.Weather != "" && !seen[f.Weather] {
		out = append(out, f.Weather)
	}
	return out
}

// attr helpers -----------------------------------------------------------

func friendlyName(doc EntityState) string {
	if doc.Attributes != nil {
		if name, ok := doc.Attributes["friendly_name"].(string); ok && name != "" {
			return name
		}
	}
	return doc.EntityID
}

func attrString(doc EntityState, key string) string {
	if doc.Attributes == nil {
		return ""
	}
	s, _ := doc.Attributes[key].(string)
	return s
}

func attrFloat(doc EntityState, key string) (float64, bool) {
	if doc.Attributes == nil {
		return 0, false
	}
	switch v := doc.Attributes[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// BuildFilteredDisplays maps fetched entity docs to the shared vue-lib
// HaFilteredData contract ({sensors[], numbers[], covers[],
// media_players[], scenes[], weather}) as a JSON-ready map. Missing docs
// leave their section empty; unavailable numbers are skipped.
func BuildFilteredDisplays(docs map[string]*EntityState, cfg *FilteredEntityConfig) map[string]interface{} {
	out := map[string]interface{}{
		"sensors":       []interface{}{},
		"numbers":       []interface{}{},
		"covers":        []interface{}{},
		"media_players": []interface{}{},
		"scenes":        []interface{}{},
		"weather":       nil,
	}
	if cfg == nil {
		return out
	}

	get := func(id string) *EntityState {
		if id == "" {
			return nil
		}
		return docs[id]
	}

	for _, id := range cfg.Sensors {
		doc := get(id)
		if doc == nil {
			continue
		}
		out["sensors"] = append(out["sensors"].([]interface{}), map[string]interface{}{
			"entity_id": doc.EntityID,
			"name":      friendlyName(*doc),
			"state":     doc.State,
			"unit":      attrString(*doc, "unit_of_measurement"),
		})
	}

	for _, id := range cfg.Numbers {
		doc := get(id)
		if doc == nil {
			continue
		}
		value, ok := attrFloat(*doc, "state")
		if !ok {
			// EntityState.State carries the raw value; attributes don't.
			if f, err := strconv.ParseFloat(doc.State, 64); err == nil {
				value, ok = f, true
			}
		}
		if !ok || doc.State == "unavailable" || doc.State == "unknown" {
			continue
		}
		minv, _ := attrFloat(*doc, "min")
		maxv, _ := attrFloat(*doc, "max")
		step, _ := attrFloat(*doc, "step")
		if maxv == 0 && minv == 0 {
			maxv = 100
		}
		if step == 0 {
			step = 1
		}
		out["numbers"] = append(out["numbers"].([]interface{}), map[string]interface{}{
			"entity_id": doc.EntityID,
			"name":      friendlyName(*doc),
			"value":     value,
			"min":       minv,
			"max":       maxv,
			"step":      step,
			"unit":      attrString(*doc, "unit_of_measurement"),
		})
	}

	for _, id := range cfg.Covers {
		doc := get(id)
		if doc == nil {
			continue
		}
		pos, ok := attrFloat(*doc, "current_position")
		if !ok {
			if doc.State == "open" {
				pos = 100
			} else {
				pos = 0
			}
		}
		out["covers"] = append(out["covers"].([]interface{}), map[string]interface{}{
			"entity_id": doc.EntityID,
			"name":      friendlyName(*doc),
			"position":  int(pos),
		})
	}

	for _, id := range cfg.MediaPlayers {
		doc := get(id)
		if doc == nil {
			continue
		}
		out["media_players"] = append(out["media_players"].([]interface{}), map[string]interface{}{
			"entity_id": doc.EntityID,
			"name":      friendlyName(*doc),
			"state":     doc.State,
		})
	}

	for _, id := range cfg.Scenes {
		doc := get(id)
		if doc == nil {
			continue
		}
		out["scenes"] = append(out["scenes"].([]interface{}), map[string]interface{}{
			"entity_id": doc.EntityID,
			"name":      friendlyName(*doc),
		})
	}

	if w := get(cfg.Weather); w != nil {
		forecast := interface{}([]interface{}{})
		if w.Attributes != nil {
			if fc, ok := w.Attributes["forecast"]; ok {
				forecast = fc
			}
		}
		temp, _ := attrFloat(*w, "temperature")
		out["weather"] = map[string]interface{}{
			"entity_id":   w.EntityID,
			"name":        friendlyName(*w),
			"state":       w.State,
			"temperature": temp,
			"unit":        attrString(*w, "temperature_unit"),
			"forecast":    forecast,
		}
	}

	return out
}
