// Package auth provides bearer-token authentication middleware mirroring the
// Python dashboard's DASHBOARD_SECRET behavior.
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Middleware rejects requests unless they carry the expected secret via an
// "Authorization: Bearer <secret>" header or a "?token=<secret>" query param
// (the latter keeps browser and WebSocket URLs simple). When secret is empty
// the middleware is a no-op, matching Python's unset-DASHBOARD_SECRET mode.
func Middleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}

		switch check(c.Request, secret) {
		case checkOK:
			c.Next()
		case checkMissing:
			respondError(c, http.StatusUnauthorized, "missing secret")
		case checkInvalid:
			respondError(c, http.StatusForbidden, "invalid secret")
		}
	}
}

type checkResult int

const (
	checkOK checkResult = iota
	checkMissing
	checkInvalid
)

func check(r *http.Request, secret string) checkResult {
	given := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		given = strings.TrimSpace(h[len("Bearer "):])
	} else if t := r.URL.Query().Get("token"); t != "" {
		given = t
	} else {
		return checkMissing
	}
	if subtle.ConstantTimeCompare([]byte(given), []byte(secret)) == 1 {
		return checkOK
	}
	return checkInvalid
}

func respondError(c *gin.Context, status int, msg string) {
	if strings.Contains(c.GetHeader("Accept"), "text/html") {
		c.Data(status, "text/html; charset=utf-8", []byte(
			"<h1>Inverter Dashboard</h1><p>"+msg+". Append <code>?token=YOUR_SECRET</code> "+
				"to the URL or send an <code>Authorization: Bearer</code> header.</p>"))
		return
	}
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}
