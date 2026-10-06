package homeassistant

import (
	"io"
	"net/http"
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
