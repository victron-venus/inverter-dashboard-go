package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
)

func TestWebsocketWithoutHomeAssistant(t *testing.T) {
	router := gin.New()
	router.Use(gin.Recovery())
	mqttClient := mqtt.NewClient("localhost", 1883)
	router.GET("/ws", websocketHandler(mqttClient, nil))
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var payload map[string]interface{}
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("MQTT-only dashboard must send initial state: %v", err)
	}
	if _, ok := payload["dashboard_version"]; !ok {
		t.Fatal("initial state has no dashboard version")
	}

	mqttMessageHandler(mqttClient, nil)()
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("MQTT-only dashboard must broadcast subsequent state: %v", err)
	}
	if _, ok := payload["dashboard_version"]; !ok {
		t.Fatal("broadcast state has no dashboard version")
	}
}
