package websocket

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

func TestHomeStatesUseConfiguredBooleanKeys(t *testing.T) {
	for _, tc := range []struct {
		name              string
		direct, connected bool
		value             interface{}
		want              interface{}
	}{
		{"on", true, true, true, true},
		{"off", true, true, false, false},
		{"unknown", true, true, nil, nil},
		{"disconnected", true, false, true, nil},
		{"disabled", false, true, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]interface{}{"booleans": map[string]interface{}{"home_example": true, "only_charging": true}}
			ui := map[string]interface{}{"home_buttons": []homeassistant.Button{{StateKey: "home_example"}, {StateKey: "only_charging"}}}
			overlay := homeassistant.Overlay{HADirectConnected: tc.connected, AdditionalFields: map[string]interface{}{"home_example": tc.value, "only_charging": false}}
			mergeHomeButtonStates(payload, ui, overlay, tc.direct)
			booleans := payload["booleans"].(map[string]interface{})
			if booleans["home_example"] != tc.want || booleans["only_charging"] != true {
				t.Fatalf("Home states = %v", booleans)
			}
		})
	}
}

func TestEmptyLocalHomeConfigurationClearsUpstreamButtons(t *testing.T) {
	ui := mergeUIConfig(map[string]interface{}{"home_buttons": []interface{}{map[string]interface{}{"entity": "switch.legacy"}}}, nil)
	if buttons, ok := ui["home_buttons"].([]homeassistant.Button); !ok || len(buttons) != 0 {
		t.Fatalf("upstream Home fallback survived: %#v", ui)
	}
}

func TestUnconfiguredHomeCommandsNeverFallBackToMQTT(t *testing.T) {
	disabled := false
	for _, direct := range []bool{false, true} {
		client := homeassistant.NewClient(&config.HomeAssistantConfig{
			URL: "http://ha.invalid", Token: "test-token", DirectControls: direct,
			SwitchEntities: []config.EntityConfig{{Key: "home_disabled", Entity: "switch.disabled", Enabled: &disabled}},
		})
		for _, message := range []Message{
			{Action: "toggle", Entity: "switch.disabled"},
			{Action: "toggle", Entity: "light.removed"},
			{Action: "toggle", Entity: "button.removed"},
			{Action: "press", Entity: "button.removed"},
		} {
			mqtt := mockmqtt.NewClient()
			if err := handleMessage(message, mqtt, client); err == nil {
				t.Fatalf("unconfigured Home request accepted: %v", message)
			}
			if len(mqtt.Published()) != 0 {
				t.Fatal("rejected Home request reached MQTT")
			}
		}
	}
}
