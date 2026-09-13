package mqtt

import (
	"fmt"
	"strconv"
)

// SetWaterMode writes the configured dbus-pump device's Mode. Readback from
// Cerbo is authoritative; a successful MQTT publish does not change UI state.
func (c *Client) SetWaterMode(which string, mode int) error {
	if mode < 0 || mode > 2 {
		return fmt.Errorf("water mode must be 0 (auto), 1 (on), or 2 (off)")
	}
	if which != "pump" && which != "valve" {
		return fmt.Errorf("water device must be pump or valve")
	}
	c.gatewayMu.RLock()
	gateway := c.gatewayMode || c.gatewayPublish != nil
	c.gatewayMu.RUnlock()
	if gateway {
		return fmt.Errorf("water mode is not supported by the gateway transport")
	}
	if c.client == nil || !c.client.IsConnectionOpen() {
		return fmt.Errorf("mqtt not connected")
	}
	c.stateMu.RLock()
	portal, instance := c.portalID, c.pumpInstance
	if which == "valve" {
		instance = c.valveInstance
	}
	device := devices(c.cerboLeaves, "pump")[strconv.Itoa(instance)]
	current, observed := device.num("Mode")
	c.stateMu.RUnlock()
	if !validPortal(portal) {
		return fmt.Errorf("cerbo portal id required for water mode")
	}
	if !observed || current < 0 || current > 2 || current != float64(int(current)) {
		return fmt.Errorf("configured %s Mode is unavailable on Cerbo", which)
	}
	topic := fmt.Sprintf("W/%s/pump/%d/Mode", portal, instance)
	body := fmt.Sprintf(`{"value":%d}`, mode)
	if token := c.client.Publish(topic, 0, false, body); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish water mode: %w", token.Error())
	}
	return nil
}

// CanControlWater reports transport capability. Each target additionally needs
// an observed valid /Mode, checked by SetWaterMode and the UI availability map.
func (c *Client) CanControlWater() bool {
	c.gatewayMu.RLock()
	gateway := c.gatewayMode || c.gatewayPublish != nil
	c.gatewayMu.RUnlock()
	return !gateway && c.client != nil && c.client.IsConnectionOpen() && validPortal(c.PortalID())
}
