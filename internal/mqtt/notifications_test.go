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

func TestStateNotificationsJSONOmittedWhenEmpty(t *testing.T) {
	out, _ := json.Marshal(state.State{})
	if strings.Contains(string(out), "notifications") {
		t.Fatalf("empty notifications leaked: %s", out)
	}
}
