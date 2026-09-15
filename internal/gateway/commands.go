package gateway

import (
	"errors"
	"fmt"
	"strings"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// ErrCommandNotOnGateway is returned for inverter/cmd/* actions that IGW
// does not expose through its command whitelist.
var ErrCommandNotOnGateway = errors.New("command not available via inverter-gateway")

// Whitelisted IGW command names (must match inverter-gateway src/whitelist.rs).
var whitelistedCommands = map[string]struct{}{
	"toggle":                        {},
	"dry_run":                       {},
	"ess_mode":                      {},
	"silence_alarm":                 {},
	"acknowledge_all_notifications": {},
}

// IsWhitelistedCommand reports whether name can be POSTed to /v1/commands/{name}.
func IsWhitelistedCommand(name string) bool {
	_, ok := whitelistedCommands[name]
	return ok
}

// MapDashboardAction translates a dashboard/WS action into an IGW command name.
// Returns ok=false when the action has no IGW equivalent (most inverter/cmd/*).
func MapDashboardAction(action string) (string, bool) {
	switch action {
	case "silence_alarm", "acknowledge_all_notifications", "toggle", "dry_run", "ess_mode":
		return action, true
	case "dismiss_banner", "acknowledge_victron_banner":
		// Desktop/UI aliases — IGW only exposes AcknowledgeAll.
		return "acknowledge_all_notifications", true
	default:
		return "", false
	}
}

func normalizeControllerCommand(name string, body any) (any, error) {
	if name != "toggle" && name != "dry_run" && name != "ess_mode" {
		return body, nil
	}
	object, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("controller command requires an object")
	}
	switch name {
	case "toggle":
		entity, ok := object["entity"].(string)
		if !ok || !state.IsControlFlag(entity) || len(object) != 2 {
			break
		}
		enabled, ok := state.ControlBool(object["state"])
		if !ok {
			break
		}
		value := "off"
		if enabled {
			value = "on"
		}
		return map[string]interface{}{"entity": strings.TrimPrefix(strings.TrimSpace(entity), "input_boolean."), "state": value}, nil
	case "dry_run":
		if _, ok := object["value"].(bool); ok && len(object) == 1 {
			return object, nil
		}
	case "ess_mode":
		if len(object) == 0 {
			return object, nil
		}
	}
	return nil, fmt.Errorf("invalid inverter-control command payload")
}
