package gateway

import (
	"bytes"
	"encoding/json"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"strings"
)

// Snapshot is the curated Cerbo leaf map returned by GET /v1/snapshot.
// Keys are "<instance>/<DBusPath>" (e.g. "0/Ac/Grid/L1/Power").
type Snapshot struct {
	Capabilities    map[string]bool            `json:"capabilities"`
	InverterPresent bool                       `json:"-"`
	Inverter        map[string]interface{}     `json:"inverter"`
	Grid            map[string]json.RawMessage `json:"grid"`
	System          map[string]json.RawMessage `json:"system"`
	Vebus           map[string]json.RawMessage `json:"vebus"`
	Battery         map[string]json.RawMessage `json:"battery"`
	Solarcharger    map[string]json.RawMessage `json:"solarcharger"`
	Pvinverter      map[string]json.RawMessage `json:"pvinverter"`
	Tank            map[string]json.RawMessage `json:"tank"`
	Pump            map[string]json.RawMessage `json:"pump"`
	EV              map[string]json.RawMessage `json:"ev"`
	EVCharger       map[string]json.RawMessage `json:"evcharger"`
	ACLoad          map[string]json.RawMessage `json:"acload"`
	Platform        map[string]json.RawMessage `json:"platform"`
	Settings        map[string]json.RawMessage `json:"settings"`
}

// leafMap is a decoded service map with flexible JSON values.
type leafMap map[string]any

func decodeLeaves(raw map[string]json.RawMessage) leafMap {
	out := make(leafMap, len(raw))
	for k, v := range raw {
		if len(v) == 0 || string(v) == "null" {
			out[k] = nil
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			continue
		}
		out[k] = val
	}
	return out
}

func decodePlatformLeaves(raw map[string]json.RawMessage) leafMap {
	out := decodeLeaves(raw)
	for key, value := range raw {
		parts := strings.Split(key, "/")
		if len(parts) < 4 || parts[1] != "Notifications" || parts[3] != "DateTime" || !json.Valid(value) {
			continue
		}
		// Only event time needs exact numeric text. Keep all other leaves in
		// the float64 representation expected by the existing telemetry maps.
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var exact any
		if err := decoder.Decode(&exact); err == nil {
			out[key] = exact
		}
	}
	return out
}

func (s *Snapshot) decoded() snapshotLeaves {
	return snapshotLeaves{
		Grid:         decodeLeaves(s.Grid),
		System:       decodeLeaves(s.System),
		Vebus:        decodeLeaves(s.Vebus),
		Battery:      decodeLeaves(s.Battery),
		Solarcharger: decodeLeaves(s.Solarcharger),
		Pvinverter:   decodeLeaves(s.Pvinverter),
		Tank:         decodeLeaves(s.Tank),
		Pump:         decodeLeaves(s.Pump),
		EV:           decodeLeaves(s.EV),
		EVCharger:    decodeLeaves(s.EVCharger),
		ACLoad:       decodeLeaves(s.ACLoad),
		Platform:     decodePlatformLeaves(s.Platform),
		Settings:     decodeLeaves(s.Settings),
	}
}

type snapshotLeaves struct {
	Grid                                                  leafMap
	System, Vebus, Battery, Solarcharger, Pvinverter      leafMap
	Tank, Pump, EV, EVCharger, ACLoad, Platform, Settings leafMap
}

// UnmarshalJSON distinguishes older gateways from an explicit controller clear.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	type snapshotAlias Snapshot
	var decoded snapshotAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, decoded.InverterPresent = fields["inverter"]
	if decoded.InverterPresent {
		var err error
		decoded.Inverter, err = state.DecodeControllerState(fields["inverter"])
		if err != nil {
			return err
		}
	}
	*s = Snapshot(decoded)
	return nil
}
