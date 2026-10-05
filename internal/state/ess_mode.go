package state

import (
	"errors"
	"math"
	"regexp"
	"time"
)

var essRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// ValidateESSSelection matches the controller's six explicit modes and bounded
// correlation ID. Extra fields cannot smuggle a different command envelope.
func ValidateESSSelection(payload any) error {
	object, ok := payload.(map[string]interface{})
	if !ok || len(object) != 2 {
		return errors.New("invalid ESS mode selection")
	}
	mode, _ := object["mode"].(string)
	id, _ := object["request_id"].(string)
	switch mode {
	case "off", "on", "optimized_with_battery_life", "optimized_without_battery_life", "keep_batteries_charged", "external_control":
		if essRequestID.MatchString(id) {
			return nil
		}
	}
	return errors.New("invalid ESS mode selection")
}

func ESSSelectionReady(st *State, now time.Time) bool {
	if st == nil || !st.ESSMode.SelectionSupported || st.ESSModeObservedAt == nil || st.DryRun == nil || *st.DryRun || st.InverterAvailable == nil || !*st.InverterAvailable {
		return false
	}
	at := *st.ESSModeObservedAt
	age := float64(now.UnixMilli())/1000 - at
	return !math.IsNaN(at) && !math.IsInf(at, 0) && age >= 0 && age <= 30
}
