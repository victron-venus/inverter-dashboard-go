package state

import (
	"bytes"
	"encoding/json"
	"math/big"
	"testing"
)

// FuzzControllerCommand preserves exact wire integers and the bounded tariff
// envelope. It calls production validators/encoders, never a command transport.
func FuzzControllerCommand(f *testing.F) {
	for _, seed := range []string{
		`{"value":null,"request_id":"fuzz-1"}`,
		`{"value":2147483647,"request_id":"fuzz-2"}`,
		`{"value":-2147483649,"request_id":"fuzz-3"}`,
		`{"value":1.0000000000000001,"request_id":"fuzz-4"}`,
		`{"request_id":"fuzz-5","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","plan":{"label":"<雪>\n&"}}`,
		`{"setpoint_override":{"value":2147483647,"request_id":"fuzz-6","last_error":null}}`,
		`null`, `[]`, `{}`, ``,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 128*1024 {
			t.Skip()
		}
		checkFuzzControllerStatus(t, raw)
		checkFuzzCommand(t, raw)
	})
}

func checkFuzzControllerStatus(t *testing.T, raw []byte) {
	t.Helper()
	// Exercise telemetry's exact-number override path; malformed input may
	// return an error but must never panic.
	decoded, err := DecodeControllerState(raw)
	if err != nil {
		return
	}
	status := ParseOverrideStatus(decoded["setpoint_override"])
	if status == nil {
		return
	}
	if _, err := json.Marshal(status); err != nil {
		t.Fatalf("accepted status cannot be serialized: %v", err)
	}
}

func checkFuzzCommand(t *testing.T, raw []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload any
	if !json.Valid(raw) || decoder.Decode(&payload) != nil {
		return
	}
	overrideOK := ValidateOverride(payload) == nil
	tariffOK := ValidateTariff(payload) == nil
	if !overrideOK && !tariffOK {
		return
	}
	body, object := payload.(map[string]any)
	if !object {
		t.Fatal("non-object command was accepted")
	}
	if overrideOK {
		checkFuzzOverride(t, body)
	}
	wire, err := EncodeControllerCommand(payload)
	if err != nil || !json.Valid(wire) {
		t.Fatalf("accepted command has invalid wire encoding: %v", err)
	}
	if tariffOK && (len(body) != 3 || len(wire) > 100000) {
		t.Fatal("tariff exceeded its wire envelope budget")
	}
	checkFuzzCommandRoundTrip(t, wire, overrideOK, tariffOK)
}

func checkFuzzOverride(t *testing.T, body map[string]any) {
	t.Helper()
	if len(body) != 2 {
		t.Fatal("override accepted an unexpected envelope")
	}
	if body["value"] == nil {
		return
	}
	number, numberOK := body["value"].(json.Number)
	integer, integerOK := new(big.Int).SetString(string(number), 10)
	if !numberOK || !integerOK || !integer.IsInt64() || integer.Int64() < -2147483648 || integer.Int64() > 2147483647 {
		t.Fatal("override accepted a lossy or out-of-range wire integer")
	}
}

func checkFuzzCommandRoundTrip(t *testing.T, wire []byte, overrideOK, tariffOK bool) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.UseNumber()
	var roundTrip any
	if err := decoder.Decode(&roundTrip); err != nil {
		t.Fatal(err)
	}
	if overrideOK != (ValidateOverride(roundTrip) == nil) || tariffOK != (ValidateTariff(roundTrip) == nil) {
		t.Fatal("wire encoding changed command validity")
	}
}
