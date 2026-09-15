package mqtt

import "fmt"

// SetWaterMode sends a native dbus-pump Mode command. Only device readback
// updates state; successful delivery never predicts the physical result.
func (c *Client) SetWaterMode(which string, mode int) error {
	if mode < 0 || mode > 2 {
		return fmt.Errorf("water mode must be 0 (auto), 1 (on), or 2 (off)")
	}
	if which != "pump" && which != "valve" {
		return fmt.Errorf("water device must be pump or valve")
	}
	if !c.CanControlWaterDevice(which) {
		return fmt.Errorf("configured %s Mode or native transport is unavailable", which)
	}
	c.stateMu.RLock()
	portal, instance := c.portalID, c.pumpInstance
	if which == "valve" {
		instance = c.valveInstance
	}
	c.stateMu.RUnlock()
	c.gatewayMu.RLock()
	gateway, publish, connected := c.gatewayMode, c.gatewayPublish, c.gatewayConnected
	c.gatewayMu.RUnlock()
	if gateway {
		if !connected || publish == nil {
			return fmt.Errorf("gateway not connected")
		}
		return publish("water_mode", map[string]interface{}{"instance": instance, "mode": mode})
	}
	if c.client == nil || !c.client.IsConnectionOpen() {
		return fmt.Errorf("mqtt not connected")
	}
	topic := fmt.Sprintf("W/%s/pump/%d/Mode", portal, instance)
	body := fmt.Sprintf(`{"value":%d}`, mode)
	if token := c.client.Publish(topic, 0, false, body); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish water mode: %w", token.Error())
	}
	return nil
}

// CanControlWaterDevice requires both current native readback and a transport
// which supports water commands. Older gateways remain read-only for Water.
func (c *Client) CanControlWaterDevice(which string) bool {
	if which != "pump" && which != "valve" {
		return false
	}
	c.gatewayMu.RLock()
	gateway, connected, hasPublisher := c.gatewayMode, c.gatewayConnected, c.gatewayPublish != nil
	c.gatewayMu.RUnlock()
	c.stateMu.RLock()
	key, instance := "pump_mode", c.pumpInstance
	if which == "valve" {
		key, instance = "water_valve_mode", c.valveInstance
	}
	known := c.state != nil && c.state.TelemetryAvailable[key]
	capable := c.state != nil && c.state.GatewayCapabilities["water_mode"]
	portal := c.portalID
	c.stateMu.RUnlock()
	if !known || instance < 0 {
		return false
	}
	if gateway {
		return connected && hasPublisher && capable
	}
	return c.client != nil && c.client.IsConnectionOpen() && validPortal(portal)
}

func (c *Client) CanControlWater() bool {
	return c.CanControlWaterDevice("pump") || c.CanControlWaterDevice("valve")
}
