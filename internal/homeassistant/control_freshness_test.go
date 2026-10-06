package homeassistant

import (
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func TestHAServiceRequiresKnownFreshCurrentEntity(t *testing.T) {
	for _, reason := range []string{"known", "never", "stale", "future", "unknown", "unavailable", "failed"} {
		t.Run(reason, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{URL: "http://ha.test", Token: "fixture", DirectControls: true, SwitchEntities: []config.EntityConfig{{Key: "lamp", Entity: "switch.lamp"}}})
			posts := 0
			c.httpClient = &http.Client{Transport: ownershipTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"entity_id":"switch.lamp","state":"on"}`
				status := 200
				if r.Method == http.MethodPost {
					posts++
					body = `[]`
				} else {
					switch reason {
					case "unknown", "unavailable":
						body = `{"entity_id":"switch.lamp","state":"` + reason + `"}`
					case "failed":
						status = 503
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			if reason != "never" {
				if _, err := c.FetchStatesOnce(); err != nil {
					t.Fatal(err)
				}
			}
			if reason == "stale" || reason == "future" {
				age := -31 * time.Second
				if reason == "future" {
					age = time.Minute
				}
				c.entityObserved["switch.lamp"] = time.Now().Add(age)
			}
			err := c.ToggleEntity("switch.lamp")
			if (err == nil) != (reason == "known") || posts != map[bool]int{true: 1, false: 0}[reason == "known"] {
				t.Fatalf("error=%v posts=%d", err, posts)
			}
			if reason == "failed" && (c.ControlsAvailable() || c.ObservedAt() != nil) {
				t.Fatal("failed poll looked connected/fresh")
			}
		})
	}
}

func TestHANumberServiceUsesObservedRange(t *testing.T) {
	c := NewClient(&config.HomeAssistantConfig{URL: "http://ha.test", Token: "fixture", DirectControls: true, FilteredEntities: &config.FilteredEntityConfig{Numbers: []string{"number.limit"}}})
	posts := 0
	c.httpClient = &http.Client{Transport: ownershipTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"entity_id":"number.limit","state":"1","attributes":{"min":-2,"max":3,"step":0.5}}`
		if r.Method == http.MethodPost {
			posts++
			body = `[]`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if _, err := c.FetchStatesOnce(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{-2.5, -2, 1.5, 3, 3.5} {
		err := c.PerformAction("number_set", "number.limit", map[string]interface{}{"value": value})
		if (err == nil) != (value >= -2 && value <= 3) {
			t.Fatalf("range check for %v: %v", value, err)
		}
	}
	if posts != 3 {
		t.Fatalf("unexpected physical service attempts: %d", posts)
	}
}

func TestHANumberServiceCannotCombineBoundsAndFreshnessFromDifferentPolls(t *testing.T) {
	c := NewClient(&config.HomeAssistantConfig{URL: "http://ha.test", Token: "fixture", DirectControls: true, FilteredEntities: &config.FilteredEntityConfig{Numbers: []string{"number.limit"}}})
	posts := 0
	c.httpClient = &http.Client{Transport: ownershipTransport(func(r *http.Request) (*http.Response, error) {
		posts++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})}
	stop, done := make(chan struct{}), make(chan struct{})
	defer func() { close(stop); <-done }()
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Neither poll permits 5: the broad range is stale, while the
			// current observation permits only 0..3.
			for _, fresh := range []bool{false, true} {
				at, maximum := time.Now().Add(-time.Minute), 10.0
				if fresh {
					at, maximum = time.Now(), 3
				}
				c.overlayMu.Lock()
				c.numberRanges = map[string][2]float64{"number.limit": {0, maximum}}
				c.entityObserved = map[string]time.Time{"number.limit": at}
				c.overlayMu.Unlock()
				runtime.Gosched()
			}
		}
	}()
	for range 4000 {
		if err := c.PerformAction("number_set", "number.limit", map[string]interface{}{"value": 5.0}); err == nil {
			t.Fatal("combined stale bounds with a different fresh observation")
		}
	}
	if posts != 0 {
		t.Fatal("invalid observation pair dispatched a service request")
	}
}
