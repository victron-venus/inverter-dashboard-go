package state

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOverrideStrictWireAndStop(t *testing.T) {
	for _, value := range []string{"null", "0", "-2147483648", "2147483647", "true", "\"1\"", "1.00000000000000001", "2147483648", "1.0"} {
		data, err := DecodeControllerState([]byte(`{"setpoint_override":{"value":` + value + `,"last_error":null,"request_id":null}}`))
		if err != nil {
			t.Fatal(err)
		}
		got := ParseOverrideStatus(data["setpoint_override"])
		want := value == "null" || value == "0" || value == "-2147483648" || value == "2147483647"
		if (got != nil) != want {
			t.Fatalf("value %s accepted=%v", value, got != nil)
		}
	}
	if ParseOverrideStatus(map[string]interface{}{"value": nil}) != nil {
		t.Fatal("incomplete status authorized")
	}
}

func TestControllerReadinessAndExactOverrideAcknowledgement(t *testing.T) {
	yes, dry := true, true
	now := time.Now()
	at := float64(now.UnixMilli()) / 1000
	id := "request"
	wrong := int32(50)
	st := &State{InverterAvailable: &yes, DryRun: &dry, SetpointOverride: &SetpointOverrideStatus{RequestID: &id}, SetpointOverrideObservedAt: &at}
	body := map[string]interface{}{"value": nil, "request_id": id}
	if !ControllerCommandReady(st, "setpoint_override", body, now) {
		t.Fatal("override incorrectly gated by dry run")
	}
	if ok, err := OverrideAcknowledged(st, body); !ok || err != nil {
		t.Fatal("stopped override not acknowledged")
	}
	st.SetpointOverride.Value = &wrong
	if ok, _ := OverrideAcknowledged(st, body); ok {
		t.Fatal("same ID wrong value acknowledged")
	}
	errText := "rejected"
	st.SetpointOverride.LastError = &errText
	if _, err := OverrideAcknowledged(st, body); err == nil {
		t.Fatal("controller error ignored")
	}
	for _, age := range []time.Duration{-time.Second, 31 * time.Second} {
		v := float64(now.Add(-age).UnixMilli()) / 1000
		st.SetpointOverrideObservedAt = &v
		if ControllerCommandReady(st, "setpoint_override", body, now) {
			t.Fatal("stale/future observation authorized")
		}
	}
}

func TestTariffEnvelopeAndRevisionGuard(t *testing.T) {
	revision := strings.Repeat("a", 64)
	body := map[string]interface{}{"request_id": "req", "revision": revision, "plan": nil}
	if ValidateTariff(body) != nil {
		t.Fatal("valid clear rejected")
	}
	for _, bad := range []any{true, []any{}, "plan"} {
		body["plan"] = bad
		if ValidateTariff(body) == nil {
			t.Fatal("invalid plan accepted")
		}
	}
	body["plan"] = map[string]interface{}{"name": strings.Repeat("x", 100000)}
	if ValidateTariff(body) == nil {
		t.Fatal("unbounded command accepted")
	}
	body["plan"] = nil
	yes := true
	at := float64(time.Now().UnixMilli()) / 1000
	st := &State{InverterAvailable: &yes, ElectricityTariffObservedAt: &at, UIConfig: map[string]interface{}{"electricity_tariff_status": map[string]interface{}{"writable": true, "revision": revision}}}
	if !ControllerCommandReady(st, "electricity_tariff", body, time.Now()) {
		t.Fatal("current revision rejected")
	}
	body["revision"] = strings.Repeat("b", 64)
	if ControllerCommandReady(st, "electricity_tariff", body, time.Now()) {
		t.Fatal("stale revision accepted")
	}
	if ValidateOverride(map[string]interface{}{"value": json.Number("1.00000000000000001"), "request_id": "id"}) == nil {
		t.Fatal("rounded wire fraction accepted")
	}
}

func TestTariffBoundUsesExactWireEncoding(t *testing.T) {
	body := map[string]interface{}{"request_id": "id", "revision": strings.Repeat("a", 64), "plan": map[string]interface{}{"name": strings.Repeat("<>&é", 15000)}}
	encoded, err := EncodeControllerCommand(body)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateTariff(body) != nil || len(encoded) > 100000 || strings.Contains(string(encoded), `\u003c`) {
		t.Fatal("wire encoding expanded accepted UTF8 envelope")
	}
	var decoded map[string]interface{}
	if json.Unmarshal(encoded, &decoded) != nil || decoded["plan"].(map[string]interface{})["name"] != body["plan"].(map[string]interface{})["name"] {
		t.Fatal("wire encoding changed tariff data")
	}
}
