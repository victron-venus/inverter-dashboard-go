package mqtt

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// Called under stateMu. Receipt time is server-owned, never decoded from MQTT.
func (c *Client) observeESSMode(data map[string]interface{}, retained bool) {
	if len(data) == 0 {
		c.controllerESSMode = nil
		c.controllerESSObserved = nil
		return
	}
	raw, exists := data["ess_mode"]
	if !exists {
		return
	}
	c.controllerESSMode = nil
	c.controllerESSObserved = nil
	if _, ok := raw.(map[string]interface{}); !ok {
		return
	}
	encoded, err := json.Marshal(raw)
	var mode state.ESSMode
	if err != nil || json.Unmarshal(encoded, &mode) != nil {
		return
	}
	c.controllerESSMode = &mode
	if !retained {
		at := float64(time.Now().UnixMilli()) / 1000
		c.controllerESSObserved = &at
	}
}

func (c *Client) CanSelectESSMode() bool {
	if !c.IsConnected() {
		return false
	}
	c.gatewayMu.RLock()
	remote := c.gatewayMode
	c.gatewayMu.RUnlock()
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.expireOptionalTelemetry(time.Now())
	return state.ESSSelectionReady(c.state, time.Now()) && (!remote || c.state.GatewayCapabilities["set_ess_mode"])
}

func (c *Client) validateESSCommand(payload any) error {
	if err := state.ValidateESSSelection(payload); err != nil {
		return err
	}
	if !c.CanSelectESSMode() {
		return fmt.Errorf("wait for live supported ESS telemetry with dry run disabled")
	}
	return nil
}
