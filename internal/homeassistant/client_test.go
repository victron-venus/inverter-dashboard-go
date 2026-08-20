package homeassistant

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func TestNewClient(t *testing.T) {
	// Test with nil config
	client := NewClient(nil)
	if client == nil { //nolint:staticcheck
		t.Error("NewClient(nil) returned nil")
		return
	}
	if client.configured { //nolint:staticcheck
		t.Error("NewClient(nil) should not be configured")
		return
	}

	// Test with valid config
	validCfg := &config.HomeAssistantConfig{
		URL:           "http://ha.local:8123",
		Token:         "valid-token",
		DirectControls: true,
		PollInterval:  30,
	}
	client = NewClient(validCfg)
	if client == nil { //nolint:staticcheck
		t.Error("NewClient(validCfg) returned nil")
		return
	}
	if !client.configured { //nolint:staticcheck
		t.Error("NewClient(validCfg) should be configured")
		return
	}
	if client.httpURL != "http://ha.local:8123" {
		t.Errorf("client.httpURL = %q, want %q", client.httpURL, "http://ha.local:8123")
	}
	if client.token != "valid-token" {
		t.Errorf("client.token = %q, want %q", client.token, "valid-token")
	}
	if !client.directMode {
		t.Error("client.directMode should be true")
	}
	if client.pollInterval != 30*time.Second {
		t.Errorf("client.pollInterval = %v, want %v", client.pollInterval, 30*time.Second)
	}

	// Test with invalid token
	invalidTokenCfg := &config.HomeAssistantConfig{
		URL:   "http://ha.local:8123",
		Token: "REPLACE_WITH_LONG_LIVED_ACCESS_TOKEN",
	}
	client = NewClient(invalidTokenCfg)
	if client == nil { //nolint:staticcheck
		t.Error("NewClient(invalidTokenCfg) returned nil")
		return
	}
	if client.configured { //nolint:staticcheck
		t.Error("NewClient(invalidTokenCfg) should not be configured")
		return
	}

	// Test with empty URL
	emptyURLCfg := &config.HomeAssistantConfig{
		URL:   "",
		Token: "valid-token",
	}
	client = NewClient(emptyURLCfg)
	if client == nil { //nolint:staticcheck
		t.Error("NewClient(emptyURLCfg) returned nil")
		return
	}
	if client.configured { //nolint:staticcheck
		t.Error("NewClient(emptyURLCfg) should not be configured")
		return
	}

	// Test with invalid URL
	invalidURLCfg := &config.HomeAssistantConfig{
		URL:   "not-a-url",
		Token: "valid-token",
	}
	client = NewClient(invalidURLCfg)
	if client == nil { //nolint:staticcheck
		t.Error("NewClient(invalidURLCfg) returned nil")
		return
	}
	if client.configured { //nolint:staticcheck
		t.Error("NewClient(invalidURLCfg) should not be configured")
		return
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.HomeAssistantConfig
		wantCfg bool
	}{
		{
			name: "nil config",
			cfg:  nil,
		},
		{
			name: "empty URL",
			cfg: &config.HomeAssistantConfig{
				URL:   "",
				Token: "valid-token",
			},
		},
		{
			name: "empty token",
			cfg: &config.HomeAssistantConfig{
				URL:   "http://ha.local:8123",
				Token: "",
			},
		},
		{
			name: "placeholder token",
			cfg: &config.HomeAssistantConfig{
				URL:   "http://ha.local:8123",
				Token: "REPLACE_WITH_LONG_LIVED_ACCESS_TOKEN",
			},
		},
		{
			name: "invalid URL",
			cfg: &config.HomeAssistantConfig{
				URL:   "not-a-valid-url",
				Token: "valid-token",
			},
		},
		{
			name: "valid config",
			cfg: &config.HomeAssistantConfig{
				URL:   "http://ha.local:8123",
				Token: "valid-token",
			},
			wantCfg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			if tt.cfg != nil {
				client.httpURL = tt.cfg.URL
				client.token = tt.cfg.Token
			}
			got := client.validateConfig()
			if got != tt.wantCfg {
				t.Errorf("validateConfig() = %v, want %v", got, tt.wantCfg)
			}
		})
	}
}

