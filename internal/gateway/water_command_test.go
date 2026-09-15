package gateway

import "testing"

func TestWaterCommandNormalization(t *testing.T) {
	if !IsWhitelistedCommand("water_mode") {
		t.Fatal("native command unavailable")
	}
	for _, body := range []interface{}{
		map[string]interface{}{"instance": 0, "mode": 2},
		map[string]interface{}{"instance": float64(8), "mode": float64(0)},
	} {
		if _, err := normalizeControllerCommand("water_mode", body); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []interface{}{
		nil, map[string]interface{}{},
		map[string]interface{}{"instance": true, "mode": 1},
		map[string]interface{}{"instance": "1", "mode": 1},
		map[string]interface{}{"instance": 1.5, "mode": 1},
		map[string]interface{}{"instance": -1, "mode": 1},
		map[string]interface{}{"instance": 1, "mode": true},
		map[string]interface{}{"instance": 1, "mode": 3},
		map[string]interface{}{"instance": 1, "mode": 1, "topic": "W/other"},
	} {
		if _, err := normalizeControllerCommand("water_mode", body); err == nil {
			t.Fatalf("invalid native command accepted: %#v", body)
		}
	}
}
