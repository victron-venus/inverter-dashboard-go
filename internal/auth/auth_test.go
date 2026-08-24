package auth

import (
	"net/http"
	"net/http/httptest"
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

func do(r *gin.Engine, url string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
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