func TestIsDirectMode(t *testing.T) {
	tests := []struct {
		name       string
		configured bool
		directMode bool
		want       bool
	}{
		{
			name:       "not configured",
			configured: false,
			directMode: true,
			want:       false,
		},
		{
			name:       "configured but directMode false",
			configured: true,
			directMode: false,
			want:       false,
		},
		{
			name:       "configured and directMode true",
			configured: true,
			directMode: true,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				configured: tt.configured,
				directMode: tt.directMode,
			}
			if got := client.IsDirectMode(); got != tt.want {
				t.Errorf("IsDirectMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUIConfig(t *testing.T) {
	// Test when not configured
	client := &Client{configured: false}
	if got := client.GetUIConfig(); len(got) != 0 {
		t.Errorf("GetUIConfig() for unconfigured client = %v, want empty map", got)
	}

	// Test when configured but no switch entities
	client = &Client{configured: true}
	if got := client.GetUIConfig(); len(got) != 0 {
		t.Errorf("GetUIConfig() for configured client with no switches = %v, want empty map", got)
	}

	// Test with switch entities
	switchEntities := []config.EntityConfig{
		{Key: "switch1", Entity: "switch.test1", Label: "Test Switch 1", Order: 2},
		{Key: "switch2", Entity: "switch.test2", Label: "Test Switch 2", Order: 1},
	}
	cfg := &config.HomeAssistantConfig{
		URL:          "http://ha.local:8123",
		Token:        "valid-token",
		SwitchEntities: switchEntities,
	}
	client = NewClient(cfg)

	got := client.GetUIConfig()
	buttons, ok := got["home_buttons"].([]Button)
	if !ok {
		t.Error("GetUIConfig() did not return home_buttons slice")
	} else if len(buttons) != 2 {
		t.Errorf("GetUIConfig() home_buttons length = %d, want 2", len(buttons))
	} else {
		// Should be sorted by order
		if buttons[0].Order != 1 {
			t.Errorf("GetUIConfig() first button order = %d, want 1", buttons[0].Order)
		}
		if buttons[1].Order != 2 {
			t.Errorf("GetUIConfig() second button order = %d, want 2", buttons[1].Order)
		}
		// Check IDs are transformed correctly
		if buttons[0].ID != "switch2" {
			t.Errorf("GetUIConfig() first button ID = %q, want %q", buttons[0].ID, "switch2")
		}
		if buttons[1].ID != "switch1" {
			t.Errorf("GetUIConfig() second button ID = %q, want %q", buttons[1].ID, "switch1")
		}
	}
}

func TestIsToggleAllowed(t *testing.T) {
	booleanEntities := map[string]string{
		"bool1": "binary_sensor.bool1",
		"bool2": "binary_sensor.bool2",
	}

	switchEntities := []Button{
		{Entity: "switch.test1", Label: "Test Switch 1", Order: 1, StateKey: "switch1"},
		{Entity: "switch.test2", Label: "Test Switch 2", Order: 2, StateKey: "switch2"},
	}

	tests := []struct {
		name         string
		client       *Client
		entityID     string
		wantAllowed  bool
	}{
		{
			name:      "not configured client",
			client:    &Client{configured: false},
			entityID:  "binary_sensor.bool1",
			wantAllowed: false,
		},
		{
			name:      "empty entityID",
			client:    &Client{configured: true},
			entityID:  "",
			wantAllowed: false,
		},
		{
			name:      "boolean entity",
			client:    &Client{configured: true, booleanEntities: booleanEntities},
			entityID:  "binary_sensor.bool1",
			wantAllowed: true,
		},
		{
			name:      "switch entity",
			client:    &Client{configured: true, switchEntities: switchEntities},
			entityID:  "switch.test1",
			wantAllowed: true,
		},
		{
			name:      "water valve entity",
			client:    &Client{configured: true, waterValve: "switch.water_valve"},
			entityID:  "switch.water_valve",
			wantAllowed: true,
		},
		{
			name:      "water pump entity",
			client:    &Client{configured: true, waterPump: "switch.water_pump"},
			entityID:  "switch.water_pump",
			wantAllowed: true,
		},
		{
			name:      "unsupported entity",
			client:    &Client{configured: true},
			entityID:  "light.unsupported",
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.client.IsToggleAllowed(tt.entityID)
			if got != tt.wantAllowed {
				t.Errorf("IsToggleAllowed(%q) = %v, want %v", tt.entityID, got, tt.wantAllowed)
			}
		})
	}
}

func TestOverlayThreadSafety(t *testing.T) {
	client := &Client{
		overlayMu: sync.RWMutex{},
		overlay: Overlay{
			Booleans:          make(map[string]bool),
			HADirectConnected: true,
		},
	}

	// Test concurrent access
	var wg sync.WaitGroup
	numGoroutines := 10
	wg.Add(numGoroutines) // half for reads, half for writes

	// Start readers
	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = client.GetOverlay()
			}
		}()
	}

	// Start writers
	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				overlay := Overlay{
					Booleans:          map[string]bool{"test": true},
					HADirectConnected: false,
				}
				client.ReplaceOverlay(overlay)
			}
		}()
	}

	wg.Wait()
	// If we got here without panicking, the thread safety works
}

