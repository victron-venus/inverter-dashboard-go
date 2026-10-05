package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func newTestClient() *Client {
	return NewClient("localhost", 1883)
}

func TestNotificationFromControl(t *testing.T) {
	c := newTestClient()
	payload, _ := json.Marshal(map[string]string{"id": "n1", "level": "warning", "title": "T", "body": "B"})
	c.onNotificationMessage(nil, &fakeMessage{topic: "inverter/notifications", payload: payload})

	got := c.state.Notifications
	if len(got) != 1 || got[0].ID != "n1" || got[0].Source != "inverter-control" {
		t.Fatalf("unexpected notifications: %+v", got)
	}
	if got[0].Ts != "" {
		t.Fatalf("remote event without time received an invented timestamp: %q", got[0].Ts)
	}
}

func TestNotificationFromControlPreservesSourceTime(t *testing.T) {
	for _, timestamp := range []string{"2026-10-05T11:47:00-07:00", "2099-10-05T18:47:00Z", "invalid", ""} {
		c := newTestClient()
		payload, err := json.Marshal(map[string]string{"id": "n1", "title": "Battery", "ts": timestamp})
		if err != nil {
			t.Fatal(err)
		}
		// A remote reconnect/replay must keep source time, including unknown time.
		for range 2 {
			c.onNotificationMessage(nil, &fakeMessage{topic: "inverter/notifications", payload: payload})
			got := c.GetState().Notifications
			if got[len(got)-1].Ts != timestamp {
				t.Fatalf("source time %q changed: %+v", timestamp, got)
			}
		}
	}
}

func TestPlatformNotificationReplayEventTime(t *testing.T) {
	for _, dateTimeFirst := range []bool{false, true} {
		c := newTestClient()
		c.SetWaterConfig("p1", 21, 1, 2)
		c.client = &recordingBroker{}
		defer c.stopKeepalive()
		sendField := func(field string, value any) {
			t.Helper()
			c.onPlatformNotificationMessage(nil, cerboMsg("N/p1/platform/0/Notifications/1/"+field, value))
		}
		checkTime := func(want string) {
			t.Helper()
			got := c.GetState().Notifications
			if len(got) != 1 || got[0].Ts != want || got[0].Title != "Internal failure" {
				t.Fatalf("DateTime first=%v, notifications = %+v, want event time %q", dateTimeFirst, got, want)
			}
		}
		if dateTimeFirst {
			sendField("DateTime", 1791226020)
		}
		sendField("Description", "Internal failure")
		if !dateTimeFirst {
			checkTime("") // Partial MQTT replay is visible, with unknown time.
			sendField("DateTime", 1791226020)
		}
		const eventTime = "2026-10-05T18:47:00Z"
		checkTime(eventTime)
		sendField("DeviceName", "JBD Battery Chain 1")
		sendField("Acknowledged", false)
		sendField("Active", true)
		checkTime(eventTime)
		// Exercise the actual subscription reset used after reconnect, without a
		// network connection. Replayed fields must never become receipt time.
		c.invalidateCerbo()
		if err := c.Subscribe(); err != nil {
			t.Fatal(err)
		}
		sendField("Description", "Internal failure")
		sendField("DateTime", 1791226020)
		checkTime(eventTime)
		c.stopKeepalive()
		sendField("Acknowledged", true)
		if got := c.GetState().Notifications; len(got) != 0 {
			t.Fatalf("acknowledged replay still visible: %+v", got)
		}
	}
}

func TestPlatformInvalidDateTimeClearsPreviousTime(t *testing.T) {
	c := newTestClient()
	sendField := func(field string, value any) {
		c.onPlatformNotificationMessage(nil, cerboMsg("N/p1/platform/0/Notifications/1/"+field, value))
	}
	sendField("Description", "Internal failure")
	for _, value := range []any{nil, true, false, 0, -1, 1791226020.5, "invalid", "NaN", 253402300800.0, 9.22e18} {
		sendField("DateTime", 1791226020)
		sendField("DateTime", value)
		got := c.GetState().Notifications
		if len(got) != 1 || got[0].Ts != "" {
			t.Fatalf("invalid DateTime %v kept/invented a time: %+v", value, got)
		}
	}
	sendField("DateTime", 253402300799.0)
	if got := c.GetState().Notifications[0].Ts; got != "9999-12-31T23:59:59Z" {
		t.Fatalf("future source event time changed: %q", got)
	}
}

func TestNotificationRingCapped(t *testing.T) {
	c := newTestClient()
	for i := 0; i < maxNotifications+10; i++ {
		payload, _ := json.Marshal(map[string]string{"id": fmt.Sprintf("n%d", i), "title": "x"})
		c.onNotificationMessage(nil, &fakeMessage{topic: "inverter/notifications", payload: payload})
	}
	if len(c.state.Notifications) != maxNotifications {
		t.Fatalf("cap not applied: %d", len(c.state.Notifications))
	}
	if c.state.Notifications[0].ID != fmt.Sprintf("n%d", 10) {
		t.Fatalf("oldest not dropped: %s", c.state.Notifications[0].ID)
	}
}

func TestVictronAlarmTransitions(t *testing.T) {
	c := newTestClient()
	topic := "N/portal/battery_512/Alarms/HighCellVoltage"
	send := func(v string) {
		c.onAlarmMessage(nil, &fakeMessage{topic: topic, payload: []byte(fmt.Sprintf(`{"value": %s}`, v))})
	}

	send("2")
	if n := c.state.Notifications; len(n) != 1 {
		t.Fatalf("want 1 notification after alarm, got %d", len(n))
	} else {
		got := n[0]
		if got.Level != "alarm" || got.Title != "Battery 512" || got.Body != "High Cell Voltage: Alarm" || got.ID != "victron-"+topic {
			t.Fatalf("bad notification: %+v", got)
		}
		if got.Ts != "" {
			t.Fatalf("raw alarm without event time received an invented timestamp: %q", got.Ts)
		}
	}

	send("2") // no transition -> no duplicate
	if len(c.state.Notifications) != 1 {
		t.Fatalf("duplicate emitted on same value")
	}

	send("1") // transition to warning appends
	if n := c.state.Notifications; len(n) != 2 || n[1].Level != "warning" {
		t.Fatalf("warning transition missing: %+v", c.state.Notifications)
	}

	send("0") // cleared
	if len(c.state.Notifications) != 0 {
		t.Fatalf("clear-on-zero failed: %+v", c.state.Notifications)
	}
}

func TestPrettyNames(t *testing.T) {
	if got := prettyServiceName("battery_512"); got != "Battery 512" {
		t.Fatalf("prettyServiceName: %q", got)
	}
	if got := prettyServiceName("vebus"); got != "Vebus" {
		t.Fatalf("prettyServiceName: %q", got)
	}
	if got := prettyAlarmName("HighCellVoltage"); got != "High Cell Voltage" {
		t.Fatalf("prettyAlarmName: %q", got)
	}
	if got := prettyAlarmName("high_cell_voltage"); got != "High Cell Voltage" {
		t.Fatalf("prettyAlarmName: %q", got)
	}
}

func TestStateNotificationsJSONClearsEmpty(t *testing.T) {
	out, _ := json.Marshal(state.State{Notifications: []state.Notification{}})
	if !strings.Contains(string(out), `"notifications":[]`) {
		t.Fatalf("empty notifications must clear the UI: %s", out)
	}
}
