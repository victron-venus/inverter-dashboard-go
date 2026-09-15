package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestHTTPFallbackCarriesFullNativeState(t *testing.T) {
	client := mqtt.NewClient("localhost", 1883)
	client.ApplyState(&state.State{
		G3: 42, T3: 100, BatterySOC: 0, EVPower: 3200,
		Batteries:          []state.Battery{{Name: "BMS", SOC: 0}},
		TelemetryAvailable: map[string]bool{"battery_soc": true, "g3": true},
	})
	router := gin.New()
	router.GET("/api/state", apiStateHandler(client))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/state", nil))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["g3"] != float64(42) || payload["t3"] != float64(100) || payload["ev_power"] != float64(3200) {
		t.Fatalf("HTTP fallback omitted native fields: %v", payload)
	}
	if len(payload["batteries"].([]interface{})) != 1 || !payload["telemetry_available"].(map[string]interface{})["battery_soc"].(bool) {
		t.Fatalf("HTTP fallback lost device/availability state: %v", payload)
	}
}

func TestHTTPAndWebSocketAgreeOnGatewayTransport(t *testing.T) {
	client := mqtt.NewClient("", 0)
	client.EnableGatewayMode()
	client.SetGatewayConnected(true)
	client.ApplyState(&state.State{G3: 42})
	router := gin.New()
	router.GET("/api/state", apiStateHandler(client))
	router.GET("/health", healthHandler(client))
	router.GET("/ws", websocketHandler(client, nil))
	server := httptest.NewServer(router)
	defer server.Close()
	server.Client().Timeout = 2 * time.Second

	for _, connected := range []bool{true, false, true} {
		client.SetGatewayConnected(connected)
		if connected {
			client.ApplyState(&state.State{G3: 42})
		}
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		var socketPayload map[string]interface{}
		err = conn.ReadJSON(&socketPayload)
		_ = conn.Close()
		if err != nil {
			t.Fatal(err)
		}
		httpPayload := getEndpointPayload(t, server, "/api/state")
		healthPayload := getEndpointPayload(t, server, "/health")
		for endpoint, payload := range map[string]map[string]interface{}{
			"WebSocket": socketPayload, "/api/state": httpPayload, "/health": healthPayload,
		} {
			for key, want := range map[string]interface{}{
				"data_source": "igw", "mqtt_connected": false,
				"gateway_connected": connected, "native_connected": connected,
			} {
				if payload[key] != want {
					t.Errorf("%s %s = %v, want %v", endpoint, key, payload[key], want)
				}
			}
		}
		wantHealth, wantQuality := "degraded", "stale"
		if connected {
			wantHealth, wantQuality = "ok", "live"
		}
		if healthPayload["status"] != wantHealth {
			t.Errorf("IGW health = %v, want %s", healthPayload["status"], wantHealth)
		}
		if !reflect.DeepEqual(httpPayload["telemetry"], socketPayload["telemetry"]) {
			t.Error("HTTP and WebSocket disagree on telemetry freshness")
		}
		telemetry, ok := httpPayload["telemetry"].(map[string]interface{})
		if !ok || telemetry["quality"] != wantQuality || telemetry["source"] != "igw" || telemetry["timestamp_source"] != "local_receipt" {
			t.Errorf("incorrect IGW telemetry: %v", telemetry)
		}
	}
}

func getEndpointPayload(t *testing.T, server *httptest.Server, path string) map[string]interface{} {
	t.Helper()
	response, err := server.Client().Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s status = %d", path, response.StatusCode)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
