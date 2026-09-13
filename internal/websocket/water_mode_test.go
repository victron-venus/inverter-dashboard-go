package websocket

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

type waterCommander struct {
	*mockmqtt.Client
	available bool
	which     string
	mode      int
	calls     int
	err       error
}

func (c *waterCommander) SetWaterMode(which string, mode int) error {
	c.calls++
	c.which, c.mode = which, mode
	return c.err
}

func (c *waterCommander) CanControlWater() bool { return c.available }

func TestWaterModeDispatcher(t *testing.T) {
	for _, which := range []string{"pump", "valve"} {
		for mode := 0; mode <= 2; mode++ {
			client := &waterCommander{Client: mockmqtt.NewClient(), available: true}
			var msg Message
			if err := json.Unmarshal(fmt.Appendf(nil, `{"action":"water_mode","which":"%s","mode":%d}`, which, mode), &msg); err != nil {
				t.Fatal(err)
			}
			if err := handleMessage(msg, client, nil); err != nil {
				t.Fatal(err)
			}
			if client.calls != 1 || client.which != which || client.mode != mode || len(client.Published()) != 0 {
				t.Fatalf("incorrect native dispatch: %#v", client)
			}
			if BuildPayload(client, nil)["water_controls_available"] != true {
				t.Fatal("native water capability omitted")
			}
		}
	}
}

func TestWaterModeRejectsMalformedPayload(t *testing.T) {
	cases := []Message{
		{Action: "water_mode", Which: "other", Mode: 1},
		{Action: "water_mode", Which: "pump", Mode: -1},
		{Action: "water_mode", Which: "pump", Mode: 3},
		{Action: "water_mode", Which: "pump", Mode: 0.5},
		{Action: "water_mode", Which: "pump", Mode: true},
		{Action: "water_mode", Which: "pump", Mode: "1"},
		{Action: "water_mode", Which: "pump", Mode: math.NaN()},
		{Action: "water_mode", Which: "pump"},
	}
	for _, msg := range cases {
		client := &waterCommander{Client: mockmqtt.NewClient(), available: true}
		if err := handleMessage(msg, client, nil); err == nil || client.calls != 0 || len(client.Published()) != 0 {
			t.Errorf("invalid water payload accepted: %#v", msg)
		}
	}
}

func TestWaterModeUnsupportedAndPublishFailures(t *testing.T) {
	msg := Message{Action: "water_mode", Which: "pump", Mode: 1}
	legacy := mockmqtt.NewClient()
	if err := handleMessage(msg, legacy, nil); err == nil || len(legacy.Published()) != 0 {
		t.Fatal("unsupported native control reached legacy command plane")
	}
	if BuildPayload(legacy, nil)["water_controls_available"] != false {
		t.Fatal("unsupported transport advertises water control")
	}
	client := &waterCommander{Client: legacy, err: fmt.Errorf("direct MQTT is unavailable")}
	if err := handleMessage(msg, client, nil); err == nil || len(client.Published()) != 0 {
		t.Fatal("failed native write fell back to controller")
	}
}
