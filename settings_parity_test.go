package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/victron-venus/inverter-dashboard-go/internal/settings"
)

func TestSettingsMutationRequiresSameOriginJSONAndMasksResponse(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	settings.Init("", "Cerbo", 1883)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings", apiSettingsPostHandler())
	for _, tc := range []struct {
		origin, media string
		want          int
	}{
		{"https://other.test", "application/json", 403}, {"https://dashboard.test", "text/plain", 415}, {"https://dashboard.test:443", "application/json", 200}, {"", "application/json", 200},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://dashboard.test/api/settings", strings.NewReader(`{"ha_token":"private-fixture"}`))
		req.Header.Set("Content-Type", tc.media)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		req.Header.Set("X-Forwarded-Host", "other.test")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != tc.want {
			t.Fatalf("origin/media %s %s status %d", tc.origin, tc.media, recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "private-fixture") {
			t.Fatal("settings response exposed credential")
		}
	}
}

func TestSettingsPersistenceFailureIsGenericServerError(t *testing.T) {
	target := filepath.Join(t.TempDir(), "private-settings.json")
	t.Setenv("INVERTER_DASHBOARD_SETTINGS_FILE", target)
	settings.Init("", "Cerbo", 1883)
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/api/settings", apiSettingsPostHandler())
	for _, tc := range []struct {
		body string
		want int
	}{{`{"show_ev":false}`, 500}, {`{"show_ev":"no"}`, 400}, {`{"unknown":true}`, 400}} {
		req := httptest.NewRequest(http.MethodPost, "http://dashboard.test/api/settings", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != tc.want || strings.Contains(recorder.Body.String(), target) {
			t.Fatalf("status=%d response=%s", recorder.Code, recorder.Body.String())
		}
	}
}
