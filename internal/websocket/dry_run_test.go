package websocket

import (
	"encoding/json"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/gateway"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

func TestGatewayDryRunFalseSurvivesFullPayloadAndClearsToUnknown(t *testing.T) {
	client := mqtt.NewClient("", 0)
	client.EnableGatewayMode()
	client.SetGatewayConnected(true)
	for _, sample := range []struct {
		name, snapshot string
		want           interface{}
	}{
		{"off", `{"inverter":{"dry_run":false}}`, false},
		{"on", `{"inverter":{"dry_run":true}}`, true},
		{"off again", `{"inverter":{"dry_run":false}}`, false},
		{"missing", `{"inverter":{"booleans":{"no_feed":false}}}`, nil},
		{"null", `{"inverter":{"dry_run":null}}`, nil},
		{"invalid", `{"inverter":{"dry_run":"invalid"}}`, nil},
		{"controller unavailable", `{"inverter":null}`, nil},
	} {
		t.Run(sample.name, func(t *testing.T) {
			var snapshot gateway.Snapshot
			if err := json.Unmarshal([]byte(sample.snapshot), &snapshot); err != nil {
				t.Fatal(err)
			}
			client.ApplyState(gateway.SnapshotToState(&snapshot, gateway.MapOptions{}))
			payload := BuildPayload(client, nil)
			value, exists := payload["dry_run"]
			if !exists || value != sample.want {
				t.Fatalf("dry_run=%v present=%v, want %v", value, exists, sample.want)
			}
			if sample.want != nil && payload["controller_controls_available"] != true {
				t.Fatal("known DRY state disabled healthy IGW controls")
			}
		})
	}
}

func TestDryRunCommandPreservesExplicitFalseAndRejectsUnknownToggle(t *testing.T) {
	client := &controllerMQTT{Client: mockmqtt.NewClient(), available: true}
	if err := handleMessage(Message{Action: "dry_run"}, client, nil); err == nil {
		t.Fatal("unknown DRY state accepted implicit toggle")
	}
	if len(client.Published()) != 0 {
		t.Fatal("unknown toggle reached transport")
	}
	for _, value := range []bool{false, true} {
		if err := handleMessage(Message{Action: "dry_run", Value: value}, client, nil); err != nil {
			t.Fatal(err)
		}
		commands := client.Published()
		if commands[len(commands)-1].Payload["value"] != value {
			t.Fatal("explicit DRY boolean changed")
		}
	}
}
