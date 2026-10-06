package homeassistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func TestHomeAssistantURLValidation(t *testing.T) {
	for _, endpoint := range []string{
		"http://homeassistant.local:8123", "http://192.168.1.20:8123/",
		"http://127.0.0.1:8123", "http://[::1]:8123", "https://ha.example.com/homeassistant/", "HTTPS://ha.example.com",
	} {
		t.Run(endpoint, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{URL: endpoint, Token: "token", DirectControls: true})
			if !c.IsDirectMode() {
				t.Fatal("operator-configured HA endpoint was rejected")
			}
		})
	}
	for _, endpoint := range []string{
		"homeassistant.local:8123", "/relative", "//homeassistant.local", "ftp://ha.local",
		"http://", "http://user:password@ha.local", "http://ha.local?target=other",
		"http://ha.local?", "http://ha.local#fragment", "http://ha.local#",
		"http://ha.local:0", "http://ha.local:65536", "http://ha.local:", "http://ha.local:abc",
	} {
		t.Run(endpoint, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{URL: endpoint, Token: "token", DirectControls: true})
			if c.IsDirectMode() {
				t.Fatal("invalid HA endpoint was accepted")
			}
			if _, err := c.newAPIRequest(context.Background(), http.MethodGet, nil, "states", "sensor.test"); err == nil {
				t.Fatal("invalid endpoint was used to build an authenticated request")
			}
		})
	}
}

func TestSettingsCannotChangeHomeAssistantDestination(t *testing.T) {
	const trustedURL = "http://ha.local:8123/homeassistant"
	for _, endpoint := range []string{
		"http://attacker.invalid:8123/homeassistant", "https://ha.local:8123/homeassistant",
		"http://ha.local:9000/homeassistant", "http://ha.local:8123/another-path",
		"http://ha.local.attacker.invalid:8123/homeassistant", "http://ha.local:8123@attacker.invalid",
		"http://ha.local:8123/homeassistant?target=other", "http://ha.local:8123/homeassistant#fragment",
		"http://127.0.0.1:9000", "http://169.254.169.254/latest/meta-data",
	} {
		t.Run(endpoint, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{URL: trustedURL, Token: "original", DirectControls: true})
			if err := c.OverrideCredentials(endpoint, "replacement"); err == nil {
				t.Fatal("untrusted destination override was accepted")
			}
			if c.httpURL != trustedURL || c.token != "original" || !c.IsDirectMode() {
				t.Fatal("rejected override changed the trusted configuration")
			}
		})
	}
	c := NewClient(&config.HomeAssistantConfig{URL: trustedURL + "/", Token: "original", DirectControls: true})
	for _, endpoint := range []string{"", trustedURL, trustedURL + "/"} {
		if err := c.OverrideCredentials(endpoint, "replacement"); err != nil || c.token != "replacement" || c.httpURL != trustedURL {
			t.Fatalf("matching endpoint/token override failed: %v", err)
		}
	}
	// An operator may configure the trusted URL while keeping the token in
	// persisted settings; accepting that token must not accept a new URL too.
	c = NewClient(&config.HomeAssistantConfig{URL: trustedURL + "/", DirectControls: true})
	if err := c.OverrideCredentials(trustedURL, "replacement"); err != nil || !c.IsDirectMode() {
		t.Fatalf("settings token did not enable the operator-configured endpoint: %v", err)
	}
	for _, cfg := range []*config.HomeAssistantConfig{nil, {DirectControls: true}} {
		c = NewClient(cfg)
		if err := c.OverrideCredentials(trustedURL, "replacement"); err == nil || c.IsDirectMode() {
			t.Fatal("settings established an endpoint without operator configuration")
		}
	}
}

