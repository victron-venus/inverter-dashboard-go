package mqtt

import "time"

// TransportStatus separates broker health from the selected native transport.
// IGW observed_at is the local receipt time of a complete gateway snapshot.
func (c *Client) TransportStatus() map[string]interface{} {
	c.gatewayMu.RLock()
	gateway, gatewayConnected := c.gatewayMode, c.gatewayConnected
	c.gatewayMu.RUnlock()
	mqttConnected := c.client != nil && c.client.IsConnectionOpen() && !gateway
	source, connected := "mqtt", mqttConnected
	if gateway {
		source, connected = "igw", gatewayConnected
	}
	c.stateMu.RLock()
	observed := c.nativeLastSeen
	c.stateMu.RUnlock()
	quality, timestamp := "unknown", ""
	if !observed.IsZero() {
		timestamp = observed.UTC().Format(time.RFC3339Nano)
		quality = "stale"
		if connected && time.Since(observed) <= 120*time.Second {
			quality = "live"
		}
	}
	return map[string]interface{}{
		"data_source": source, "mqtt_connected": mqttConnected,
		"gateway_connected": gateway && gatewayConnected, "native_connected": connected,
		"telemetry": map[string]interface{}{"source": source, "observed_at": timestamp, "quality": quality, "timestamp_source": "local_receipt"},
	}
}
