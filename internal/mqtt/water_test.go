package mqtt

import "testing"

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
