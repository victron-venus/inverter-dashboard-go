package state

import (
	"encoding/json"
	"math"
	"strconv"
	"time"
)

// NotificationTime converts a native Victron DateTime (Unix seconds) to its
// source event time. Missing or invalid values stay unknown, never receipt time.
// Numeric strings remain supported for gateway compatibility. Real future dates
// are preserved so clients can show clock skew honestly.
func NotificationTime(value any) string {
	var seconds float64
	switch v := value.(type) {
	case float64:
		seconds = v
	case float32:
		seconds = float64(v)
	case int:
		seconds = float64(v)
	case int64:
		seconds = float64(v)
	case json.Number:
		var err error
		seconds, err = v.Float64()
		if err != nil {
			return ""
		}
	case string:
		var err error
		seconds, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return ""
		}
	default:
		return ""
	}
	// RFC3339 has a four-digit year. Validate before converting to int64,
	// which would otherwise truncate fractions or overflow on corrupt input.
	const lastRFC3339Second = 253402300799 // 9999-12-31T23:59:59Z
	if math.IsNaN(seconds) || seconds <= 0 || seconds > lastRFC3339Second || math.Trunc(seconds) != seconds {
		return ""
	}
	return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
}
