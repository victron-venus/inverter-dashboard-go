package mqtt

import (
	"context"
	"errors"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// These receipts are owned by this process and only genuine controller frames
// renew them. Retained frames remain visible but cannot authorize commands.
func (c *Client) observeControllerCommands(data map[string]interface{}, retained bool) {
	var at *float64
	if !retained {
		n := float64(time.Now().UnixMilli()) / 1000
		at = &n
	}
	if raw, exists := data["setpoint_override"]; exists {
		c.overrideReceipt++
		c.state.SetpointOverride = state.ParseOverrideStatus(raw)
		c.state.SetpointOverrideObservedAt = nil
		if c.state.SetpointOverride != nil {
			c.state.SetpointOverrideObservedAt = at
		}
	}
	if _, exists := data["ui_config"]; exists {
		c.state.ElectricityTariffObservedAt = at
	}
}

func (c *Client) CanControllerCommand(name string, payload any) bool {
	if !c.IsConnected() {
		return false
	}
	c.gatewayMu.RLock()
	remote := c.gatewayMode
	c.gatewayMu.RUnlock()
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.expireOptionalTelemetry(time.Now())
	return state.ControllerCommandReady(c.state, name, payload, time.Now()) && (!remote || c.state.GatewayCapabilities[name])
}

func (c *Client) bindControllerCallback(session uint64, override bool) paho.MessageHandler {
	return func(_ paho.Client, msg paho.Message) {
		generation := c.pushGenerationNow()
		if c.mqttSession.Load() != session {
			return
		}
		c.applyControllerMessage(msg, session, generation, override)
	}
}

func (c *Client) applyControllerMessage(msg paho.Message, session, generation uint64, override bool) {
	raw := msg.Payload()
	if override && len(raw) == 0 {
		raw = []byte("null")
	}
	if override {
		raw = append(append([]byte(`{"setpoint_override":`), raw...), '}')
	}
	data, err := state.DecodeControllerState(raw)
	if err != nil {
		return
	}
	c.gatewayMu.RLock()
	if c.gatewayMode || c.mqttSession.Load() != session || c.pushGenerationNow() != generation {
		c.gatewayMu.RUnlock()
		return
	}
	c.stateMu.Lock()
	// Source changes cannot pass the gateway read lock while this receipt applies.
	if override {
		c.observeControllerCommands(data, msg.Retained())
	} else {
		c.controllerLastSeen = time.Now()
		c.controllerLiveAt = time.Time{}
		if !msg.Retained() {
			c.controllerLiveAt = time.Now()
		}
		c.observeESSMode(data, msg.Retained())
		c.mergeDaemonState(data)
		c.observeControllerCommands(data, msg.Retained())
		c.state.ESSModeObservedAt = c.controllerESSObserved
		if c.state.GridBackup != nil {
			c.state.GridBackupObservedAt = c.state.GridBackup.MeasurementTime
		}
	}
	c.stateMu.Unlock()
	c.gatewayMu.RUnlock()
	c.lastStateMu.Lock()
	c.lastStateTime = time.Now()
	c.lastStateMu.Unlock()
	c.triggerHandler()
}

// One bounded attempt: never buffer persistent changes for replay after a
// reconnect, and only settle override after an exact fresh controller readback.
func (c *Client) publishControllerCommand(name string, payload any) error {
	var err error
	if name == "setpoint_override" {
		err = state.ValidateOverride(payload)
	} else {
		err = state.ValidateTariff(payload)
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	generation, session := c.pushGenerationNow(), c.mqttSession.Load()
	if !c.CanControllerCommand(name, payload) {
		return errors.New("wait for current supported controller telemetry")
	}
	c.gatewayMu.RLock()
	if c.pushGenerationNow() != generation || c.mqttSession.Load() != session {
		c.gatewayMu.RUnlock()
		return errors.New("controller source changed")
	}
	if fn := c.gatewayPublish; fn != nil {
		c.gatewayMu.RUnlock()
		return fn(name, payload)
	}
	encoded, err := state.EncodeControllerCommand(payload)
	if err != nil {
		c.gatewayMu.RUnlock()
		return err
	}
	// Recheck under the transport lock immediately before physical dispatch.
	c.stateMu.RLock()
	receipt := c.overrideReceipt
	ready := state.ControllerCommandReady(c.state, name, payload, time.Now())
	if !ready || c.gatewayMode || c.client == nil || !c.client.IsConnectionOpen() {
		c.stateMu.RUnlock()
		c.gatewayMu.RUnlock()
		return errors.New("controller unavailable")
	}
	token := c.client.Publish("inverter/cmd/"+name, 0, false, encoded)
	c.stateMu.RUnlock()
	c.gatewayMu.RUnlock()
	if err := c.awaitPublish(ctx, token, generation, session); err != nil {
		return err
	}
	if name != "setpoint_override" {
		return nil
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		confirmed, ackErr := c.overrideAcknowledgement(ctx, generation, session, receipt, payload)
		if ackErr != nil {
			return ackErr
		}
		if confirmed {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Pair receipt serial, status, source and freshness atomically. A later retained
// frame cannot lend its serial to an earlier matching status snapshot.
func (c *Client) overrideAcknowledgement(ctx context.Context, generation, session, receipt uint64, payload any) (bool, error) {
	c.gatewayMu.RLock()
	defer c.gatewayMu.RUnlock()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if c.gatewayMode || generation != c.pushGenerationNow() || session != c.mqttSession.Load() || c.client == nil || !c.client.IsConnectionOpen() || !state.ControllerCommandReady(c.state, "setpoint_override", payload, time.Now()) {
		return false, errors.New("controller source changed")
	}
	if c.overrideReceipt <= receipt {
		return false, nil
	}
	confirmed, err := state.OverrideAcknowledged(c.state, payload)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return confirmed, err
}

func (c *Client) awaitPublish(ctx context.Context, token paho.Token, generation, session uint64) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if generation != c.pushGenerationNow() || session != c.mqttSession.Load() {
			return errors.New("command source changed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		case <-token.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if generation != c.pushGenerationNow() || session != c.mqttSession.Load() {
				return errors.New("command source changed")
			}
			if token.Error() != nil {
				return errors.New("MQTT command publish failed")
			}
			return nil
		}
	}
}
