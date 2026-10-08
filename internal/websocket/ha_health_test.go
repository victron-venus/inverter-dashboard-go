package websocket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

func TestSuccessfulHAActionWithFailedRefreshPublishesDisconnected(t *testing.T) {
	for _, msg := range []Message{
		{Action: "toggle", Entity: "switch.lamp"},
		{Action: "scene_activate", Entity: "scene.evening"},
	} {
		t.Run(msg.Action, func(t *testing.T) {
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				status, body := http.StatusOK, `{"state":"on"}`
				if r.Method == http.MethodPost {
					posts++
					body = `[]`
				} else if posts > 0 {
					status = http.StatusServiceUnavailable
				}
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, body)
			}))
			defer server.Close()
			ha := homeassistant.NewClient(&config.HomeAssistantConfig{
				URL: server.URL, Token: "fixture", DirectControls: true,
				SwitchEntities:   []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
				FilteredEntities: &config.FilteredEntityConfig{Scenes: []string{"scene.evening"}},
			})
			o, err := ha.FetchStatesOnce()
			if err != nil {
				t.Fatal(err)
			}
			ha.ReplaceOverlay(o)
			mqtt := mockmqtt.NewClient()
			if err := handleMessage(msg, mqtt, ha); err != nil {
				t.Fatalf("successful command should not be retried after read failure: %v", err)
			}
			payload := BuildPayload(mqtt, ha)
			if payload["ha_direct_connected"] != false || ha.ControlsAvailable() || ha.ObservedAt() != nil {
				t.Fatal("action refresh left published health connected or controls fresh")
			}
			if got := payload["booleans"].(map[string]interface{})["home_lamp"]; got != nil {
				t.Fatalf("failed refresh published old on/off observation: %v", got)
			}
			if ha.GetOverlay().AdditionalFields["home_lamp"] != true || posts != 1 || len(mqtt.Published()) != 0 {
				t.Fatal("read failure discarded cached maps or retried the physical command")
			}
		})
	}
}
