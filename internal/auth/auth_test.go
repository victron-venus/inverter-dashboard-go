package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRouter(t *testing.T, secret string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(secret))
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

func doMethod(r *gin.Engine, method, url string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func do(r *gin.Engine, url string, headers map[string]string) *httptest.ResponseRecorder {
	return doMethod(r, http.MethodGet, url, headers)
}

func TestMiddlewareNoopWhenSecretEmpty(t *testing.T) {
	r := newRouter(t, "")
	if w := do(r, "/", nil); w.Code != http.StatusOK {
		t.Fatalf("empty secret must be open: got %d", w.Code)
	}
}

func TestMiddlewareMissingSecret(t *testing.T) {
	r := newRouter(t, "s3cret")
	if w := do(r, "/", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no credentials: got %d, want 401", w.Code)
	}
}

func TestMiddlewareInvalidSecret(t *testing.T) {
	r := newRouter(t, "s3cret")
	if w := do(r, "/?token=wrong", nil); w.Code != http.StatusForbidden {
		t.Fatalf("bad query token: got %d, want 403", w.Code)
	}
	if w := do(r, "/", map[string]string{"Authorization": "Bearer wrong"}); w.Code != http.StatusForbidden {
		t.Fatalf("bad bearer: got %d, want 403", w.Code)
	}
}

func TestMiddlewareValidSecret(t *testing.T) {
	r := newRouter(t, "s3cret")
	if w := do(r, "/?token=s3cret", nil); w.Code != http.StatusOK {
		t.Fatalf("query token: got %d, want 200", w.Code)
	}
	if w := do(r, "/", map[string]string{"Authorization": "Bearer s3cret"}); w.Code != http.StatusOK {
		t.Fatalf("bearer header: got %d, want 200", w.Code)
	}
}

func TestMiddlewareHTMLHintForBrowsers(t *testing.T) {
	r := newRouter(t, "s3cret")
	w := do(r, "/", map[string]string{"Accept": "text/html,application/xhtml+xml"})
	if w.Code != http.StatusUnauthorized || w.Body.Len() == 0 {
		t.Fatalf("browser request: got %d, body %q", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("content type: got %q", ct)
	}
}

func TestMiddlewareSkipsHealthAndMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware("s3cret"))
	r.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "healthy") })
	r.GET("/metrics", func(c *gin.Context) { c.String(http.StatusOK, "metrics") })
	r.GET("/api/state", func(c *gin.Context) { c.String(http.StatusOK, "state") })

	if w := do(r, "/health", nil); w.Code != http.StatusOK {
		t.Fatalf("/health must stay open without secret: got %d", w.Code)
	}
	if w := do(r, "/metrics", nil); w.Code != http.StatusOK {
		t.Fatalf("/metrics must stay open without secret: got %d", w.Code)
	}
	if w := do(r, "/api/state", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("/api/state must still require secret: got %d", w.Code)
	}
}

func TestMiddlewareHTMLAbortStopsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	htmlAccept := map[string]string{"Accept": "text/html,application/xhtml+xml"}

	cases := []struct {
		name       string
		method     string
		url        string
		headers    map[string]string
		wantStatus int
	}{
		{
			name:       "missing token GET 401",
			method:     http.MethodGet,
			url:        "/api/state",
			headers:    htmlAccept,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token GET 403",
			method:     http.MethodGet,
			url:        "/api/state?token=wrong",
			headers:    htmlAccept,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing token POST settings 401",
			method:     http.MethodPost,
			url:        "/api/settings",
			headers:    htmlAccept,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token POST settings 403",
			method:     http.MethodPost,
			url:        "/api/settings?token=wrong",
			headers:    htmlAccept,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handlerRan := false
			r := gin.New()
			r.Use(Middleware("s3cret"))
			handler := func(c *gin.Context) {
				handlerRan = true
				c.String(http.StatusOK, "CANARY_PROTECTED")
			}
			r.GET("/api/state", handler)
			r.POST("/api/settings", handler)

			req := httptest.NewRequest(tc.method, tc.url, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", w.Code, tc.wantStatus)
			}
			if handlerRan {
				t.Fatalf("protected handler must not run after HTML auth failure")
			}
			if strings.Contains(w.Body.String(), "CANARY_PROTECTED") {
				t.Fatalf("response must not include protected handler body: %q", w.Body.String())
			}
		})
	}
}

func TestMiddlewareAllowsAssetsWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware("s3cret"))
	r.GET("/assets/index-test.js", func(c *gin.Context) {
		c.String(http.StatusOK, "console.log(1)")
	})
	r.GET("/api/state", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/settings", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Browser subresource: no ?token= on /assets (index.html refs are bare paths).
	w := do(r, "/assets/index-test.js", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /assets without token: got %d, want 200", w.Code)
	}

	// Protected API/write routes must still require credentials.
	w = do(r, "/api/state", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/state without token: got %d, want 401", w.Code)
	}
	w = doMethod(r, http.MethodPost, "/api/settings", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/settings without token: got %d, want 401", w.Code)
	}
}

func TestAuthenticatedPageAssetRefsLoadWithoutQueryToken(t *testing.T) {
	// Mirrors Python test_embedded_spa_asset_references_are_served: authorized
	// HTML references bare /assets/... paths that must load without ?token=.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware("s3cret"))
	const htmlBody = "<!doctype html><script type=\"module\" src=\"/assets/index-DZCqHsvF.js\"></script>" +
		"<link rel=\"stylesheet\" href=\"/assets/index-BjpFTIot.css\">"
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlBody))
	})
	r.GET("/assets/index-DZCqHsvF.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/javascript", []byte("export{}"))
	})
	r.GET("/assets/index-BjpFTIot.css", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/css", []byte("body{}"))
	})

	w := do(r, "/?token=s3cret", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("authorized index: got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "/assets/index-DZCqHsvF.js") || !strings.Contains(body, "/assets/index-BjpFTIot.css") {
		t.Fatalf("index missing asset refs: %q", body)
	}
	for _, path := range []string{"/assets/index-DZCqHsvF.js", "/assets/index-BjpFTIot.css"} {
		aw := do(r, path, nil)
		if aw.Code != http.StatusOK {
			t.Fatalf("GET %s without token: got %d, want 200 (authenticated page would not load JS/CSS)", path, aw.Code)
		}
	}
}

func TestMiddlewareAssetsExemptionIsStrictlyPrefixed(t *testing.T) {
	// /assets and /assets/* restore the Python StaticFiles contract only.
	// Prefix lookalikes and all API/write routes stay guarded.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware("s3cret"))
	okHandler := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	r.GET("/assets", okHandler)
	r.GET("/assets/app.js", okHandler)
	r.GET("/assetsfoo", okHandler)
	r.GET("/asset", okHandler)
	r.GET("/api/assets", okHandler)
	r.GET("/api/state", okHandler)
	r.POST("/api/command", okHandler)
	r.PUT("/api/settings", okHandler)
	r.DELETE("/api/session", okHandler)
	r.GET("/ws", okHandler)

	if w := do(r, "/assets", nil); w.Code != http.StatusOK {
		t.Fatalf("GET /assets: got %d, want 200", w.Code)
	}
	if w := do(r, "/assets/app.js", nil); w.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js: got %d, want 200", w.Code)
	}

	for _, path := range []string{"/assetsfoo", "/asset", "/api/assets", "/api/state", "/ws"} {
		w := do(r, path, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s without token: got %d, want 401", path, w.Code)
		}
	}
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/command"},
		{http.MethodPut, "/api/settings"},
		{http.MethodDelete, "/api/session"},
	} {
		w := doMethod(r, tc.method, tc.path, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: got %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}