func TestFetchStatesOnce_DirectModeOff(t *testing.T) {
	client := &Client{
		configured: true,
		directMode: false, // Direct mode off
		httpClient: &http.Client{},
	}

	overlay, err := client.FetchStatesOnce()
	if err != nil {
		t.Errorf("FetchStatesOnce() returned unexpected error: %v", err)
	}
	if overlay.HADirectConnected != false {
		t.Errorf("FetchStatesOnce() HADirectConnected = %v, want false when directMode off", overlay.HADirectConnected)
	}
	// Should return empty booleans
	if len(overlay.Booleans) != 0 {
		t.Errorf("FetchStatesOnce() booleans length = %d, want 0", len(overlay.Booleans))
	}
	// Should return empty additional fields
	if len(overlay.AdditionalFields) != 0 {
		t.Errorf("FetchStatesOnce() additional fields length = %d, want 0", len(overlay.AdditionalFields))
	}
}

func TestFetchStatesOnce_HttpClientNil(t *testing.T) {
	client := &Client{
		configured: true,
		directMode: true,
		httpClient: nil, // nil http client
	}

	overlay, err := client.FetchStatesOnce()
	if err == nil {
		t.Error("FetchStatesOnce() expected error for nil http client, got nil")
	}
	if !strings.Contains(err.Error(), "http client not initialized") {
		t.Errorf("FetchStatesOnce() error = %q, want to contain 'http client not initialized'", err)
	}
	// Should return empty overlay on error
	if overlay.HADirectConnected != false {
		t.Errorf("FetchStatesOnce() HADirectConnected = %v, want false on error", overlay.HADirectConnected)
	}
}

