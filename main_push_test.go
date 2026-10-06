package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/victron-venus/inverter-dashboard-go/internal/auth"
	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/html"
	"github.com/victron-venus/inverter-dashboard-go/internal/logging"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
)

func TestNotificationStaticRoutesAreNarrowAndCorrectlyTyped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.Middleware("test-secret"))
	registerNotificationAssets(r, func(name string) ([]byte, bool) { return []byte("fixture:" + name), true })
	for path, mime := range map[string]string{"/notifications-sw.js": "application/javascript; charset=utf-8", "/manifest.webmanifest": "application/manifest+json", "/notification-icon.svg": "image/svg+xml"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" || rec.Header().Get("Content-Type") != mime {
			t.Fatal(path, rec.Code, rec.Header())
		}
		if path == "/notifications-sw.js" && rec.Header().Get("Service-Worker-Allowed") != "/" {
			t.Fatal("worker root scope missing")
		}
	}
	for _, path := range []string{"/notifications-sw.js/extra", "/manifest.webmanifest/private", "/notification-icon.svg.bak", "/api/notifications/status", "/api/state"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 401 {
			t.Fatal("auth surface broadened", path, rec.Code)
		}
	}
}
func TestCreateServerIncludesExactNotificationAssetRoutesAndProtectsAPI(t *testing.T) {
	client := mqtt.NewClient("", 0)
	logger := logging.New("test", "test", slog.LevelError)
	router := createServer(client, nil, &config.Config{DashboardSecret: "secret"}, logger, nil)
	for _, name := range []string{"notifications-sw.js", "manifest.webmanifest", "notification-icon.svg"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest("GET", "/"+name, nil))
		data, present := html.GetNotificationAsset(name)
		want := http.StatusNotFound
		if present {
			want = http.StatusOK
		}
		if rec.Code != want || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal(name, rec.Code)
		}
		if present && rec.Body.String() != string(data) {
			t.Fatal("embedded bytes changed")
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/api/notifications/status", nil))
	if rec.Code != 401 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("notification API unprotected")
	}
}
