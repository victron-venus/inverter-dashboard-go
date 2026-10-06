package mqtt

import (
	"context"
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

type heldPublishToken struct{ done chan struct{} }

func (t heldPublishToken) Wait() bool { <-t.done; return true }
func (t heldPublishToken) WaitTimeout(d time.Duration) bool {
	select {
	case <-t.done:
		return true
	case <-time.After(d):
		return false
	}
}
func (t heldPublishToken) Done() <-chan struct{} { return t.done }
func (t heldPublishToken) Error() error          { return nil }

type heldPublishBroker struct {
	essRecordingBroker
	token     heldPublishToken
	published chan struct{}
}

func (b *heldPublishBroker) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	b.essRecordingBroker.Publish(topic, qos, retained, payload)
	close(b.published)
	return b.token
}

func TestHeldNativePublishDoesNotBlockFailoverOrReportRetiredAcceptance(t *testing.T) {
	for _, action := range []string{"dry_run", "water_mode", "electricity_tariff", "silence_alarm", "acknowledge_all_notifications"} {
		t.Run(action, func(t *testing.T) {
			c := NewClient("localhost", 1883)
			b := &heldPublishBroker{token: heldPublishToken{make(chan struct{})}, published: make(chan struct{})}
			c.client = b
			c.SetWaterConfig("p1", 21, 3, 8)
			controllerMessage(c, `{"dry_run":false,"ui_config":{"electricity_tariff_status":{"writable":true,"revision":"`+strings.Repeat("a", 64)+`"}}}`)
			send(c, "pump/3", "Mode", 0)
			result := make(chan error, 1)
			go func() {
				switch action {
				case "water_mode":
					result <- c.SetWaterMode("pump", 1)
				case "electricity_tariff":
					result <- c.PublishCommand(action, map[string]interface{}{"plan": nil, "revision": strings.Repeat("a", 64), "request_id": "id"})
				default:
					result <- c.PublishCommand(action, map[string]interface{}{"value": true})
				}
			}()
			<-b.published
			switched := make(chan struct{})
			go func() { c.EnableGatewayMode(); close(switched) }()
			select {
			case <-switched:
			case <-time.After(time.Second):
				close(b.token.done)
				t.Fatal("pending publish blocked failover")
			}
			close(b.token.done)
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("retired source reported accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("retired publish did not finish")
			}
		})
	}
}

func TestPublishWaitHonorsDeadlineAndAtomicOverrideSnapshot(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.client = &essRecordingBroker{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.awaitPublish(ctx, heldPublishToken{make(chan struct{})}, c.pushGenerationNow(), c.mqttSession.Load()); err == nil {
		t.Fatal("held token ignored deadline")
	}
	controllerMessage(c, `{"setpoint_override":{"value":42,"request_id":"same","last_error":null}}`)
	serial := c.overrideReceipt
	c.onStateMessage(nil, &retainedESSMessage{&fakeMessage{payload: []byte(`{"setpoint_override":{"value":42,"request_id":"same","last_error":null}}`)}})
	if ok, err := c.overrideAcknowledgement(context.Background(), c.pushGenerationNow(), c.mqttSession.Load(), serial, map[string]interface{}{"value": 42, "request_id": "same"}); ok || err == nil {
		t.Fatal("later retained serial borrowed a live matching status")
	}
	controllerMessage(c, `{"setpoint_override":{"value":43,"request_id":"other","last_error":null}}`)
	if ok, _ := c.overrideAcknowledgement(context.Background(), c.pushGenerationNow(), c.mqttSession.Load(), serial, map[string]interface{}{"value": 42, "request_id": "same"}); ok {
		t.Fatal("later different receipt acknowledged old request")
	}
	if ok, err := c.overrideAcknowledgement(ctx, c.pushGenerationNow(), c.mqttSession.Load(), serial, map[string]interface{}{"value": 43, "request_id": "other"}); ok || err == nil {
		t.Fatal("expired deadline accepted acknowledgement")
	}
}