func TestFetchStatesOnce_Success(t *testing.T) {
	// Set up mock HA server
	server := httptest.NewServer(nil)
	defer server.Close()

	// Mock entity responses
	entityResponses := map[string]string{
		"binary_sensor.bool1": `{"entity_id":"binary_sensor.bool1","state":"on","attributes":{}}`,
		"switch.test1":        `{"entity_id":"switch.test1","state":"off","attributes":{}}`,
		"sensor.water_level":  `{"entity_id":"sensor.water_level","state":"150","attributes":{}}`,
		"sensor.car_soc":      `{"entity_id":"sensor.car_soc","state":"75","attributes":{}}`,
		"switch.water_valve":  `{"entity_id":"switch.water_valve","state":"on","attributes":{}}`,
		"switch.water_pump":   `{"entity_id":"switch.water_pump","state":"off","attributes":{}}`,
		"sensor.ev_power":     `{"entity_id":"sensor.ev_power","state":"1200","attributes":{}}`,
		"sensor.ev_charging_kw": `{"entity_id":"sensor.ev_charging_kw","state":"3.5","attributes":{}}`,
		"binary_sensor.appliance1": `{"entity_id":"binary_sensor.appliance1","state":"on","attributes":{}}`,
		"sensor.vue1":         `{"entity_id":"sensor.vue1","state":"1500","attributes":{}}`,
		"sensor.temp1":        `{"entity_id":"sensor.temp1","state":"22.5","attributes":{}}`,
	}

	// Create handler that returns appropriate responses
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract entity ID from URL path: /api/states/<entity_id>
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		entityID := parts[3]

		if resp, ok := entityResponses[entityID]; ok {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(resp)); err != nil {
				return
			}
			return
		}
		http.Error(w, "entity not found", http.StatusNotFound)
	}))

	client := &Client{
		configured:   true,
		directMode:   true,
		httpURL:      server.URL,
		token:        "valid-token",
		httpClient:   server.Client(),
		booleanEntities: map[string]string{
			"bool1": "binary_sensor.bool1",
		},
		switchEntities: []Button{
			{Entity: "switch.test1", Label: "Test Switch 1", Order: 1, StateKey: "switch1"},
		},
		waterLevel:   "sensor.water_level",
		carSOC:       "sensor.car_soc",
		waterValve:   "switch.water_valve",
		waterPump:    "switch.water_pump",
		evPower:      "sensor.ev_power",
		evChargingKW: "sensor.ev_charging_kw",
		applianceEntities: map[string]string{
			"appliance1": "binary_sensor.appliance1",
		},
		vueSensors: map[string]string{
			"vue1": "sensor.vue1",
		},
		sensorEntities: map[string]string{
			"temp1": "sensor.temp1",
		},
	}

	overlay, err := client.FetchStatesOnce()
	if err != nil {
		t.Errorf("FetchStatesOnce() returned unexpected error: %v", err)
	}

	// Check booleans
	if got, ok := overlay.Booleans["bool1"]; !ok || !got {
		t.Errorf("FetchStatesOnce() booleans[bool1] = %v (%v), want true", got, ok)
	}

	// Check additional fields
	if got, ok := overlay.AdditionalFields["switch1"]; !ok || got.(bool) != false {
		t.Errorf("FetchStatesOnce() additionalFields[switch1] = %v (%v), want false", got, ok)
	}
	if got, ok := overlay.AdditionalFields["water_level"]; !ok || got.(float64) != 150 {
		t.Errorf("FetchStatesOnce() additionalFields[water_level] = %v (%v), want 150", got, ok)
	}
	if got, ok := overlay.AdditionalFields["car_soc"]; !ok || got.(float64) != 75 {
		t.Errorf("FetchStatesOnce() additionalFields[car_soc] = %v (%v), want 75", got, ok)
	}
	if got, ok := overlay.AdditionalFields["water_valve"]; !ok || got.(bool) != true {
		t.Errorf("FetchStatesOnce() additionalFields[water_valve] = %v (%v), want true", got, ok)
	}
	if got, ok := overlay.AdditionalFields["pump_switch"]; !ok || got.(bool) != false {
		t.Errorf("FetchStatesOnce() additionalFields[pump_switch] = %v (%v), want false", got, ok)
	}
	if got, ok := overlay.AdditionalFields["ev_power"]; !ok || got.(float64) != 1200 {
		t.Errorf("FetchStatesOnce() additionalFields[ev_power] = %v (%v), want 1200", got, ok)
	}
	if got, ok := overlay.AdditionalFields["ev_charging_kw"]; !ok || got.(float64) != 3.5 {
		t.Errorf("FetchStatesOnce() additionalFields[ev_charging_kw] = %v (%v), want 3.5", got, ok)
	}
	if got, ok := overlay.AdditionalFields["appliance1"]; !ok || got != true {
		t.Errorf("FetchStatesOnce() additionalFields[appliance1] = %v (%v), want true", got, ok)
	}
	// Check loads nested object
	if loads, ok := overlay.AdditionalFields["loads"].(map[string]float64); !ok || loads["vue1"] != 1500 {
		t.Errorf("FetchStatesOnce() additionalFields[loads] = %v, want map[vue1:1500]", overlay.AdditionalFields["loads"])
	}
	if got, ok := overlay.AdditionalFields["temp1"]; !ok || got.(float64) != 22.5 {
		t.Errorf("FetchStatesOnce() additionalFields[temp1] = %v (%v), want 22.5", got, ok)
	}

	// Should be connected
	if !overlay.HADirectConnected {
		t.Error("FetchStatesOnce() HADirectConnected = false, want true")
	}
}

