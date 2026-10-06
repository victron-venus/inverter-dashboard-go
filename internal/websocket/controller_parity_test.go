package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"github.com/victron-venus/inverter-dashboard-go/internal/auth"
	"github.com/victron-venus/inverter-dashboard-go/internal/settings"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket/mockmqtt"
)

func TestWebSocketOriginAndCorrelatedResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := gin.New()
	server.Use(auth.Middleware("fixture"))
	client := mockmqtt.NewClient()
	server.GET("/ws", func(c *gin.Context) { HandleWebSocket(c, client, nil) })
	srv := httptest.NewServer(server)
	defer srv.Close()
	for _, origin := range []string{"https://attacker.invalid", "null", srv.URL + "/path", srv.URL + "?query=1", srv.URL + "#fragment", srv.URL, ""} {
		t.Run(origin, func(t *testing.T) {
			headers := http.Header{"Authorization": []string{"Bearer fixture"}, "X-Forwarded-Host": []string{"attacker.invalid"}}
			if origin != "" {
				headers.Set("Origin", origin)
			}
			conn, response, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", headers)
			if response != nil && response.Body != nil {
				defer func() { _ = response.Body.Close() }()
			}
			valid := origin == "" || origin == srv.URL
			if (err == nil) != valid {
				t.Fatalf("origin authorization error=%v", err)
			}
			if !valid {
				return
			}
			defer func() { _ = conn.Close() }()
			var frame map[string]interface{}
			if err := conn.ReadJSON(&frame); err != nil {
				t.Fatal(err)
			}
			if err := conn.WriteJSON(map[string]interface{}{"action": "setpoint", "value": 3, "request_id": "request-one"}); err != nil {
				t.Fatal(err)
			}
			if err := conn.ReadJSON(&frame); err != nil {
				t.Fatal(err)
			}
			if frame["type"] != "command_result" || frame["request_id"] != "request-one" || frame["status"] != "accepted" {
				t.Fatalf("unexpected scoped result: %v", frame)
			}
			if err := conn.ReadJSON(&frame); err != nil {
				t.Fatal(err)
			}
			if err := conn.WriteJSON(map[string]interface{}{"action": "toggle", "entity": "switch.unconfigured", "request_id": "request-two"}); err != nil {
				t.Fatal(err)
			}
			if err := conn.ReadJSON(&frame); err != nil {
				t.Fatal(err)
			}
			if frame["type"] != "command_error" || frame["request_id"] != "request-two" || strings.Contains(frame["error"].(string), "switch.unconfigured") {
				t.Fatal("rejection missing correlation or exposed internal details")
			}
		})
	}
}

func TestOverrideEnvelopeDistinguishesNullMissingAndFraction(t *testing.T) {
	for _, raw := range []string{`{"action":"set_setpoint_override","request_id":"id"}`, `{"action":"set_setpoint_override","request_id":"id","value":null,"topic":"bad"}`} {
		var msg Message
		if json.Unmarshal([]byte(raw), &msg) == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	var msg Message
	if json.Unmarshal([]byte(`{"action":"set_setpoint_override","request_id":"id","value":null}`), &msg) != nil {
		t.Fatal("explicit stop rejected")
	}
}

func TestWebSocketSettingsNeverContainCredential(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	settings.Init("", "Cerbo", 1883)
	if err = settings.Apply(map[string]interface{}{"ha_token": "private-fixture"}); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(BuildPayload(mockmqtt.NewClient(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-fixture") || !strings.Contains(string(raw), "***") {
		t.Fatal("private settings credential escaped into state")
	}
}
