package mqtt

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestControllerNotificationReplayReplacesSameID(t *testing.T) {
	c := newTestClient()
	t.Cleanup(c.cmdBuffer.Stop)
	for _, payload := range []string{
		`{"id":"same","title":"First","level":"warning","ts":"2026-10-05T18:47:00Z"}`,
		`{"id":"same","title":"Updated","level":"alarm","ts":"2026-10-05T18:47:00Z"}`,
	} {
		c.onNotificationMessage(nil, &fakeMessage{payload: []byte(payload)})
	}
	got := c.GetState().Notifications
	if len(got) != 1 || got[0].Title != "Updated" || got[0].Ts != "2026-10-05T18:47:00Z" {
		t.Fatalf("replayed notification duplicated or lost source time: %+v", got)
	}
}

func TestControllerNotificationNormalizesIdentityAndLevel(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    *state.Notification
	}{
		{`null`, nil},
		{`{}`, nil},
		{`{"id":"  ","title":" \n"}`, nil},
		{`{"title":" Battery ","level":" WARNING "}`, &state.Notification{ID: "Battery", Title: "Battery", Level: "warning", Source: "inverter-control"}},
		{`{"id":" status ","level":"unknown"}`, &state.Notification{ID: "status", Title: "status", Level: "info", Source: "inverter-control"}},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			c := newTestClient()
			t.Cleanup(c.cmdBuffer.Stop)
			c.onNotificationMessage(nil, &fakeMessage{payload: []byte(tc.payload)})
			got := c.GetState().Notifications
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("empty notification accepted: %+v", got)
				}
			} else if len(got) != 1 || got[0] != *tc.want {
				t.Fatalf("notification = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestInvalidPlatformMessageCannotSuppressRawAlarms(t *testing.T) {
	for _, tc := range []struct{ suffix, payload string }{
		{"UnknownField", `{"value":1}`},
		{"Description/extra", `{"value":"bad"}`},
		{"Description", `{}`},
		{"Description", `{"value":{}}`},
		{"DateTime", `{"value":[]}`},
		{"Acknowledged", `{"value":99}`},
		{"Type", `{"value":0.5}`},
		{"Description", `null`},
		{"Description", `{"unrelated":"bad"}`},
	} {
		t.Run(tc.suffix+tc.payload, func(t *testing.T) {
			c := newTestClient()
			t.Cleanup(c.cmdBuffer.Stop)
			c.onPlatformNotificationMessage(nil, &fakeMessage{topic: "N/p1/platform/0/Notifications/1/" + tc.suffix, payload: []byte(tc.payload)})
			c.onAlarmMessage(nil, cerboMsg("N/p1/battery/0/Alarms/LowVoltage", 2))
			if got := c.GetState().Notifications; len(got) != 1 || got[0].ID != "victron-N/p1/battery/0/Alarms/LowVoltage" {
				t.Fatalf("malformed platform message disabled raw fallback: %+v", got)
			}
		})
	}
}

func TestRawAlarmRejectsNonDiscreteValuesWithoutClearing(t *testing.T) {
	for _, value := range []any{true, false, 1.5, -1, 3, "1.00000000000000001", "NaN", map[string]any{}} {
		c := newTestClient()
		t.Cleanup(c.cmdBuffer.Stop)
		topic := "N/p1/battery/0/Alarms/LowVoltage"
		c.onAlarmMessage(nil, cerboMsg(topic, 2))
		c.onAlarmMessage(nil, cerboMsg(topic, value))
		if got := c.GetState().Notifications; len(got) != 1 || got[0].Level != "alarm" {
			t.Fatalf("invalid alarm value %v changed live warning: %+v", value, got)
		}
	}
}

func TestPlatformCacheDoesNotCrossMQTTSessions(t *testing.T) {
	c := newTestClient()
	t.Cleanup(c.cmdBuffer.Stop)
	c.SetWaterConfig("p1", 21, 1, 2)
	c.client = &recordingBroker{}
	t.Cleanup(c.stopKeepalive)
	prefix := "N/p1/platform/0/Notifications/1/"
	c.onPlatformNotificationMessage(nil, cerboMsg(prefix+"Description", "Old event"))
	c.onPlatformNotificationMessage(nil, cerboMsg(prefix+"DateTime", 1791226020))
	c.onPlatformNotificationMessage(nil, cerboMsg(prefix+"Acknowledged", true))
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	c.onPlatformNotificationMessage(nil, cerboMsg(prefix+"Description", "New event"))
	got := c.GetState().Notifications
	if len(got) != 1 || got[0].Title != "New event" || got[0].Ts != "" {
		t.Fatalf("new event inherited old acknowledgment/time: %+v", got)
	}
}

func TestReconnectRestoresRawFallbackAndKeepsInactiveGatewayBanners(t *testing.T) {
	c := newTestClient()
	t.Cleanup(c.cmdBuffer.Stop)
	c.SetWaterConfig("p1", 21, 1, 2)
	c.client = &recordingBroker{}
	t.Cleanup(c.stopKeepalive)
	c.onPlatformNotificationMessage(nil, cerboMsg("N/p1/platform/0/Notifications/1/Description", "Old platform"))
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	c.onAlarmMessage(nil, cerboMsg("N/p1/battery/0/Alarms/LowVoltage", 2))
	if got := c.GetState().Notifications; len(got) != 1 || got[0].Title == "Old platform" {
		t.Fatalf("reconnect kept retired platform suppression: %+v", got)
	}
	c.EnableGatewayMode()
	c.ApplyState(&state.State{Notifications: []state.Notification{{ID: "victron-gateway", Title: "Gateway warning", Source: "victron"}}})
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	if got := c.GetState().Notifications; len(got) != 1 || got[0].ID != "victron-gateway" {
		t.Fatalf("background MQTT probe cleared active gateway warnings: %+v", got)
	}
}

func TestRawAlarmNumericStringsRemainCompatible(t *testing.T) {
	c := newTestClient()
	t.Cleanup(c.cmdBuffer.Stop)
	topic := "N/p1/battery/0/Alarms/LowVoltage"
	for _, value := range []string{"2", "1", "0"} {
		c.onAlarmMessage(nil, cerboMsg(topic, value))
		got := c.GetState().Notifications
		if value == "0" {
			if len(got) != 0 {
				t.Fatalf("clear string did not clear alarm: %+v", got)
			}
		} else if len(got) != 1 || (value == "2" && got[0].Level != "alarm") || (value == "1" && got[0].Level != "warning") {
			t.Fatalf("discrete numeric string %q not accepted: %+v", value, got)
		}
	}
}

func TestPlatformTombstoneOnlyClearsItsField(t *testing.T) {
	c := newTestClient()
	t.Cleanup(c.cmdBuffer.Stop)
	prefix := "N/p1/platform/0/Notifications/1/"
	for field, value := range map[string]any{"Description": "Battery warning", "DeviceName": "Battery", "DateTime": 1791226020, "Type": 0} {
		c.onPlatformNotificationMessage(nil, cerboMsg(prefix+field, value))
	}
	c.onPlatformNotificationMessage(nil, &fakeMessage{topic: prefix + "DeviceName"})
	got := c.GetState().Notifications
	if len(got) != 1 || got[0].Body != "" || got[0].Ts != "2026-10-05T18:47:00Z" {
		t.Fatalf("metadata tombstone erased event: %+v", got)
	}
	c.onPlatformNotificationMessage(nil, &fakeMessage{topic: prefix + "DateTime"})
	got = c.GetState().Notifications
	if len(got) != 1 || got[0].Ts != "" {
		t.Fatalf("time tombstone erased event: %+v", got)
	}
	c.onPlatformNotificationMessage(nil, &fakeMessage{topic: prefix + "Description"})
	if got = c.GetState().Notifications; len(got) != 0 {
		t.Fatalf("description tombstone kept event: %+v", got)
	}
}

func TestWirePlatformEnumAndFlagsPreserveExactValue(t *testing.T) {
	for _, field := range []string{"Type", "Acknowledged", "Active", "Silenced"} {
		_, valid := decodePlatformNotification(&fakeMessage{topic: "N/p1/platform/0/Notifications/1/" + field, payload: []byte(`{"value":1.00000000000000001}`)})
		if valid {
			t.Fatalf("fractional wire %s accepted", field)
		}
	}
}
