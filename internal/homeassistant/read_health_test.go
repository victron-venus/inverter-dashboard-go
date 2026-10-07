package homeassistant

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func TestFailedRefreshDisconnectsStoredHealthAndRecovers(t *testing.T) {
	for _, failure := range []string{"transport", "status", "invalid-json"} {
		t.Run(failure, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{
				URL: "http://ha.test", Token: "fixture", DirectControls: true,
				SwitchEntities:   []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
				FilteredEntities: &config.FilteredEntityConfig{Numbers: []string{"number.limit"}},
			})
			failed := false
			c.httpClient = &http.Client{Transport: ownershipTransport(func(r *http.Request) (*http.Response, error) {
				status, body := http.StatusOK, `{"state":"1","attributes":{"min":0,"max":5}}`
				if failed {
					switch failure {
					case "transport":
						return nil, errors.New("fixture connection failed")
					case "status":
						status = http.StatusServiceUnavailable
					case "invalid-json":
						body = "{"
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			good, err := c.FetchStatesOnce()
			if err != nil || !good.HADirectConnected {
				t.Fatalf("initial observation: %#v, %v", good, err)
			}
			c.ReplaceOverlay(good)
			failed = true
			bad, err := c.FetchStatesOnce()
			if err != nil || bad.HADirectConnected || c.GetOverlay().HADirectConnected {
				t.Errorf("failed refresh retained connected health: result=%#v stored=%#v err=%v", bad, c.GetOverlay(), err)
			}
			if !reflect.DeepEqual(c.GetOverlay().AdditionalFields, good.AdditionalFields) || !reflect.DeepEqual(c.GetOverlay().Booleans, good.Booleans) {
				t.Fatal("failed refresh replaced last committed maps")
			}
			if c.ControlsAvailable() || c.ObservedAt() != nil || len(c.entityObserved) != 0 || len(c.numberRanges) != 0 {
				t.Fatal("failed observation retained freshness or control bounds")
			}
			if err := c.PerformAction("number_set", "number.limit", map[string]interface{}{"value": 1.0}); err == nil {
				t.Fatal("stale number observation permitted a service request")
			}
			failed = false
			recovered, err := c.FetchStatesOnce()
			if err != nil || !recovered.HADirectConnected {
				t.Fatalf("recovery: %#v, %v", recovered, err)
			}
			c.ReplaceOverlay(recovered)
			if !c.GetOverlay().HADirectConnected || !c.ControlsAvailable() || c.ObservedAt() == nil {
				t.Fatal("successful refresh did not restore health and freshness")
			}
		})
	}
}

func TestDecodedPartialRefreshRemainsConnected(t *testing.T) {
	for _, state := range []string{"off", "0", "unknown", "unavailable"} {
		t.Run(state, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{
				URL: "http://ha.test", Token: "fixture", DirectControls: true,
				SwitchEntities: []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
				SensorEntities: map[string]string{"room_temperature": "sensor.room"},
			})
			c.httpClient = &http.Client{Transport: ownershipTransport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "sensor.room") {
					return nil, errors.New("fixture sensor failed")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"state":"` + state + `"}`))}, nil
			})}
			o, err := c.FetchStatesOnce()
			if err != nil || !o.HADirectConnected || !c.ControlsAvailable() {
				t.Fatalf("decoded %q response was treated as transport failure: %#v, %v", state, o, err)
			}
			want := interface{}(false)
			if state == "unknown" || state == "unavailable" {
				want = nil
			}
			if o.AdditionalFields["home_lamp"] != want {
				t.Fatalf("decoded state semantics changed: %#v", o.AdditionalFields)
			}
		})
	}
}

func TestFailedRefreshPreservesOverlayCommittedDuringRead(t *testing.T) {
	c := NewClient(&config.HomeAssistantConfig{
		URL: "http://ha.test", Token: "fixture", DirectControls: true,
		SwitchEntities: []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
	})
	started, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c.httpClient = &http.Client{Transport: ownershipTransport(func(_ *http.Request) (*http.Response, error) {
		close(started)
		<-finish
		return nil, errors.New("fixture connection failed")
	})}
	c.ReplaceOverlay(Overlay{HADirectConnected: true, AdditionalFields: map[string]interface{}{"home_lamp": false}})
	go func() {
		defer close(done)
		_, _ = c.FetchStatesOnce()
	}()
	<-started
	c.ReplaceOverlay(Overlay{HADirectConnected: true, AdditionalFields: map[string]interface{}{"home_lamp": true}})
	close(finish)
	<-done
	if o := c.GetOverlay(); o.HADirectConnected || o.AdditionalFields["home_lamp"] != true {
		t.Fatalf("failure restored a pre-read snapshot or retained health: %#v", o)
	}
}
