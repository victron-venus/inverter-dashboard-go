package websocket

import (
	"errors"
	"strings"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func requireController(client MQTTCommander) error {
	if capability, ok := client.(interface{ CanControlInverter() bool }); ok && !capability.CanControlInverter() {
		return errors.New("inverter-control state or transport is unavailable")
	}
	return nil
}

func handleControllerToggle(entity string, requested interface{}, client MQTTCommander) error {
	if err := requireController(client); err != nil {
		return err
	}
	entity = strings.TrimPrefix(strings.TrimSpace(entity), "input_boolean.")
	if !state.IsControlFlag(entity) {
		return errors.New("unknown inverter-control flag")
	}
	var value bool
	var ok bool
	if requested != nil {
		value, ok = state.ControlBool(requested)
	} else if current := client.GetState(); current != nil {
		value, ok = state.ControlBool(current.Booleans[entity])
		value = !value
	}
	if !ok {
		return errors.New("control flag state is unknown or invalid")
	}
	text := "off"
	if value {
		text = "on"
	}
	// Absolute setters are never interpreted as HA entity commands.
	return client.PublishCommand("toggle", map[string]interface{}{"entity": entity, "state": text})
}

func handleDryRun(requested interface{}, client MQTTCommander) error {
	if err := requireController(client); err != nil {
		return err
	}
	value, ok := requested.(bool)
	if requested == nil {
		current := client.GetState()
		if current == nil || current.DryRun == nil {
			return errors.New("dry_run state is unknown")
		}
		value, ok = !*current.DryRun, true
	}
	if !ok {
		return errors.New("dry_run value must be boolean")
	}
	return client.PublishCommand("dry_run", map[string]interface{}{"value": value})
}
