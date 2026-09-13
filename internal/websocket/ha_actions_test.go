package websocket

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

type haActionTransport func(*http.Request) (*http.Response, error)

func (f haActionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func actionHAClient() *homeassistant.Client {
	return homeassistant.NewClient(&config.HomeAssistantConfig{
		URL: "http://ha.test", Token: "test-token", DirectControls: true,
		SwitchEntities: []config.EntityConfig{{Key: "laundry_start", Entity: "button.washer_start"}},
		FilteredEntities: &config.FilteredEntityConfig{
			Numbers: []string{"number.limit", "input_number.helper"}, Covers: []string{"cover.blind"},
			MediaPlayers: []string{"media_player.radio"}, Scenes: []string{"scene.evening"},
		},
	})
}

func TestRichHADispatchMakesRESTRequest(t *testing.T) {
	cases := []struct{ message, path, body string }{
		{`{"action":"number_set","entity":"number.limit","value":12.5}`, "number/set_value", `{"entity_id":"number.limit","value":12.5}`},
		{`{"action":"number_set","entity":"input_number.helper","value":0}`, "input_number/set_value", `{"entity_id":"input_number.helper","value":0}`},
		{`{"action":"set_cover_position","entity":"cover.blind","position":0}`, "cover/set_cover_position", `{"entity_id":"cover.blind","position":0}`},
		{`{"action":"media_player","entity":"media_player.radio","mp_action":"play"}`, "media_player/media_play", `{"entity_id":"media_player.radio"}`},
		{`{"action":"media_player","entity":"media_player.radio","mp_action":"pause"}`, "media_player/media_pause", `{"entity_id":"media_player.radio"}`},
		{`{"action":"media_player","entity":"media_player.radio","mp_action":"stop"}`, "media_player/media_stop", `{"entity_id":"media_player.radio"}`},
		{`{"action":"scene_activate","entity":"scene.evening"}`, "scene/turn_on", `{"entity_id":"scene.evening"}`},
		{`{"action":"press","entity":"button.washer_start"}`, "button/press", `{"entity_id":"button.washer_start"}`},
		{`{"action":"toggle","entity":"button.washer_start"}`, "button/press", `{"entity_id":"button.washer_start"}`},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			previous := http.DefaultTransport
			defer func() { http.DefaultTransport = previous }()
			posts := 0
			http.DefaultTransport = haActionTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"entity_id":"sensor.test","state":"0","attributes":{}}`
				if r.Method == http.MethodPost {
					posts++
					if r.URL.String() != "http://ha.test/api/services/"+tc.path || r.Header.Get("Authorization") != "Bearer test-token" {
						t.Errorf("unexpected target/auth: %s", r.URL)
					}
					var actual, expected map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&actual)
					_ = json.Unmarshal([]byte(tc.body), &expected)
					actualJSON, _ := json.Marshal(actual)
					expectedJSON, _ := json.Marshal(expected)
					if string(actualJSON) != string(expectedJSON) {
						t.Errorf("body=%s, want=%s", actualJSON, expectedJSON)
					}
					body = `[]`
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			var msg Message
			if err := json.Unmarshal([]byte(tc.message), &msg); err != nil {
				t.Fatal(err)
			}
			mqtt := mockmqtt.NewClient()
			if err := handleMessage(msg, mqtt, actionHAClient()); err != nil {
				t.Fatal(err)
			}
			if posts != 1 || len(mqtt.Published()) != 0 {
				t.Errorf("posts=%d, daemon commands=%v", posts, mqtt.Published())
			}
		})
	}
}

func TestRichHARejectsInvalidOrUnconfiguredActions(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = haActionTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("rejected action sent request: %s", r.URL)
		return nil, nil
	})
	cases := []Message{
		{Action: "number_set", Entity: "number.unconfigured", Value: 1.0},
		{Action: "number_set", Entity: "number.limit", Value: "1"},
		{Action: "number_set", Entity: "number.limit", Value: true},
		{Action: "number_set", Entity: "number.limit", Value: math.NaN()},
		{Action: "number_set", Entity: "number.limit", Value: math.Inf(1)},
		{Action: "number_set", Entity: "number.limit"},
		{Action: "number_set", Entity: "input_boolean.only_charging", Value: 1.0},
		{Action: "set_cover_position", Entity: "cover.blind", Position: -1.0},
		{Action: "set_cover_position", Entity: "cover.blind", Position: 101.0},
		{Action: "set_cover_position", Entity: "cover.blind", Position: 0.5},
		{Action: "set_cover_position", Entity: "cover.blind", Position: true},
		{Action: "media_player", Entity: "media_player.radio", MPAction: "delete"},
		{Action: "scene_activate", Entity: "scene.unconfigured"},
		{Action: "press", Entity: "button.unconfigured"},
	}
	for _, msg := range cases {
		mqtt := mockmqtt.NewClient()
		if err := handleMessage(msg, mqtt, actionHAClient()); err == nil {
			t.Errorf("accepted invalid message: %#v", msg)
		}
		if len(mqtt.Published()) != 0 {
			t.Errorf("invalid action reached daemon: %#v", mqtt.Published())
		}
	}
	if err := handleMessage(Message{Action: "scene_activate", Entity: "scene.evening"}, mockmqtt.NewClient(), nil); err == nil {
		t.Fatal("accepted rich HA action without HA")
	}
}

func TestHAServiceFailureDoesNotFallback(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	http.DefaultTransport = haActionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	mqtt := mockmqtt.NewClient()
	if err := handleMessage(Message{Action: "press", Entity: "button.washer_start"}, mqtt, actionHAClient()); err == nil {
		t.Fatal("failed HA action reported success")
	}
	if len(mqtt.Published()) != 0 {
		t.Fatal("HA failure sent a duplicate daemon command")
	}
}
