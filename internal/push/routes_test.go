package push

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/victron-venus/inverter-dashboard-go/internal/auth"
)

func apiRouter(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := NewService(Config{Enabled: true, DataDir: testDirectory(t)})
	t.Cleanup(s.Close)
	if !s.Available() {
		t.Fatal("service unavailable")
	}
	router := gin.New()
	router.Use(auth.Middleware("secret"))
	RegisterRoutes(router, s)
	return router, s
}
func apiRequest(router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://dashboard.example"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://dashboard.example")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
func TestAPIRealBrowserSubscriptionAndOpaqueResponses(t *testing.T) {
	r, s := apiRouter(t)
	sub := validTestSubscription(t)
	raw, _ := json.Marshal(map[string]any{"subscription": map[string]any{"endpoint": sub.Endpoint, "expirationTime": nil, "keys": sub.Keys}, "preferences": DefaultPreferences()})
	rec := apiRequest(r, "POST", "/api/notifications/subscription", string(raw), nil)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/notifications/status", "/api/notifications/subscription/status"} {
		method := "GET"
		body := ""
		if strings.HasSuffix(path, "subscription/status") {
			method = "POST"
			body = `{"endpoint":"` + sub.Endpoint + `"}`
		}
		rec = apiRequest(r, method, path, body, nil)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || bytes.Contains(rec.Body.Bytes(), []byte(sub.Endpoint)) || bytes.Contains(rec.Body.Bytes(), []byte(sub.Keys.Auth)) {
			t.Fatal("unsafe status response")
		}
	}
	body := `{"endpoint":"` + sub.Endpoint + `"}`
	rec = apiRequest(r, "POST", "/api/notifications/test", body, nil)
	if rec.Code != 202 || queueSize(s.store) != 1 {
		t.Fatal("test not queued")
	}
	rec = apiRequest(r, "POST", "/api/notifications/test", body, nil)
	if rec.Code != 429 {
		t.Fatal("test flood")
	}
	rec = apiRequest(r, "DELETE", "/api/notifications/subscription", body, nil)
	if rec.Code != 200 || queueSize(s.store) != 0 {
		t.Fatal("delete failed")
	}
}
func TestAPICSFRAuthenticationBodyAndSchemaBounds(t *testing.T) {
	r, _ := apiRouter(t)
	for _, tc := range []struct {
		name, body string
		headers    map[string]string
		status     int
	}{
		{"missing auth", `{}`, map[string]string{"Authorization": ""}, 401},
		{"missing origin", `{}`, map[string]string{"Origin": ""}, 403},
		{"foreign origin", `{}`, map[string]string{"Origin": "https://evil.example", "X-Forwarded-Host": "evil.example"}, 403},
		{"insecure origin", `{}`, map[string]string{"Origin": "http://dashboard.example"}, 403},
		{"cross site", `{}`, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"form", `{}`, map[string]string{"Content-Type": "text/plain"}, 400},
		{"oversized", strings.Repeat(" ", 8193), nil, 413},
		{"trailing oversized", `{}` + strings.Repeat(" ", 8193), nil, 413},
		{"unknown", `{"endpoint":"https://fcm.googleapis.com/path","secret":true}`, nil, 400},
		{"arbitrary host", `{"endpoint":"https://127.0.0.1/path"}`, nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := apiRequest(r, "POST", "/api/notifications/subscription/status", tc.body, tc.headers)
			if rec.Code != tc.status || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal(rec.Code, rec.Body.String())
			}
		})
	}
	sub := validTestSubscription(t)
	raw, _ := json.Marshal(map[string]any{"subscription": map[string]any{"endpoint": sub.Endpoint, "expirationTime": nil, "keys": sub.Keys, "unexpected": true}, "preferences": DefaultPreferences()})
	if rec := apiRequest(r, "POST", "/api/notifications/subscription", string(raw), nil); rec.Code != 400 {
		t.Fatal("unknown browser field allowed")
	}
}
func TestAPIUnavailableStatusDoesNotExposeKeys(t *testing.T) {
	r := gin.New()
	s := NewService(Config{})
	RegisterRoutes(r, s)
	rec := apiRequest(r, "GET", "/api/notifications/status", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"available":false`) || !strings.Contains(rec.Body.String(), `"publicKey":null`) {
		t.Fatal(rec.Body.String())
	}
	rec = apiRequest(r, "POST", "/api/notifications/test", `{}`, nil)
	if rec.Code != 503 {
		t.Fatal(rec.Code)
	}
}
