package mqtt

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type retiringSubscriptionBroker struct {
	*recordingBroker
	afterSubscribe func(string)
}

func (b *retiringSubscriptionBroker) Subscribe(topic string, qos byte, handler paho.MessageHandler) paho.Token {
	token := b.recordingBroker.Subscribe(topic, qos, handler)
	if b.afterSubscribe != nil {
		b.afterSubscribe(topic)
	}
	return token
}

func TestNativeSubscriptionsKeepOriginalSessionAfterConnectionLoss(t *testing.T) {
	for _, tc := range []struct {
		name     string
		retireAt string
		refresh  bool
	}{
		{"before-native-subscriptions", "inverter/notifications", false},
		{"during-native-subscriptions", "N/p1/system/+/#", false},
		{"during-portal-refresh", "N/p1/system/+/#", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkNativeSubscriptionSession(t, tc.retireAt, tc.refresh)
		})
	}
}

func checkNativeSubscriptionSession(t *testing.T, retireAt string, refresh bool) {
	t.Helper()
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	retired := false
	broker := &retiringSubscriptionBroker{recordingBroker: &recordingBroker{}}
	broker.afterSubscribe = func(topic string) {
		if topic == retireAt && !retired {
			c.advanceMQTTSession()
			retired = true
		}
	}
	c.client = broker
	if refresh {
		c.refreshPortal("p1", c.mqttSession.Load())
	} else if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	defer c.stopKeepalive()
	if !retired {
		t.Fatal("test did not retire the connection during subscription")
	}
	message := &fakeMessage{
		topic:   "N/p1/platform/0/Notifications/1/Description",
		payload: []byte(`{"value":"Native alarm"}`),
	}
	broker.handlers["N/p1/platform/+/#"](nil, message)
	if len(c.GetState().Notifications) != 0 {
		t.Fatal("old connection native handler captured a newer session")
	}
	broker.afterSubscribe = nil
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	broker.handlers["N/p1/platform/+/#"](nil, message)
	if len(c.GetState().Notifications) != 1 {
		t.Fatal("fresh subscription lost its native notification")
	}
}

func TestPortalDiscoveryRejectsRetiredSubscription(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, duringDecode := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/decode=%v", native, duringDecode), func(t *testing.T) {
				checkRetiredPortalSubscription(t, native, duringDecode)
			})
		}
	}
}

func checkRetiredPortalSubscription(t *testing.T, native, duringDecode bool) {
	t.Helper()
	c := NewClient("localhost", 1883)
	broker := &recordingBroker{}
	c.client = broker
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	defer c.stopKeepalive()
	topic, payload := "inverter/portal", []byte(`"p1"`)
	if native {
		topic, payload = "N/+/heartbeat", []byte(`{"value":123}`)
	}
	callback := broker.handlers[topic]
	msg := &pausedNotification{
		fakeMessage: &fakeMessage{topic: strings.ReplaceAll(topic, "+", "p1"), payload: payload},
		entered:     make(chan struct{}), resume: make(chan struct{}),
	}
	done := make(chan struct{})
	if duringDecode {
		go func() { defer close(done); callback(nil, msg) }()
		select {
		case <-msg.entered:
		case <-time.After(time.Second):
			t.Fatal("portal callback did not reach decode")
		}
	}
	c.advanceMQTTSession()
	close(msg.resume)
	if !duringDecode {
		go func() { defer close(done); callback(nil, msg) }()
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("portal callback did not finish")
	}
	if c.PortalID() != "" {
		t.Fatal("retired callback changed the portal identity")
	}
}

type pausedNotification struct {
	*fakeMessage
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (m *pausedNotification) Payload() []byte {
	m.once.Do(func() {
		close(m.entered)
		<-m.resume
	})
	return m.fakeMessage.Payload()
}

func TestPlatformNotificationKeepsSubscriptionSessionDuringDecode(t *testing.T) {
	c := newTestClient()
	c.SetWaterConfig("p1", 21, 1, 2)
	msg := &pausedNotification{
		fakeMessage: &fakeMessage{
			topic:   "N/p1/platform/0/Notifications/1/Description",
			payload: []byte(`{"value":"Retired native alarm"}`),
		},
		entered: make(chan struct{}), resume: make(chan struct{}),
	}
	callback := c.bindPushCallback(c.mqttSession.Load(), true)
	done := make(chan struct{})
	go func() { defer close(done); callback(nil, msg) }()
	select {
	case <-msg.entered:
	case <-time.After(time.Second):
		t.Fatal("native notification did not reach decode")
	}
	// Connection loss retires the MQTT session before the next subscription
	// resets push generation. The old callback must retain its original token.
	c.advanceMQTTSession()
	close(msg.resume)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("native notification callback did not finish")
	}
	if got := c.GetState().Notifications; len(got) != 0 {
		t.Fatalf("native callback recaptured the new session: %+v", got)
	}
}

