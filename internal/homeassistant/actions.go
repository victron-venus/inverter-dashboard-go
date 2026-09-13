package homeassistant

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

func actionNumber(value interface{}) (float64, bool) {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}

// PerformAction operates only the rich entities explicitly configured for HA.
// It never falls back to inverter-control for rejected or failed requests.
func (c *Client) PerformAction(action, entity string, payload map[string]interface{}) error {
	if !c.IsDirectMode() || c.filteredEntities == nil {
		return fmt.Errorf("direct Home Assistant controls are not configured")
	}
	if entity == "" || IsControlFlag(entity) {
		return fmt.Errorf("home assistant entity required")
	}
	domain := strings.Split(entity, ".")[0]
	fields := map[string]interface{}{}
	allowed := false
	service := ""
	switch action {
	case "number_set":
		allowed = (domain == "number" || domain == "input_number") && slices.Contains(c.filteredEntities.Numbers, entity)
		value, ok := actionNumber(payload["value"])
		if !ok {
			return fmt.Errorf("number value must be finite")
		}
		fields["value"] = value
		service = "set_value"
	case "set_cover_position":
		allowed = domain == "cover" && slices.Contains(c.filteredEntities.Covers, entity)
		value, ok := actionNumber(payload["position"])
		if !ok || value < 0 || value > 100 || math.Trunc(value) != value {
			return fmt.Errorf("cover position must be an integer from 0 to 100")
		}
		fields["position"] = int(value)
		service = "set_cover_position"
	case "media_player":
		allowed = domain == "media_player" && slices.Contains(c.filteredEntities.MediaPlayers, entity)
		mpAction, _ := payload["mp_action"].(string)
		service = map[string]string{"play": "media_play", "pause": "media_pause", "stop": "media_stop"}[mpAction]
	case "scene_activate":
		allowed = domain == "scene" && slices.Contains(c.filteredEntities.Scenes, entity)
		service = "turn_on"
	}
	if !allowed || service == "" {
		return fmt.Errorf("home assistant action is not allowed for this entity")
	}
	return c.callServiceData(domain, service, entity, fields)
}
