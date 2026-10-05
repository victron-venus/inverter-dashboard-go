package mqtt

import (
	"encoding/json"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

type retainedESSMessage struct{ *fakeMessage }

func (*retainedESSMessage) Retained() bool { return true }

type essRecordingBroker struct{ recordingBroker }

func (b *essRecordingBroker) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	if bytes, ok := payload.([]byte); ok {
		payload = string(bytes)
	}
	return b.recordingBroker.Publish(topic, qos, retained, payload)
}

const essSelectionStatus = `{"ess_mode":{"selection_supported":true,"selected":"external_control","vebus_mode":3,"request_id":"previous","error":"","mode_name":"External control"},"dry_run":false,"ess_mode_observed_at":99999999999,"ess_mode_controls_available":true}`

func TestESSSelectionRequiresLiveSupportedTelemetry(t *testing.T) {
	for _, reason := range []string{"live", "retained", "stale", "future", "dry", "unknown", "unsupported", "cleared"} {
		t.Run(reason, func(t *testing.T) {
			c := NewClient("localhost", 1883)
			broker := &essRecordingBroker{}
			c.client = broker
			controllerMessage(c, essSelectionStatus)
			switch reason {
			case "retained":
				c.onStateMessage(nil, &retainedESSMessage{&fakeMessage{topic: "inverter/state", payload: []byte(essSelectionStatus)}})
			case "stale":
				at := float64(time.Now().Add(-31*time.Second).UnixMilli()) / 1000
				c.state.ESSModeObservedAt = &at
			case "future":
				at := float64(time.Now().Add(time.Minute).UnixMilli()) / 1000
				c.state.ESSModeObservedAt = &at
			case "dry":
				v := true
				c.state.DryRun = &v
			case "unknown":
				c.state.DryRun = nil
			case "unsupported":
				c.state.ESSMode.SelectionSupported = false
			case "cleared":
				c.invalidateCerbo()
			}
			body := map[string]interface{}{"mode": "off", "request_id": "new-request"}
			err := c.PublishCommand("set_ess_mode", body)
			if (err == nil) != (reason == "live") {
				t.Fatalf("selection %s error=%v", reason, err)
			}
			if reason != "live" && len(broker.writes) != 0 {
				t.Fatal("rejected ESS command reached broker")
			}
			if reason == "live" {
				if got := c.GetState().ESSMode; got.Selected != "external_control" || got.RequestID != "previous" {
					t.Fatal("optimistic ESS acknowledgement")
				}
			}
			if reason == "retained" && c.GetState().ESSModeObservedAt != nil {
				t.Fatal("retained/forged timestamp authorized write")
			}
		})
	}
}

func TestESSControllerSelectionSurvivesNativeAndSlimUpdates(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.client = &recordingBroker{}
	c.SetWaterConfig("p1", 21, 1, 2)
	controllerMessage(c, essSelectionStatus)
	observed := *c.GetState().ESSModeObservedAt
	send(c, "settings/0", "Settings/CGwacs/Hub4Mode", 3)
	st := c.GetState()
	if !st.ESSMode.SelectionSupported || st.ESSMode.Selected != "external_control" || st.ESSMode.RequestID != "previous" {
		t.Fatal("native display erased selection contract")
	}
	controllerMessage(c, `{"dry_run":false,"uptime":123}`)
	if got := c.GetState(); got.ESSModeObservedAt == nil || *got.ESSModeObservedAt != observed || got.ESSMode.RequestID != "previous" {
		t.Fatal("unrelated tick changed ESS observation")
	}
	controllerMessage(c, "null")
	if got := c.GetState(); got.ESSModeObservedAt != nil || got.ESSMode.SelectionSupported {
		t.Fatal("cleared controller still authorizes menu")
	}
}

func TestESSControllerUnsupportedWireUpdateRevokesNativeOverlaySelection(t *testing.T) {
	c := NewClient("localhost", 1883)
	broker := &essRecordingBroker{}
	c.client = broker
	c.SetWaterConfig("p1", 21, 1, 2)
	controllerMessage(c, essSelectionStatus)
	send(c, "settings/0", "Settings/CGwacs/Hub4Mode", 3)
	if !c.CanSelectESSMode() {
		t.Fatal("supported controller with native overlay must allow selection")
	}
	controllerMessage(c, `{"ess_mode":{"selection_supported":false},"dry_run":false}`)
	if c.CanSelectESSMode() || c.GetState().ESSMode.SelectionSupported {
		t.Fatal("unsupported wire update retained stale selection capability")
	}
	if err := c.PublishCommand("set_ess_mode", map[string]interface{}{"mode": "off", "request_id": "revoked"}); err == nil {
		t.Fatal("unsupported controller selection was accepted")
	}
	if len(broker.writes) != 0 {
		t.Fatal("revoked selection reached broker")
	}
}

func TestESSSelectionEnvelopeValidation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"mode":"wrong","request_id":"id"}`, `{"mode":"off","request_id":"../other"}`, `{"mode":"off","request_id":"id","topic":"evil"}`} {
		var body map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			t.Fatal(err)
		}
		if err := state.ValidateESSSelection(body); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, mode := range []string{"off", "on", "optimized_with_battery_life", "optimized_without_battery_life", "keep_batteries_charged", "external_control"} {
		if err := state.ValidateESSSelection(map[string]interface{}{"mode": mode, "request_id": "uuid-1"}); err != nil {
			t.Fatal(err)
		}
	}
}
