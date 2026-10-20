package mqtt

import (
	"sync"
	"testing"
	"time"
)

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
		})
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
			guardHeld := false
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				if !c.gatewayMu.TryLock() {
					guardHeld = true
					break
				}
				c.gatewayMu.Unlock()
				time.Sleep(time.Millisecond)
			}
			if !guardHeld {
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
			for _, done := range []chan struct{}{callbackDone, retired} {
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("state commit or source retirement deadlocked")
				}
			}
		})
	}
}
