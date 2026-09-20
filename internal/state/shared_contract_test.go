package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedDashboardContract(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "dashboard")
	read := func(name string, into interface{}) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, into); err != nil {
			t.Fatal(err)
		}
	}
	var lock struct {
		SHA256 map[string]string `json:"sha256"`
	}
	read("contract-lock.json", &lock)
	for name, expected := range lock.SHA256 {
		data, err := os.ReadFile(filepath.Join(root, "v1", name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != expected {
			t.Fatalf("modified fixture: %s", name)
		}
	}
	var fixtures struct {
		Keys []struct {
			Input    string  `json:"input"`
			Expected *string `json:"expected"`
		} `json:"key_cases"`
		Booleans []struct {
			Input    interface{} `json:"input"`
			Expected *bool       `json:"expected"`
		} `json:"boolean_cases"`
	}
	read("v1/fixtures.json", &fixtures)
	for _, fixture := range fixtures.Keys {
		if IsControlFlag(fixture.Input) != (fixture.Expected != nil) {
			t.Errorf("key contract: %q", fixture.Input)
		}
	}
	for _, fixture := range fixtures.Booleans {
		actual, known := ControlBool(fixture.Input)
		if known != (fixture.Expected != nil) || (known && actual != *fixture.Expected) {
			t.Errorf("boolean contract: %#v", fixture.Input)
		}
	}
}
