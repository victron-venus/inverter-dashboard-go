package gateway

import "errors"

// ErrCommandNotOnGateway is returned for inverter/cmd/* actions that IGW
// does not expose yet (telemetry-first whitelist).
var ErrCommandNotOnGateway = errors.New(
	"command not available via inverter-gateway (IGW whitelist is telemetry-first: " +
		"silence_alarm, acknowledge_all_notifications; inverter/cmd/* not exposed yet)",
)

// Whitelisted IGW command names (must match inverter-gateway src/whitelist.rs).
var whitelistedCommands = map[string]struct{}{
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
	case "silence_alarm", "acknowledge_all_notifications":
		return action, true
	case "dismiss_banner", "acknowledge_victron_banner":
		// Desktop/UI aliases — IGW only exposes AcknowledgeAll.
		return "acknowledge_all_notifications", true
	default:
		return "", false
	}
}