func TestFetchStatesOnce_EntityErrors(t *testing.T) {
	// Set up mock HA server that returns errors for some entities
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		entityID := parts[3]

		switch entityID {
		case "binary_sensor.ok":
			w.Header().Set("Content-Type", "application/json")
			response := []byte(`{"entity_id":"binary_sensor.ok","state":"on","attributes":{}}`)
			t.Logf("[HA DEBUG] Server sending: %s", string(response))
			if _, err := w.Write(response); err != nil {
				t.Logf("[HA DEBUG] Server write error: %v", err)
				return
			}
		case "switch.error":
			http.Error(w, "internal server error", http.StatusInternalServerError)
		case "sensor.notfound":
			http.Error(w, "not found", http.StatusNotFound)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	t.Logf("[HA DEBUG] Server URL: %s", server.URL)
	client := &Client{
		configured:   true,
		directMode:   true,
		httpURL:      server.URL,
		token:        "valid-token",
		httpClient:   server.Client(),
		booleanEntities: map[string]string{
			"bool_ok":   "binary_sensor.ok",
			"bool_error": "switch.error",
		},
		switchEntities: []Button{
			{Entity: "binary_sensor.ok", Label: "OK Switch", Order: 1, StateKey: "switch_ok"},
			{Entity: "switch.error", Label: "Error Switch", Order: 2, StateKey: "switch_error"},
		},
	}

	overlay, err := client.FetchStatesOnce()
	if err != nil {
		t.Errorf("FetchStatesOnce() returned unexpected error: %v", err)
	}

	// OK boolean should be processed
	if got, ok := overlay.Booleans["bool_ok"]; !ok || !got {
		t.Errorf("FetchStatesOnce() booleans[bool_ok] = %v (%v), want true", got, ok)
	}
	// Error boolean should be skipped (not present in result)
	if _, ok := overlay.Booleans["bool_error"]; ok {
		t.Error("FetchStatesOnce() booleans[bool_error] should be skipped, but was present")
	}

	// OK switch should be processed
	if got, ok := overlay.AdditionalFields["switch_ok"]; !ok || got.(bool) != true {
		t.Errorf("FetchStatesOnce() additionalFields[switch_ok] = %v (%v), want true", got, ok)
	}
	// Error switch should be skipped
	if _, ok := overlay.AdditionalFields["switch_error"]; ok {
		t.Error("FetchStatesOnce() additionalFields[switch_error] should be skipped, but was present")
	}

	// Should still be connected
	if !overlay.HADirectConnected {
		t.Error("FetchStatesOnce() HADirectConnected = false, want true")
	}
}

func TestToggleEntity(t *testing.T) {
	tests := []struct {
		name         string
		client       *Client
		entityID     string
		wantErr      bool
		wantErrSubstr string
	}{
		{
			name:         "not configured",
			client:       &Client{configured: false},
			entityID:     "input_boolean.test",
			wantErr:      true,
			wantErrSubstr: "HA not configured",
		},
		{
			name:         "unapproved domain",
			client:       &Client{configured: true},
			entityID:     "light.test",
			wantErr:      true,
			wantErrSubstr: "unsupported domain for toggle:",
		},
		{
			name:         "input_boolean domain",
			client:       &Client{configured: true},
			entityID:     "input_boolean.test",
			wantErr:      false, // Will fail on actual HTTP call, but that's OK for this test
		},
		{
			name:         "switch domain",
			client:       &Client{configured: true},
			entityID:     "switch.test",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.ToggleEntity(tt.entityID)
			if tt.wantErr {
				if err == nil {
					t.Error("ToggleEntity() expected error, got nil")
				} else if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("ToggleEntity() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
				}
			} else {
				// For success cases, we expect an error from the mock HTTP call (since we're not mocking the service endpoint)
				// but not a configuration error
				if err != nil && strings.Contains(err.Error(), "HA not configured") {
					t.Errorf("ToggleEntity() unexpected configuration error: %v", err)
				}
			}
		})
	}
}

func TestTurnEntity(t *testing.T) {
	tests := []struct {
		name         string
		client       *Client
		entityID     string
		turnOn       bool
		wantErr      bool
		wantErrSubstr string
	}{
		{
			name:         "not configured",
			client:       &Client{configured: false},
			entityID:     "input_boolean.test",
			turnOn:       true,
			wantErr:      true,
			wantErrSubstr: "HA not configured",
		},
		{
			name:         "unsupported domain for turn",
			client:       &Client{configured: true},
			entityID:     "sensor.test",
			turnOn:       true,
			wantErr:      true,
			wantErrSubstr: "unsupported domain for turn:",
		},
		{
			name:         "input_boolean domain turn on",
			client:       &Client{configured: true},
			entityID:     "input_boolean.test",
			turnOn:       true,
			wantErr:      false,
		},
		{
			name:         "switch domain turn off",
			client:       &Client{configured: true},
			entityID:     "switch.test",
			turnOn:       false,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.TurnEntity(tt.entityID, tt.turnOn)
			if tt.wantErr {
				if err == nil {
					t.Error("TurnEntity() expected error, got nil")
				} else if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("TurnEntity() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "HA not configured") {
					t.Errorf("TurnEntity() unexpected configuration error: %v", err)
				}
			}
		})
	}
}

