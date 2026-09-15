package websocket

import (
	"fmt"
	"strings"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func requireController(client MQTTCommander) error {
	if capability, ok := client.(interface{ CanControlInverter() bool }); ok && !capability.CanControlInverter() {
		return fmt.Errorf("inverter-control state or transport is unavailable")
	}
	return nil
}

func handleControllerToggle(entity string, requested interface{}, client MQTTCommander) error {
	if err := requireController(client); err != nil {
		return err
	}
	entity = strings.TrimPrefix(strings.TrimSpace(entity), "input_boolean.")
	if !state.IsControlFlag(entity) {
		return fmt.Errorf("unknown inverter-control flag")
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
		return fmt.Errorf("control flag state is unknown or invalid")
	}
	text := "off"
	if value {
		text = "on"
	}
	// Absolute setters are never interpreted as HA entity commands.
	return client.PublishCommand("toggle", map[string]interface{}{"entity": entity, "state": text})
}
