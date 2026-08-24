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
	c.onWaterMessage(nil, waterMsg("N/p1/pump/startstop2/State", 1))
	c.onWaterMessage(nil, waterMsg("N/p1/pump/startstop1/State", 0))

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
	c.onWaterMessage(nil, waterMsg("N/p1/pump/startstop3/State", 1))
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
