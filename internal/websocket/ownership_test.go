package websocket

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestHAOwnershipAcrossDisconnect(t *testing.T) {
	for _, connected := range []bool{true, false} {
		s := &state.State{
			Booleans: map[string]interface{}{"only_charging": true, "no_feed": false, "holiday": true},
			Loads:    map[string]float64{"22": 130}, WaterLevel: 0.5,
			BatterySOC: 35, EVPower: 200, CarSOC: 70,
		}
		o := homeassistant.Overlay{
			HADirectConnected: connected,
			Booleans:          map[string]bool{"only_charging": false, "no_feed": true, "holiday": false},
			AdditionalFields: map[string]interface{}{
				"loads": map[string]float64{"old": 999}, "water_level": 99.0,
				"battery_soc": 99.0, "ev_power": 999.0, "car_soc": 99.0,
				"washer_power": true, "washer_time": 5400.0, "room_temperature": 21.5,
			},
		}
		merged := mergeStates(s, o, []string{"only_charging", "no_feed", "holiday", "loads", "water_level", "car_soc", "washer_power", "washer_time"})
		b := merged["booleans"].(map[string]interface{})
		if b["only_charging"] != true || b["no_feed"] != false || b["holiday"] != false {
			t.Fatalf("connected=%v: ownership violated: %#v", connected, b)
		}
		for key, want := range map[string]float64{"water_level": 0.5, "battery_soc": 35, "ev_power": 200, "car_soc": 70} {
			if merged[key] != want {
				t.Errorf("connected=%v: %s=%v, want %v", connected, key, merged[key], want)
			}
		}
		loads := merged["loads"].(map[string]interface{})
		if len(loads) != 1 || loads["22"] != 130.0 {
			t.Errorf("Cerbo loads overwritten: %#v", loads)
		}
		if merged["washer_power"] != connected {
			t.Errorf("HA appliance state lost: %#v", merged["washer_power"])
		}
		if connected && (merged["washer_time"] != 5400.0 || merged["room_temperature"] != 21.5) {
			t.Errorf("HA data missing: %#v", merged)
		}
		if s.Booleans["holiday"] != true || s.Booleans["only_charging"] != true {
			t.Fatal("merge mutated MQTT state")
		}
	}
}

func TestNoHAKeepsControllerFlags(t *testing.T) {
	s := &state.State{Booleans: map[string]interface{}{"only_charging": true}}
	merged := mergeStates(s, homeassistant.Overlay{}, nil)
	if merged["booleans"].(map[string]interface{})["only_charging"] != true {
		t.Fatal("absent HA client cleared controller flag")
	}
}
