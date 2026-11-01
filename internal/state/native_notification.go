package state

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// ParsePlatformNotificationKey accepts only native notification slot leaves.
// Unrelated platform metadata must not disable the raw-alarm fallback.
func ParsePlatformNotificationKey(key string) (uint32, uint32, string, bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 4 || parts[1] != "Notifications" {
		return 0, 0, "", false
	}
	instance, err1 := strconv.ParseUint(parts[0], 10, 32)
	slot, err2 := strconv.ParseUint(parts[2], 10, 32)
	if err1 != nil || err2 != nil || slot > 20 {
		return 0, 0, "", false
	}
	switch parts[3] {
	case "Description", "DeviceName", "Service", "DateTime", "Type", "Acknowledged", "Active", "Silenced":
		return uint32(instance), uint32(slot), parts[3], true
	default:
		return 0, 0, "", false
	}
}

// NativeNotificationLevel accepts the exact native 0/1/2 enum, including
// numeric strings emitted by some gateways. Booleans and fractions are not enums.
func NativeNotificationLevel(value any) (int, bool) {
	var number float64
	var text string
	textValue := false
	switch v := value.(type) {
	case float64:
		number = v
	case float32:
		number = float64(v)
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	case json.Number:
		text, textValue = string(v), true
	case string:
		text, textValue = strings.TrimSpace(v), true
	default:
		return 0, false
	}
	if textValue {
		var err error
		number, err = strconv.ParseFloat(text, 64)
		if err != nil || (number != 0 && number != 1 && number != 2) {
			return 0, false
		}
		if number == 0 {
			// Underflow is not zero. Avoid expanding an attacker-controlled
			// negative exponent merely to discover a nonzero fraction.
			mantissa, _, _ := strings.Cut(strings.ToLower(text), "e")
			for _, digit := range mantissa {
				if digit >= '1' && digit <= '9' {
					return 0, false
				}
			}
			return 0, true
		}
		exact, ok := new(big.Rat).SetString(text)
		if !ok || !exact.IsInt() {
			return 0, false
		}
	}
	if math.IsNaN(number) || (number != 0 && number != 1 && number != 2) {
		return 0, false
	}
	return int(number), true
}

// NativeNotificationBool reads flag leaves without coercing arbitrary numbers.
func NativeNotificationBool(value any) (bool, bool) {
	if b, ok := value.(bool); ok {
		return b, true
	}
	if text, ok := value.(string); ok {
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	level, ok := NativeNotificationLevel(value)
	return level == 1, ok && level <= 1
}

// ValidPlatformNotificationValue rejects malformed leaves before they can
// claim authority over raw alarms. Invalid scalar dates deliberately remain
// valid updates: they clear stale event time instead of inventing receipt time.
func ValidPlatformNotificationValue(field string, value any) bool {
	if value == nil {
		return true
	}
	switch field {
	case "Description", "DeviceName", "Service":
		_, ok := value.(string)
		return ok
	case "DateTime":
		switch value.(type) {
		case string, json.Number, float64, float32, int, int64, bool:
			return true
		}
	case "Type":
		_, ok := NativeNotificationLevel(value)
		return ok
	case "Acknowledged", "Active", "Silenced":
		_, ok := NativeNotificationBool(value)
		return ok
	}
	return false
}
