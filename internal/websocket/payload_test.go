package websocket

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockha"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

type connectedMQTT struct {
	*mockmqtt.Client
	connected bool
}

func (c *connectedMQTT) IsConnected() bool { return c.connected }

func TestBuildPayloadReportsCurrentSourceHealth(t *testing.T) {
	client := &connectedMQTT{Client: mockmqtt.NewClient(), connected: true}
	if BuildPayload(client, nil)["mqtt_connected"] != true {
		t.Fatal("connected source is not reported")
	}
	client.connected = false
	if BuildPayload(client, nil)["mqtt_connected"] != false {
		t.Fatal("source disconnection is not emitted in snapshot")
	}
	if _, exists := BuildPayload(mockmqtt.NewClient(), nil)["mqtt_connected"]; exists {
		t.Fatal("snapshot invented a connection status for an older client")
	}
}

func TestBuildPayloadPreservesFullSnapshotAndControllerUI(t *testing.T) {
	var s state.State
	if err := json.Unmarshal([]byte(`{
		"booleans":{"only_charging":true}, "gt":123,
		"batteries":[{"name":"Battery","soc":60,"voltage":51}],
		"loads":{"22":130}, "load_names":{"22":"Kitchen"},
		"daily_stats":{"produced_today":1.2,"produced_yesterday":4.5},
		"ui_config":{"header_toggles":[{"id":"only_charging"}],"home_buttons":[{"id":"old"}]},
		"limits":{"min":-2000,"max":2000},"loop_interval":0.33
	}`), &s); err != nil {
		t.Fatal(err)
	}
	mqtt := mockmqtt.NewClient()
	mqtt.SetState(&s)
	ha := mockha.NewClient()
	ha.SetDirectMode(true)
	ha.SetOverlay(homeassistant.Overlay{
		HADirectConnected: true,
		AdditionalFields:  map[string]interface{}{"washer_time": 300.0, "loads": map[string]float64{"old": 999}},
	})
	ha.SetUIConfig(map[string]interface{}{"home_buttons": []interface{}{map[string]interface{}{"id": "laundry"}}})
	payload := BuildPayload(mqtt, ha)
	for _, key := range []string{"batteries", "loads", "load_names", "daily_stats", "limits", "loop_interval", "washer_time", "ui_config"} {
		if payload[key] == nil {
			t.Errorf("full snapshot lost %s", key)
		}
	}
	config := payload["ui_config"].(map[string]interface{})
	if config["header_toggles"] == nil || config["home_buttons"] == nil || config["settings"] == nil {
		t.Errorf("controller and local UI configuration not merged: %#v", config)
	}
	if s.UIConfig["home_buttons"].([]interface{})[0].(map[string]interface{})["id"] != "old" {
		t.Fatal("local overlay mutated controller UI configuration")
	}
	if !reflect.DeepEqual(payload, buildPayload(mqtt, ha, ha.GetOverlay())) {
		t.Fatal("HTTP/initial state differs from broadcast snapshot")
	}
	if payload["loads"].(map[string]interface{})["22"] != 130.0 {
		t.Fatal("legacy HA loads replaced Cerbo loads")
	}
}
