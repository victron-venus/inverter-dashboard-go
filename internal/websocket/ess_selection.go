package websocket

import (
	"encoding/json"
	"fmt"
)

// Reject extra fields in explicit ESS envelopes instead of silently dropping
// them while decoding. Other legacy actions keep their established decoding.
func (m *Message) UnmarshalJSON(data []byte) error {
	type wireMessage Message
	var decoded wireMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Action == "set_ess_mode" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if len(fields) != 3 || fields["mode"] == nil || fields["request_id"] == nil {
			return fmt.Errorf("invalid ESS mode selection")
		}
	}
	*m = Message(decoded)
	return nil
}
