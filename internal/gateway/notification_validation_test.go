package gateway

import (
	"encoding/json"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestMalformedPlatformLeavesDoNotSuppressRawAlarms(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"0/Notifications/1/Unknown", "noise"},
		{"0/Notifications/1/Description/extra", "noise"},
		{"bad/Notifications/1/Description", "noise"},
		{"0/Notifications/21/Description", "noise"},
		{"0/Notifications/1/Description", map[string]any{}},
		{"0/Notifications/1/DateTime", []any{}},
		{"0/Notifications/1/Acknowledged", 99.0},
		{"0/Notifications/1/Type", 0.5},
	} {
		t.Run(tc.key, func(t *testing.T) {
			got := mapNotifications(snapshotLeaves{Platform: leafMap{tc.key: tc.value}, Battery: leafMap{"0/Alarms/LowVoltage": 2.0}})
			if len(got) != 1 || got[0].ID != "victron-battery/0/Alarms/LowVoltage" {
				t.Fatalf("malformed platform leaf suppressed raw alarm: %+v", got)
			}
		})
	}
}

func TestRawGatewayAlarmsRequireDiscreteLevel(t *testing.T) {
	for _, value := range []any{true, false, 1.5, -1.0, 3.0, "1.00000000000000001", "NaN", map[string]any{}} {
		var got []state.Notification
		mapAlarmLeaves("battery", leafMap{"0/Alarms/LowVoltage": value}, &got)
		if len(got) != 0 {
			t.Fatalf("invalid raw alarm %v produced banner: %+v", value, got)
		}
	}
	for _, value := range []any{"1", "2", 1.0, 2.0} {
		var got []state.Notification
		mapAlarmLeaves("battery", leafMap{"0/Alarms/LowVoltage": value}, &got)
		if len(got) != 1 {
			t.Fatalf("compatible discrete alarm %v missing: %+v", value, got)
		}
	}
}

func TestWireAlarmEnumPreservesExactValue(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(`{"battery":{"1/Alarms/LowVoltage":1.00000000000000001},"system":{"0/Ac/Grid/L1/Power":100.25}}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	got := SnapshotToState(&snapshot, MapOptions{})
	if len(got.Notifications) != 0 {
		t.Fatalf("fractional wire alarm became live enum: %+v", got.Notifications)
	}
	if got.G1 != 100.25 {
		t.Fatalf("ordinary telemetry representation changed: %v", got.G1)
	}
}

func TestWireAcknowledgementPreservesExactValue(t *testing.T) {
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(`{"platform":{"0/Notifications/1/Description":"Keep visible","0/Notifications/1/Acknowledged":1.00000000000000001}}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	got := SnapshotToState(&snapshot, MapOptions{})
	if len(got.Notifications) != 1 {
		t.Fatalf("fractional wire flag acknowledged banner: %+v", got.Notifications)
	}
}

func TestPlatformNotificationIDsRetainUint32Range(t *testing.T) {
	got := mapNotifications(snapshotLeaves{Platform: leafMap{
		"4294967295/Notifications/0/Description":  "Maximum instance",
		"2147483648/Notifications/20/Description": "High instance, last slot",
		"0/Notifications/20/Description":          "Zero instance",
		"2147483648/Notifications/0/Description":  "High instance, first slot",
	}})
	want := []string{
		"victron-platform-0-20",
		"victron-platform-2147483648-0",
		"victron-platform-2147483648-20",
		"victron-platform-4294967295-0",
	}
	if len(got) != len(want) {
		t.Fatalf("valid uint32 instances were dropped: %+v", got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("notification %d ID = %q, want %q", i, got[i].ID, id)
		}
	}
}
