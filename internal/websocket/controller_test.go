package websocket

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockha"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

type controllerMQTT struct {
	*mockmqtt.Client
	available bool
}

func (c *controllerMQTT) CanControlInverter() bool { return c.available }

func TestControllerFlagsUseAbsoluteCommandsWithoutHA(t *testing.T) {
	for _, haEnabled := range []bool{false, true} {
		c := &controllerMQTT{Client: mockmqtt.NewClient(), available: true}
		c.SetState(&state.State{Booleans: map[string]interface{}{"only_charging": false}})
		ha := mockha.NewClient()
		ha.SetDirectMode(haEnabled)
		ha.SetToggleAllowed("input_boolean.only_charging", true)
		if err := handleMessage(Message{Action: "toggle", Entity: "input_boolean.only_charging", State: "on"}, c, ha); err != nil {
			t.Fatal(err)
		}
		command := c.Published()[0]
		if command.Action != "toggle" || command.Payload["entity"] != "only_charging" || command.Payload["state"] != "on" {
			t.Fatalf("wrong control command: %+v", command)
		}
		if c.GetState().Booleans["only_charging"] != false {
			t.Fatal("command optimistically changed confirmed state")
		}
		if err := handleMessage(Message{Action: "dry_run", Value: true}, c, nil); err != nil {
			t.Fatal(err)
		}
		if c.Published()[1].Payload["value"] != true {
			t.Fatal("dry_run ignored explicit value")
		}
		if err := handleMessage(Message{Action: "ess_mode"}, c, nil); err != nil {
			t.Fatal(err)
		}
		if BuildPayload(c, nil)["controller_controls_available"] != true {
			t.Fatal("HA absence disabled controller buttons")
		}
	}
}

func TestControllerCommandsRejectUnavailableOrInvalidState(t *testing.T) {
	c := &controllerMQTT{Client: mockmqtt.NewClient(), available: false}
	for _, message := range []Message{{Action: "toggle", Entity: "no_feed", State: true}, {Action: "dry_run", Value: false}, {Action: "ess_mode"}} {
		if err := handleMessage(message, c, nil); err == nil {
			t.Fatalf("unavailable command accepted: %+v", message)
		}
	}
	c.available = true
	for _, message := range []Message{{Action: "toggle", Entity: "no_feed"}, {Action: "toggle", Entity: "no_feed", State: 2}, {Action: "dry_run", Value: "false"}} {
		if err := handleMessage(message, c, nil); err == nil {
			t.Fatalf("invalid/unknown command accepted: %+v", message)
		}
	}
	if len(c.Published()) != 0 {
		t.Fatal("rejected command reached transport")
	}
}

func TestHAOverlayCannotReplaceControllerOrEVState(t *testing.T) {
	available := true
	st := &state.State{InverterAvailable: &available, Booleans: map[string]interface{}{"no_feed": false}, EVChargingPower: 0, CarChargingPower: 3200, ESSMode: state.ESSMode{ModeName: "External control", IsExternal: true}}
	payload := mergeStates(st, homeassistant.Overlay{HADirectConnected: true, Booleans: map[string]bool{"no_feed": true}, AdditionalFields: map[string]interface{}{"inverter_available": false, "car_charging_power": 9999, "ev_charging_power": 9000, "ess_mode": "HA"}}, nil)
	if payload["inverter_available"] != true || payload["car_charging_power"] != float64(3200) || payload["ev_charging_power"] != float64(0) || payload["booleans"].(map[string]interface{})["no_feed"] != false {
		t.Fatalf("HA contaminated controller/native state: %+v", payload)
	}
}
