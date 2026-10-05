package state

import (
	"encoding/json"
	"math"
	"testing"
)

func TestNotificationTime(t *testing.T) {
	for _, value := range []any{1791226020, int64(1791226020), float64(1791226020), "1791226020", "1791226020.0", "179122602e1", json.Number("1791226020")} {
		if got := NotificationTime(value); got != "2026-10-05T18:47:00Z" {
			t.Errorf("NotificationTime(%v) = %q", value, got)
		}
	}
	for _, value := range []any{nil, true, false, "", "invalid", "NaN", "Inf", 0, -1, 1791226020.5, "1791226020.5", "1791226020.000000001", json.Number("1791226020.000000001"), 253402300800.0, int64(math.MaxInt64), math.NaN(), math.Inf(1)} {
		if got := NotificationTime(value); got != "" {
			t.Errorf("invalid NotificationTime(%v) = %q, want unknown", value, got)
		}
	}
	if got := NotificationTime(253402300799.0); got != "9999-12-31T23:59:59Z" {
		t.Errorf("valid future date = %q, want unchanged source time", got)
	}
}
