package gateway

import "encoding/json"

// Snapshot is the curated Cerbo leaf map returned by GET /v1/snapshot.
// Keys are "<instance>/<DBusPath>" (e.g. "0/Ac/Grid/L1/Power").
type Snapshot struct {
	Grid         map[string]json.RawMessage `json:"grid"`
	System       map[string]json.RawMessage `json:"system"`
	Vebus        map[string]json.RawMessage `json:"vebus"`
	Battery      map[string]json.RawMessage `json:"battery"`
	Solarcharger map[string]json.RawMessage `json:"solarcharger"`
	Pvinverter   map[string]json.RawMessage `json:"pvinverter"`
	Tank         map[string]json.RawMessage `json:"tank"`
	Pump         map[string]json.RawMessage `json:"pump"`
	EV           map[string]json.RawMessage `json:"ev"`
	EVCharger    map[string]json.RawMessage `json:"evcharger"`
	ACLoad       map[string]json.RawMessage `json:"acload"`
	Platform     map[string]json.RawMessage `json:"platform"`
	Settings     map[string]json.RawMessage `json:"settings"`
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
		Platform:     decodeLeaves(s.Platform),
		Settings:     decodeLeaves(s.Settings),
	}
}

type snapshotLeaves struct {
	Grid                                                  leafMap
	System, Vebus, Battery, Solarcharger, Pvinverter      leafMap
	Tank, Pump, EV, EVCharger, ACLoad, Platform, Settings leafMap
}
