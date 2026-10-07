package main

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/logging"
)

type healthTransport func(*http.Request) (*http.Response, error)

func (f healthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHAPollPublishesDisconnectAndRecovery(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	status := http.StatusOK
	http.DefaultTransport = healthTransport(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"state":"on"}`))}, nil
	})
	c := homeassistant.NewClient(&config.HomeAssistantConfig{
		URL: "http://ha.test", Token: "fixture", DirectControls: true,
		SwitchEntities: []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
	})
	logger := logging.New("health-test", "test", slog.LevelError)
	for _, wantConnected := range []bool{true, false, true} {
		status = http.StatusOK
		if !wantConnected {
			status = http.StatusServiceUnavailable
		}
		haPollTick(c, logger)
		if c.GetOverlay().HADirectConnected != wantConnected || c.ControlsAvailable() != wantConnected {
			t.Fatalf("poll health=%v controls=%v, want %v", c.GetOverlay().HADirectConnected, c.ControlsAvailable(), wantConnected)
		}
		if c.GetOverlay().AdditionalFields["home_lamp"] != true {
			t.Fatal("poll discarded last committed value")
		}
	}
}
