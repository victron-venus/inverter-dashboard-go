package mqtt

import (
	"testing"
	"time"
)

func TestSetWaterModeUsesConfiguredDeviceAndAwaitsReadback(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 3, 8)
	send(c, "pump/3", "Mode", 0)
	send(c, "pump/8", "Mode", 2)
	b := &recordingBroker{}
	c.client = b
	if !c.CanControlWater() {
		t.Fatal("live direct control capability missing")
	}
	if err := c.SetWaterMode("pump", 1); err != nil {
		t.Fatal(err)
	}
	if err := c.SetWaterMode("valve", 0); err != nil {
		t.Fatal(err)
	}
	if len(b.writes) != 2 || b.writes[0] != `W/p1/pump/3/Mode={"value":1}` || b.writes[1] != `W/p1/pump/8/Mode={"value":0}` {
		t.Fatalf("incorrect water command: %v", b.writes)
	}
	if st := c.GetState(); st.PumpMode != 0 || st.WaterValveMode != 2 {
		t.Fatal("publish optimistically changed observed state")
	}
}

func TestSetWaterModeRejectsInvalidUnavailableAndGatewayWrites(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 3, 8)
	b := &recordingBroker{}
	c.client = b
	for _, tc := range []struct {
		which string
		mode  int
	}{{"pump", -1}, {"pump", 3}, {"unknown", 0}, {"pump", 1}} {
		if err := c.SetWaterMode(tc.which, tc.mode); err == nil {
			t.Fatalf("accepted invalid/unavailable mode: %+v", tc)
		}
	}
	send(c, "pump/3", "Mode", 0)
	c.EnableGatewayMode()
	if c.CanControlWater() {
		t.Fatal("gateway advertised water control")
	}
	if err := c.SetWaterMode("pump", 1); err == nil {
		t.Fatal("gateway write accepted")
	}
	c.DisableGatewayMode()
	send(c, "pump/3", "Mode", nil)
	if err := c.SetWaterMode("pump", 1); err == nil {
		t.Fatal("invalidated Mode accepted")
	}
	if len(b.writes) != 0 {
		t.Fatalf("rejected commands were published: %v", b.writes)
	}
}

func TestSetWaterModeRejectsDisconnectedMQTT(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 3, 8)
	send(c, "pump/3", "Mode", 0)
	if err := c.SetWaterMode("pump", 1); err == nil {
		t.Fatal("disconnected write accepted")
	}
}

func TestWaterModesRequireFreshNonretainedOwnLeaf(t *testing.T) {
	for _, reason := range []string{"retained", "stale", "future", "unrelated", "live"} {
		t.Run(reason, func(t *testing.T) {
			c := NewClient("localhost", 1883)
			b := &recordingBroker{}
			c.client = b
			c.SetWaterConfig("p1", 21, 3, 8)
			send(c, "pump/3", "Mode", 0)
			switch reason {
			case "retained":
				c.onCerboLiveMessage(nil, &retainedESSMessage{cerboMsg("N/p1/pump/3/Mode", 0)})
			case "stale", "unrelated":
				c.waterModeObserved["pump_mode"] = time.Now().Add(-31 * time.Second)
			case "future":
				c.waterModeObserved["pump_mode"] = time.Now().Add(time.Minute)
			}
			if reason == "unrelated" {
				send(c, "pump/3", "State", 1)
			}
			err := c.SetWaterMode("pump", 1)
			if (err == nil) != (reason == "live") {
				t.Fatalf("error=%v", err)
			}
			if reason != "live" && len(b.writes) != 0 {
				t.Fatal("stale Mode dispatched physical write")
			}
		})
	}
	if keepaliveInterval != 20*time.Second || keepaliveInterval >= 30*time.Second {
		t.Fatal("read-only refresh must precede command expiry")
	}
}

func TestWaterRefreshReadsOnlyUniqueConfiguredModes(t *testing.T) {
	c := NewClient("localhost", 1883)
	b := &recordingBroker{}
	c.client = b
	c.SetWaterConfig("p1", 21, 0, 0)
	c.refreshWaterModes()
	if len(b.writes) != 1 || b.writes[0] != "R/p1/pump/0/Mode=" {
		t.Fatalf("wrong readback targets: %v", b.writes)
	}
	c.EnableGatewayMode()
	c.refreshWaterModes()
	if len(b.writes) != 1 {
		t.Fatal("inactive MQTT sent readback request")
	}
}

func TestMalformedModeRevokesOnlyMatchingWaterAuthority(t *testing.T) {
	for _, payload := range []string{`{"value":null}`, `{"value":true}`, `{"value":"1"}`, `{"value":-1}`, `{"value":3}`, `{"value":0.5}`, `{"value":[]}`, `{"value":NaN}`, `{"value":1e1000}`, `{}`, `{not-json`, ``} {
		t.Run(payload, func(t *testing.T) {
			c := NewClient("localhost", 1883)
			b := &recordingBroker{}
			c.client = b
			c.SetWaterConfig("p1", 21, 3, 8)
			send(c, "pump/3", "Mode", 0)
			send(c, "pump/8", "Mode", 2)
			for _, topic := range []string{"N/foreign/pump/3/Mode", "N/p1/pump/9/Mode", "N/p1/pump/3/State"} {
				c.onCerboLiveMessage(nil, &fakeMessage{topic: topic, payload: []byte(`{not-json`)})
			}
			c.applyCerboMessage(&fakeMessage{topic: "N/p1/pump/3/Mode", payload: []byte(payload)}, c.pushGenerationNow()+1)
			if !c.CanControlWaterDevice("pump") {
				t.Fatal("unrelated or retired source invalidated selected Mode")
			}
			c.onCerboLiveMessage(nil, &fakeMessage{topic: "N/p1/pump/3/Mode", payload: []byte(payload)})
			if c.CanControlWaterDevice("pump") || c.SetWaterMode("pump", 1) == nil || len(b.writes) != 0 {
				t.Fatal("malformed Mode retained physical command authority")
			}
			if !c.CanControlWaterDevice("valve") {
				t.Fatal("different configured instance lost authority")
			}
			send(c, "pump/3", "Mode", 0)
			if !c.CanControlWaterDevice("pump") {
				t.Fatal("valid fresh Mode did not restore control")
			}
		})
	}
}
