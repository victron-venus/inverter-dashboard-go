package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/logging"
)

func TestHAPollPublishesDisconnectAndRecovery(t *testing.T) {
	var status atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = fmt.Fprint(w, `{"state":"on"}`)
	}))
	defer server.Close()
	c := homeassistant.NewClient(&config.HomeAssistantConfig{
		URL: server.URL, Token: "fixture", DirectControls: true,
		SwitchEntities: []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
	})
	logger := logging.New("health-test", "test", slog.LevelError)
	for _, wantConnected := range []bool{true, false, true} {
		status.Store(http.StatusOK)
		if !wantConnected {
			status.Store(http.StatusServiceUnavailable)
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