func TestControllerNotificationRejectsRetiredSourceDuringDecode(t *testing.T) {
	for _, transition := range []string{"gateway", "mqtt-session", "push-generation", "unchanged"} {
		t.Run(transition, func(t *testing.T) {
			checkControllerNotificationRetirement(t, transition)
		})
	}
}

func checkControllerNotificationRetirement(t *testing.T, transition string) {
	t.Helper()
	c := newTestClient()
	callback := c.bindPushCallback(c.mqttSession.Load(), false)
	msg := &pausedNotification{
		fakeMessage: &fakeMessage{
			topic:   "inverter/notifications",
			payload: []byte(`{"id":"pending","title":"Pending alarm","ts":"2026-10-20T09:00:00Z"}`),
		},
		entered: make(chan struct{}),
		resume:  make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		callback(nil, msg)
	}()
	select {
	case <-msg.entered:
	case <-time.After(time.Second):
		t.Fatal("notification did not reach decode")
	}
	switch transition {
	case "gateway":
		c.EnableGatewayMode()
	case "mqtt-session":
		c.advanceMQTTSession()
	case "push-generation":
		c.resetMQTTPush(false)
	}
	close(msg.resume)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("notification callback did not finish")
	}
	got := c.GetState().Notifications
	if transition == "unchanged" {
		if len(got) != 1 || got[0].ID != "pending" || got[0].Ts != "2026-10-20T09:00:00Z" {
			t.Fatalf("current source notification lost or changed: %+v", got)
		}
	} else if len(got) != 0 {
		t.Fatalf("retired %s callback inserted notification: %+v", transition, got)
	}
}

func TestControllerNotificationIgnoresInactiveMQTTSource(t *testing.T) {
	c := newTestClient()
	c.EnableGatewayMode()
	callback := c.bindPushCallback(c.mqttSession.Load(), false)
	callback(nil, &fakeMessage{payload: []byte(`{"id":"inactive","title":"Inactive MQTT"}`)})
	if got := c.GetState().Notifications; len(got) != 0 {
		t.Fatalf("inactive MQTT inserted notification: %+v", got)
	}
	c.DisableGatewayMode()
	callback(nil, &fakeMessage{payload: []byte(`{"id":"current","title":"Current MQTT"}`)})
	if got := c.GetState().Notifications; len(got) != 1 || got[0].ID != "current" {
		t.Fatalf("current recovery session notification lost: %+v", got)
	}
}

func TestNotificationStateCommitCannotStraddleSourceRetirement(t *testing.T) {
	for _, retirement := range []string{"session", "generation"} {
		t.Run(retirement, func(t *testing.T) {
			c := newTestClient()
			c.stateMu.Lock()
			callbackDone := make(chan struct{})
			go func() {
				defer close(callbackDone)
				c.bindPushCallback(c.mqttSession.Load(), false)(nil, &fakeMessage{
					payload: []byte(`{"id":"in-progress","title":"In progress"}`),
				})
			}()
			// Wait until the callback holds the source guard and is blocked on
			// the state lock; this reproduces the old check-to-commit window.
			if !waitNotificationSourceGuard(c) {
				c.stateMu.Unlock()
				t.Fatal("callback did not enter source guard")
			}
			retired := make(chan struct{})
			go func() {
				defer close(retired)
				if retirement == "session" {
					c.advanceMQTTSession()
				} else {
					c.resetMQTTPush(false)
				}
			}()
			select {
			case <-retired:
				t.Error("source retired before in-flight notification state commit")
			case <-time.After(25 * time.Millisecond):
			}
			c.stateMu.Unlock()
			waitNotificationSignal(t, callbackDone, "state commit deadlocked")
			waitNotificationSignal(t, retired, "source retirement deadlocked")
		})
	}
}

func waitNotificationSignal(t *testing.T, done <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}

func waitNotificationSourceGuard(c *Client) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !c.gatewayMu.TryLock() {
			return true
		}
		c.gatewayMu.Unlock()
		time.Sleep(time.Millisecond)
	}
	return false
}

func TestPortalRefreshKeepsSchedulingSession(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	broker := &recordingBroker{}
	c.client = broker
	scheduledSession := c.mqttSession.Load()
	c.advanceMQTTSession()
	c.refreshPortal("p1", scheduledSession)
	if len(broker.handlers) != 0 {
		t.Fatal("delayed portal refresh rebound a retired subscription")
	}
	c.refreshPortal("p1", c.mqttSession.Load())
	if broker.handlers["N/p1/platform/+/#"] == nil {
		t.Fatal("current portal refresh did not subscribe")
	}
}
