package state

import (
	"encoding/json"
	"testing"
)

func TestNativeLevelDoesNotRoundFractionsOrUnderflow(t *testing.T) {
	for _, value := range []any{json.Number(""), json.Number("1.00000000000000001"), "1.00000000000000001", "1e-99999999", "", "NaN", "Inf", true} {
		if level, ok := NativeNotificationLevel(value); ok {
			t.Fatalf("invalid level %v accepted as %d", value, level)
		}
	}
	for _, value := range []any{json.Number("2.0"), "2e0", 2, 2.0} {
		if level, ok := NativeNotificationLevel(value); !ok || level != 2 {
			t.Fatalf("compatible level %v = %d,%v", value, level, ok)
		}
	}
	if level, ok := NativeNotificationLevel("0e-99999999"); !ok || level != 0 {
		t.Fatalf("exact zero rejected: %d %v", level, ok)
	}
}
