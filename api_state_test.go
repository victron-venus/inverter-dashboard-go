package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
