package mqtt

import (
	"encoding/json"
	"fmt"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// fakeMessage implements mqtt.Message for tests.
type fakeMessage struct {
	topic   string
	payload []byte
}

func (f *fakeMessage) Duplicate() bool { return false }
func (f *fakeMessage) Qos() byte       { return 0 }
func (f *fakeMessage) Retained() bool  { return false }
func (f *fakeMessage) MessageID() uint16 {
	return 1
}
func (f *fakeMessage) Topic() string   { return f.topic }
func (f *fakeMessage) Payload() []byte { return f.payload }
func (f *fakeMessage) Ack()            {}
func (f *fakeMessage) Error() error    { return nil }

func waterMsg(topic string, value interface{}) *fakeMessage {
	payload, _ := json.Marshal(map[string]interface{}{"value": value})
	return &fakeMessage{topic: topic, payload: payload}
}

func TestOnWaterMessageMapsTopicsToState(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)

	c.onWaterMessage(nil, waterMsg("N/p1/tank/21/Level", 66.5))
	c.onWaterMessage(nil, waterMsg("N/p1/pump/2/State", 1))
	c.onWaterMessage(nil, waterMsg("N/p1/pump/1/State", 0))

	st := c.GetState()
	if st.WaterLevel != 66.5 {
		t.Errorf("WaterLevel = %v, want 66.5", st.WaterLevel)
	}
	if !st.WaterValve {
		t.Error("WaterValve = false, want true")
	}
	if st.PumpSwitch {
		t.Error("PumpSwitch = true, want false")
	}
}

func TestOnWaterMessageIgnoresForeignTopics(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)

	c.onWaterMessage(nil, waterMsg("N/other/tank/21/Level", 10))
	c.onWaterMessage(nil, waterMsg("N/p1/tank/9/Level", 10))
	c.onWaterMessage(nil, waterMsg("N/p1/pump/3/State", 1))
	c.onWaterMessage(nil, waterMsg(fmt.Sprintf("N/p1/tank/%d/Capacity", c.tankInstance), 200))

	st := c.GetState()
	if st.WaterLevel != 0 || st.WaterValve || st.PumpSwitch {
		t.Errorf("unexpected state from foreign topics: %+v", st)
	}
}

func TestOnWaterMessageBadPayload(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)

	c.onWaterMessage(nil, &fakeMessage{topic: "N/p1/tank/21/Level", payload: []byte("junk")})
	c.onWaterMessage(nil, waterMsg("N/p1/tank/21/Level", "not-a-number"))

	st := c.GetState()
	if st.WaterLevel != 0 {
		t.Errorf("WaterLevel = %v, want 0", st.WaterLevel)
	}
}

// compile-time interface check
var _ mqtt.Message = (*fakeMessage)(nil)

func TestOnPvInverterMessageCollectsDevices(t *testing.T) {
	c := NewClient("localhost", 1883)

	c.onPvInverterMessage(nil, waterMsg("N/p1/pvinverter/9895/Ac/Power", 181))
	c.onPvInverterMessage(nil, waterMsg("N/p1/pvinverter/369/Ac/Power", 163))
	c.onPvInverterMessage(nil, waterMsg("N/p1/pvinverter/369/Ac/L1/Voltage", 126.0))
	c.onPvInverterMessage(nil, waterMsg("N/p1/pvinverter/369/Ac/L1/Current", 1.29))
	c.onPvInverterMessage(nil, waterMsg("N/p1/pvinverter/369/ProductName", "Tasmota PV 1"))
	// Foreign service and bad payload are ignored.
	c.onPvInverterMessage(nil, waterMsg("N/p1/solarcharger/290/Yield/Power", 100))
	c.onPvInverterMessage(nil, &fakeMessage{topic: "N/p1/pvinverter/369/Ac/Power", payload: []byte("junk")})

	st := c.GetState()
	if len(st.PvInverters) != 2 {
		t.Fatalf("PvInverters len = %d, want 2", len(st.PvInverters))
	}
	first := st.PvInverters[0]
	if first.Power != 163 || first.PVVoltage != 126 || first.Current != 1.29 || first.Name != "Tasmota PV 1" {
		t.Errorf("first inverter = %+v, want V=126 I=1.29 P=163 named", first)
	}
	if st.PvInverters[0].Power > st.PvInverters[1].Power {
		t.Error("inverters not sorted by instance")
	}
}