func TestHomeAssistantServicePathsStayUnderAPI(t *testing.T) {
	var calls atomic.Int32
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/homeassistant/api/services/switch/turn_on" || r.URL.RawQuery != "" {
			t.Errorf("unexpected service request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer configured-token" {
			t.Error("missing configured bearer token")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["entity_id"] != "switch.lamp" {
			t.Errorf("unexpected service body: %v (%v)", body, err)
		}
	}))
	defer ha.Close()
	c := NewClient(&config.HomeAssistantConfig{URL: ha.URL + "/homeassistant/", Token: "configured-token", DirectControls: true})
	c.entityObserved = map[string]time.Time{"switch.lamp": time.Now()}
	if err := c.callService("switch", "turn_on", "switch.lamp"); err != nil {
		t.Fatalf("service call failed: %v", err)
	}
	if err := c.callServiceDomain("switch/turn_on", "switch.lamp"); err != nil {
		t.Fatalf("domain service call failed: %v", err)
	}
	for _, service := range []string{"", "switch", "../config", "switch/../config", "switch/turn_on/../../config", "switch/..", "//attacker.invalid", "switch/turn_on\\config"} {
		if err := c.callServiceDomain(service, "switch.lamp"); err == nil {
			t.Errorf("invalid service %q accepted", service)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("got %d requests, want 2", calls.Load())
	}
}

func TestHomeAssistantRequestsStayOnTrustedEndpoint(t *testing.T) {
	var calls atomic.Int32
	ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/homeassistant/api/states/sensor.room" || r.URL.RawQuery != "" {
			t.Errorf("unexpected API URL: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer configured-token" {
			t.Error("missing configured bearer token")
		}
		_ = json.NewEncoder(w).Encode(EntityState{EntityID: "sensor.room", State: "21"})
	}))
	defer ha.Close()
	c := NewClient(&config.HomeAssistantConfig{URL: ha.URL + "/homeassistant/", Token: "configured-token", DirectControls: true})
	if err := c.OverrideCredentials("http://attacker.invalid/", "attacker-token"); err == nil {
		t.Fatal("untrusted settings accepted")
	}
	if state, err := c.getEntityState(context.Background(), "sensor.room"); err != nil || state != "21" {
		t.Fatalf("trusted HA request failed: %q, %v", state, err)
	}
	if doc, err := c.getEntityDoc(context.Background(), "sensor.room"); err != nil || doc.State != "21" {
		t.Fatalf("trusted HA document request failed: %v, %v", doc, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("got %d requests, want 2", calls.Load())
	}
	for _, entity := range []string{"../config", "sensor.room/../../config", "//attacker.invalid", ".", "..", "sensor.room\\config"} {
		if _, err := c.getEntityDoc(context.Background(), entity); err == nil {
			t.Errorf("path traversal entity %q accepted", entity)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("invalid entity performed a network request")
	}
	req, err := c.newAPIRequest(context.Background(), http.MethodGet, nil, "states", "sensor.room?x=1#fragment")
	if err != nil || req.URL.RawQuery != "" || req.URL.Fragment != "" || !strings.HasSuffix(req.URL.Path, "sensor.room?x=1#fragment") {
		t.Fatalf("entity delimiters were interpreted as URL routing: %v, %v", req, err)
	}
}

func TestHomeAssistantDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, sameOrigin := range []bool{false, true} {
			var leakedRequests atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				leakedRequests.Add(1)
				_ = json.NewEncoder(w).Encode(EntityState{State: "on"})
			}))
			ha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirected" {
					leakedRequests.Add(1)
					return
				}
				target := destination.URL
				if sameOrigin {
					target = "/redirected"
				}
				http.Redirect(w, r, target, status)
			}))
			c := NewClient(&config.HomeAssistantConfig{URL: ha.URL, Token: "private-token", DirectControls: true})
			if _, err := c.getEntityDoc(context.Background(), "sensor.room"); err == nil {
				t.Errorf("GET redirect %d accepted (same origin: %v)", status, sameOrigin)
			}
			if err := c.callService("switch", "turn_on", "switch.lamp"); err == nil {
				t.Errorf("service redirect %d accepted (same origin: %v)", status, sameOrigin)
			}
			if leakedRequests.Load() != 0 {
				t.Errorf("redirect %d sent %d unauthorized requests", status, leakedRequests.Load())
			}
			ha.Close()
			destination.Close()
		}
	}
}
