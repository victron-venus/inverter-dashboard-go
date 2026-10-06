package mqtt

import (
	"fmt"
	"time"
)

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
	generation, session := c.pushGenerationNow(), c.mqttSession.Load()
	c.gatewayMu.RLock()
	c.stateMu.RLock()
	portal, instance := c.portalID, c.pumpInstance
	if which == "valve" {
		instance = c.valveInstance
	}
	c.stateMu.RUnlock()
	gateway, publish, connected := c.gatewayMode, c.gatewayPublish, c.gatewayConnected
	if generation != c.pushGenerationNow() || session != c.mqttSession.Load() {
		c.gatewayMu.RUnlock()
		return fmt.Errorf("water source changed")
	}
	if gateway {
		c.gatewayMu.RUnlock()
		if !connected || publish == nil {
			return fmt.Errorf("gateway not connected")
		}
		return publish("water_mode", map[string]interface{}{"instance": instance, "mode": mode})
	}
	defer c.gatewayMu.RUnlock()
	c.stateMu.RLock()
	known := c.waterControlReady(which, time.Now())
	c.stateMu.RUnlock()
	if !known {
		return fmt.Errorf("water observation is stale")
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
	known := c.state != nil && c.state.TelemetryAvailable[key] && c.waterControlReady(which, time.Now())
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

func (c *Client) waterControlReady(which string, now time.Time) bool {
	key := "pump_mode"
	if which == "valve" {
		key = "water_valve_mode"
	}
	at := c.waterModeObserved[key]
	age := now.Sub(at)
	return c.state != nil && c.state.TelemetryAvailable[key] && !at.IsZero() && age >= 0 && age <= 30*time.Second
}

// suppress-republish keepalive does not renew static Mode observations. Read
// only the selected device leaves; never broaden this into a full-tree poll.
func (c *Client) refreshWaterModes() {
	c.gatewayMu.RLock()
	defer c.gatewayMu.RUnlock()
	if c.gatewayMode || c.client == nil || !c.client.IsConnectionOpen() {
		return
	}
	c.stateMu.RLock()
	portal := c.portalID
	instances := []int{c.pumpInstance, c.valveInstance}
	c.stateMu.RUnlock()
	if !validPortal(portal) {
		return
	}
	seen := map[int]bool{}
	for _, instance := range instances {
		if instance < 0 || seen[instance] {
			continue
		}
		seen[instance] = true
		token := c.client.Publish(fmt.Sprintf("R/%s/pump/%d/Mode", portal, instance), 0, false, "")
		if !token.WaitTimeout(2 * time.Second) {
			return
		}
	}
}
