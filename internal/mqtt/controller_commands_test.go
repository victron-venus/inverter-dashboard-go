package mqtt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

const overrideStatus = `{"setpoint_override":{"value":null,"request_id":null,"last_error":null},"dry_run":true}`

type overrideBroker struct {
	essRecordingBroker
	onWrite func()
}

func (b *overrideBroker) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	token := b.essRecordingBroker.Publish(topic, qos, retained, payload)
	if b.onWrite != nil {
		b.onWrite()
	}
	return token
}

func TestOverrideFreshnessRetainedAndExactReadback(t *testing.T) {
	for _, reason := range []string{"live", "stop", "retained", "stale", "future", "unsupported", "sourcechange"} {
		t.Run(reason, func(t *testing.T) {
			c := NewClient("localhost", 1883)
			b := &overrideBroker{}
			c.client = b
			controllerMessage(c, overrideStatus)
			var value any = 42
			if reason == "stop" {
				value = nil
			}
			body := map[string]interface{}{"value": value, "request_id": "selection"}
			switch reason {
			case "retained":
				c.onStateMessage(nil, &retainedESSMessage{&fakeMessage{payload: []byte(overrideStatus)}})
			case "stale", "future":
				offset := -31 * time.Second
				if reason == "future" {
					offset = time.Minute
				}
				at := float64(time.Now().Add(offset).UnixMilli()) / 1000
				c.state.SetpointOverrideObservedAt = &at
			case "unsupported":
				controllerMessage(c, `{"setpoint_override":{"value":null}}`)
			}
			b.onWrite = func() {
				if reason == "sourcechange" {
					c.mqttSession.Add(1)
					return
				}
				encoded, _ := json.Marshal(map[string]interface{}{"setpoint_override": map[string]interface{}{"value": value, "request_id": "selection", "last_error": nil}, "dry_run": true})
				// Publish is under the transport read lock; the source callback is independent.
				go controllerMessage(c, string(encoded))
			}
			err := c.PublishCommand("setpoint_override", body)
			want := reason == "live" || reason == "stop"
			if (err == nil) != want {
				t.Fatalf("error=%v want accepted=%v", err, want)
			}
			if !want && reason != "sourcechange" && len(b.writes) != 0 {
				t.Fatal("rejected command dispatched")
			}
			if want && (len(b.writes) != 1 || !strings.HasPrefix(b.writes[0], "inverter/cmd/setpoint_override=")) {
				t.Fatal("not exactly one native override")
			}
		})
	}
}

func TestControllerCallbacksCannotReviveRetiredSource(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.client = &recordingBroker{}
	bound := c.bindControllerCallback(c.mqttSession.Load(), false)
	c.mqttSession.Add(1)
	bound(nil, &fakeMessage{payload: []byte(overrideStatus)})
	if c.GetState().SetpointOverride != nil {
		t.Fatal("old MQTT session revived control")
	}
	c.EnableGatewayMode()
	c.onStateMessage(nil, &fakeMessage{payload: []byte(overrideStatus)})
	if c.GetState().SetpointOverride != nil {
		t.Fatal("inactive MQTT replaced gateway control")
	}
}

func TestControllerHeaderFreshnessIsIndependentOfDisplayAndRetainedState(t *testing.T) {
	for _, reason := range []string{"live", "retained", "stale", "future", "unrelated"} {
		t.Run(reason, func(t *testing.T) {
			c := NewClient("localhost", 1883)
			b := &essRecordingBroker{}
			c.client = b
			controllerMessage(c, `{"booleans":{"no_feed":false},"dry_run":false}`)
			if reason == "retained" {
				c.onStateMessage(nil, &retainedESSMessage{&fakeMessage{payload: []byte(`{"booleans":{"no_feed":false},"dry_run":false}`)}})
			}
			if reason == "stale" || reason == "unrelated" {
				c.controllerLiveAt = time.Now().Add(-31 * time.Second)
			}
			if reason == "future" {
				c.controllerLiveAt = time.Now().Add(time.Minute)
			}
			if reason == "unrelated" {
				send(c, "system/0", "Ac/Grid/L1/Power", 123)
			}
			if !*c.GetState().InverterAvailable {
				t.Fatal("display incorrectly cleared at command timeout")
			}
			err := c.PublishCommand("dry_run", map[string]interface{}{"value": true})
			if (err == nil) != (reason == "live") || c.CanControlInverter() != (reason == "live") {
				t.Fatalf("capability/publish mismatch: %v", err)
			}
		})
	}
}

func TestMatchingOverrideBeforePublishDoesNotAcknowledgeNewAttempt(t *testing.T) {
	c := NewClient("localhost", 1883)
	b := &overrideBroker{}
	c.client = b
	controllerMessage(c, `{"setpoint_override":{"value":42,"request_id":"reuse","last_error":null}}`)
	done := make(chan struct{})
	b.onWrite = func() { go func() { defer close(done); time.Sleep(40 * time.Millisecond); c.mqttSession.Add(1) }() }
	err := c.PublishCommand("setpoint_override", map[string]interface{}{"value": 42, "request_id": "reuse"})
	<-done
	if err == nil {
		t.Fatal("old matching status acknowledged without a new observation")
	}
}
