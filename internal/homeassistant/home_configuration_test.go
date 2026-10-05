package homeassistant

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func TestConfiguredHomeOrderAndUnknownReadings(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"unavailable"}`))
	}))
	defer server.Close()
	disabled := false
	client := NewClient(&config.HomeAssistantConfig{
		URL: server.URL, Token: "test-token", DirectControls: true,
		SwitchEntities: []config.EntityConfig{
			{Key: "home_last", Entity: "switch.last", Order: 2},
			{Key: "home_first", Entity: "light.example", Label: "Laundry guard", Order: 1},
			{Key: "home_peer", Entity: "switch.peer", Order: 1},
			{Key: "home_disabled", Entity: "switch.disabled", Enabled: &disabled},
			{Key: "ess_mode_controls_available", Entity: "switch.invalid"},
		},
	})
	buttons := client.GetUIConfig()["home_buttons"].([]Button)
	if len(buttons) != 3 || buttons[0].StateKey != "home_first" || buttons[1].StateKey != "home_peer" || buttons[0].Label != "Laundry guard" {
		t.Fatalf("Home button configuration = %#v", buttons)
	}
	if client.IsToggleAllowed("switch.disabled") || client.IsToggleAllowed("switch.invalid") {
		t.Fatal("unavailable Home target authorized")
	}
	overlay, err := client.FetchStatesOnce()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requested, []string{"/api/states/light.example", "/api/states/switch.peer", "/api/states/switch.last"}) {
		t.Fatalf("requests = %v", requested)
	}
	for _, key := range []string{"home_first", "home_peer", "home_last"} {
		if value, exists := overlay.AdditionalFields[key]; !exists || value != nil {
			t.Fatalf("unavailable Home state %s = %v (exists %t)", key, value, exists)
		}
	}
}

func TestEmptyHomeConfigurationIsExplicit(t *testing.T) {
	client := NewClient(nil)
	buttons, ok := client.GetUIConfig()["home_buttons"].([]Button)
	if !ok || len(buttons) != 0 {
		t.Fatalf("empty Home configuration = %#v", client.GetUIConfig())
	}
}
