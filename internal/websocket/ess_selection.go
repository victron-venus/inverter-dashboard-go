package websocket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// Reject extra fields in explicit ESS envelopes instead of silently dropping
// them while decoding. Other legacy actions keep their established decoding.
func (m *Message) UnmarshalJSON(data []byte) error {
	type wireMessage Message
	var decoded wireMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.RequestID != "" && !state.ValidRequestID(decoded.RequestID) {
		return fmt.Errorf("invalid command request ID")
	}
	if decoded.Action == "set_ess_mode" || decoded.Action == "set_setpoint_override" || decoded.Action == "electricity_tariff" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		key, count := "mode", 3
		if decoded.Action == "set_setpoint_override" {
			key = "value"
			if raw, exists := fields[key]; exists {
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				if err := decoder.Decode(&decoded.Value); err != nil {
					return err
				}
			}
		}
		if decoded.Action == "electricity_tariff" {
			key = "plan"
			count = 4
			if fields["revision"] == nil {
				return fmt.Errorf("missing tariff revision")
			}
		}
		if len(fields) != count || fields[key] == nil || fields["request_id"] == nil {
			return fmt.Errorf("invalid ESS mode selection")
		}
	}
	*m = Message(decoded)
	return nil
}