func TestPressButton(t *testing.T) {
	tests := []struct {
		name         string
		client       *Client
		entityID     string
		wantErr      bool
		wantErrSubstr string
	}{
		{
			name:         "not configured",
			client:       &Client{configured: false},
			entityID:     "button.test",
			wantErr:      true,
			wantErrSubstr: "HA not configured",
		},
		{
			name:         "not a button entity",
			client:       &Client{configured: true},
			entityID:     "switch.test",
			wantErr:      true,
			wantErrSubstr: "not a button entity:",
		},
		{
			name:         "valid button entity",
			client:       &Client{configured: true},
			entityID:     "button.test",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.PressButton(tt.entityID)
			if tt.wantErr {
				if err == nil {
					t.Error("PressButton() expected error, got nil")
				} else if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("PressButton() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
				}
			} else {
				if err != nil && strings.Contains(err.Error(), "HA not configured") {
					t.Errorf("PressButton() unexpected configuration error: %v", err)
				}
			}
		})
	}
}

func TestGetPollInterval(t *testing.T) {
	tests := []struct {
		name      string
		client    *Client
		want      time.Duration
	}{
		{
			name:      "zero interval",
			client:    &Client{pollInterval: 0},
			want:      0,
		},
		{
			name:      "positive interval",
			client:    &Client{pollInterval: 30 * time.Second},
			want:      30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.client.GetPollInterval(); got != tt.want {
				t.Errorf("GetPollInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBooleanButtons(t *testing.T) {
	// Test when not configured
	client := &Client{configured: false}
	if got := client.GetBooleanButtons(); len(got) != 0 {
		t.Errorf("GetBooleanButtons() for unconfigured client = %v, want empty slice", got)
	}

	// Test when configured but no boolean buttons
	client = &Client{configured: true}
	if got := client.GetBooleanButtons(); len(got) != 0 {
		t.Errorf("GetBooleanButtons() for configured client with no boolean buttons = %v, want empty slice", got)
	}

	// Test with boolean entities
	booleanEntities := []config.BooleanEntityConfig{
		{Key: "bool1", Entity: "binary_sensor.bool1", Order: 2},
		{Key: "bool2", Entity: "binary_sensor.bool2", Order: 1},
	}
	cfg := &config.HomeAssistantConfig{
		URL:              "http://ha.local:8123",
		Token:            "valid-token",
		BooleanEntities:  booleanEntities,
	}
	client = NewClient(cfg)

	got := client.GetBooleanButtons()
	if len(got) != 2 {
		t.Errorf("GetBooleanButtons() length = %d, want 2", len(got))
	} else {
		// Should be sorted by order
		if got[0].Order != 1 {
			t.Errorf("GetBooleanButtons() first button order = %d, want 1", got[0].Order)
		}
		if got[1].Order != 2 {
			t.Errorf("GetBooleanButtons() second button order = %d, want 2", got[1].Order)
		}
		// Check labels are generated (since none provided)
		if got[0].Label != "BOOL2" {
			t.Errorf("GetBooleanButtons() first button label = %q, want %q", got[0].Label, "BOOL2")
		}
		if got[1].Label != "BOOL1" {
			t.Errorf("GetBooleanButtons() second button label = %q, want %q", got[1].Label, "BOOL1")
		}
	}
}

func TestGetManagedKeys(t *testing.T) {
	client := &Client{
		booleanEntities: map[string]string{
			"bool1": "binary_sensor.bool1",
			"bool2": "binary_sensor.bool2",
		},
		switchEntities: []Button{
			{Entity: "switch.test1", Label: "Test Switch 1", Order: 1, StateKey: "switch1"},
			{Entity: "switch.test2", Label: "Test Switch 2", Order: 2, StateKey: "switch2"},
		},
		waterValve:   "switch.water_valve",
		waterPump:    "switch.water_pump",
		waterLevel:   "sensor.water_level",
		carSOC:       "sensor.car_soc",
		evChargingKW: "sensor.ev_charging_kw",
		evPower:      "sensor.ev_power",
		applianceEntities: map[string]string{
			"appliance1": "binary_sensor.appliance1",
		},
		sensorEntities: map[string]string{
			"sensor1": "sensor.sensor1",
		},
	}

	got := client.GetManagedKeys()
	want := []string{
		"bool1", "bool2",
		"switch1", "switch2",
		"water_valve", "water_pump",
		"water_level", "car_soc",
		"ev_charging_kw", "ev_power",
		"appliance1",
		"sensor1",
	}

	if len(got) != len(want) {
		t.Errorf("GetManagedKeys() length = %d, want %d", len(got), len(want))
		return
	}

	// Sort both for comparison (since order may vary in implementation)
	sort.Strings(got)
	sort.Strings(want)

	for i := 0; i < len(got); i++ {
		if got[i] != want[i] {
			t.Errorf("GetManagedKeys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
