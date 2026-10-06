package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"time"
)

type SetpointOverrideStatus struct {
	Value     *int32  `json:"value"`
	LastError *string `json:"last_error"`
	RequestID *string `json:"request_id"`
}

func ValidRequestID(id string) bool { return essRequestID.MatchString(id) }

func int32Value(v any) (*int32, bool) {
	if v == nil {
		return nil, true
	}
	var n float64
	switch v := v.(type) {
	case json.Number:
		i, err := strconv.ParseInt(string(v), 10, 32)
		if err != nil {
			return nil, false
		}
		out := int32(i)
		return &out, true
	case float64:
		n = v
	case int:
		n = float64(v)
	case int32:
		n = float64(v)
	default:
		return nil, false
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < math.MinInt32 || n > math.MaxInt32 {
		return nil, false
	}
	out := int32(n)
	return &out, true
}

// ParseOverrideStatus requires every acknowledgement field; null value is a
// supported stopped override, whereas a missing/invalid envelope is unknown.
func ParseOverrideStatus(raw any) *SetpointOverrideStatus {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	for _, key := range []string{"value", "last_error", "request_id"} {
		if _, ok := m[key]; !ok {
			return nil
		}
	}
	value, ok := int32Value(m["value"])
	if !ok {
		return nil
	}
	status := &SetpointOverrideStatus{Value: value}
	for key, dst := range map[string]**string{"last_error": &status.LastError, "request_id": &status.RequestID} {
		if m[key] == nil {
			continue
		}
		s, ok := m[key].(string)
		if !ok {
			return nil
		}
		*dst = &s
	}
	return status
}

func ValidateOverride(payload any) error {
	m, ok := payload.(map[string]interface{})
	if !ok || len(m) != 2 {
		return errors.New("invalid override command")
	}
	_, present := m["value"]
	_, valid := int32Value(m["value"])
	id, _ := m["request_id"].(string)
	if !present || !valid || !ValidRequestID(id) {
		return errors.New("invalid override command")
	}
	return nil
}

var tariffRevision = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidateTariff(payload any) error {
	m, ok := payload.(map[string]interface{})
	if !ok || len(m) != 3 {
		return errors.New("invalid tariff command")
	}
	id, _ := m["request_id"].(string)
	revision, _ := m["revision"].(string)
	plan, present := m["plan"]
	_, object := plan.(map[string]interface{})
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if !ValidRequestID(id) || !tariffRevision.MatchString(revision) || !present || (plan != nil && !object) || encoder.Encode(payload) != nil || encoded.Len()-1 > 100000 {
		return errors.New("invalid tariff command")
	}
	return nil
}

func ObservationFresh(at *float64, now time.Time) bool {
	if at == nil {
		return false
	}
	age := float64(now.UnixMilli())/1000 - *at
	return !math.IsNaN(*at) && !math.IsInf(*at, 0) && age >= 0 && age <= 30
}

func ControllerCommandReady(st *State, name string, payload any, now time.Time) bool {
	if st == nil || st.InverterAvailable == nil || !*st.InverterAvailable {
		return false
	}
	switch name {
	case "setpoint_override":
		return st.SetpointOverride != nil && ObservationFresh(st.SetpointOverrideObservedAt, now)
	case "electricity_tariff":
		status, _ := st.UIConfig["electricity_tariff_status"].(map[string]interface{})
		writable, _ := status["writable"].(bool)
		rev, _ := status["revision"].(string)
		body, _ := payload.(map[string]interface{})
		matches := payload == nil || body["revision"] == rev
		return writable && tariffRevision.MatchString(rev) && matches && ObservationFresh(st.ElectricityTariffObservedAt, now)
	}
	return false
}

func OverrideAcknowledged(st *State, payload any) (bool, error) {
	m, _ := payload.(map[string]interface{})
	s := st.SetpointOverride
	if s == nil || s.RequestID == nil || *s.RequestID != m["request_id"] {
		return false, nil
	}
	if s.LastError != nil {
		return false, errors.New("controller rejected override")
	}
	expected, _ := int32Value(m["value"])
	equal := (expected == nil && s.Value == nil) || (expected != nil && s.Value != nil && *expected == *s.Value)
	return equal, nil
}

// DecodeControllerState preserves the lexical integer of the safety-critical
// override field without changing numeric types in other telemetry.
func DecodeControllerState(data []byte) (map[string]interface{}, error) {
	var values map[string]interface{}
	if len(data) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if raw, ok := fields["setpoint_override"]; ok {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var exact any
		if err := decoder.Decode(&exact); err != nil {
			return nil, err
		}
		values["setpoint_override"] = exact
	}
	return values, nil
}
