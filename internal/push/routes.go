package push

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes deliberately does not install authentication. The hosting
// router must retain the dashboard's existing authentication middleware.
func RegisterRoutes(router gin.IRouter, service *Service) {
	group := router.Group("/api/notifications")
	group.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") })
	group.GET("/status", func(c *gin.Context) { c.JSON(http.StatusOK, service.status()) })
	mutations := group.Group("")
	mutations.Use(func(c *gin.Context) {
		origin, err := url.Parse(c.GetHeader("Origin"))
		site := c.GetHeader("Sec-Fetch-Site")
		media, _, mediaErr := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || origin.Scheme != "https" || origin.Host != c.Request.Host || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || (site != "" && site != "same-origin") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "same-origin HTTPS request required"})
			return
		}
		if mediaErr != nil || media != "application/json" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "JSON request required"})
			return
		}
		if !service.Available() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "notifications unavailable"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBytes)
	})
	mutations.POST("/subscription/status", func(c *gin.Context) {
		endpoint, ok := readEndpoint(c)
		if !ok {
			return
		}
		prefs, registered := service.store.SubscriptionStatus(endpoint)
		c.JSON(http.StatusOK, gin.H{"registered": registered, "preferences": prefs})
	})
	mutations.POST("/subscription", func(c *gin.Context) {
		var body struct {
			Subscription *struct {
				Endpoint       string       `json:"endpoint"`
				ExpirationTime *float64     `json:"expirationTime"`
				Keys           webpush.Keys `json:"keys"`
			} `json:"subscription"`
			Preferences *Preferences `json:"preferences"`
		}
		if !readJSON(c, &body) {
			return
		}
		if body.Subscription == nil || body.Preferences == nil {
			replyError(c, errPushEndpoint)
			return
		}
		if err := service.store.Register(webpush.Subscription{Endpoint: body.Subscription.Endpoint, Keys: body.Subscription.Keys}, *body.Preferences); err != nil {
			replyError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"registered": true, "preferences": body.Preferences})
	})
	mutations.DELETE("/subscription", func(c *gin.Context) {
		endpoint, ok := readEndpoint(c)
		if !ok {
			return
		}
		if err := service.store.Delete(endpoint); err != nil {
			replyError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"registered": false})
	})
	mutations.POST("/test", func(c *gin.Context) {
		endpoint, ok := readEndpoint(c)
		if !ok {
			return
		}
		if err := service.store.QueueTest(endpoint, time.Now()); err != nil {
			replyError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"queued": true})
	})
}
func readJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		if trailingErr := decoder.Decode(new(any)); trailingErr != io.EOF {
			if trailingErr != nil {
				err = trailingErr
			} else {
				err = errors.New("invalid JSON")
			}
		}
	}
	if err == nil {
		return true
	}
	var size *http.MaxBytesError
	status := http.StatusBadRequest
	if errors.As(err, &size) {
		status = http.StatusRequestEntityTooLarge
	}
	c.AbortWithStatusJSON(status, gin.H{"error": "invalid notification request"})
	return false
}
func readEndpoint(c *gin.Context) (string, bool) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if !readJSON(c, &body) {
		return "", false
	}
	if _, err := validateEndpoint(body.Endpoint); err != nil {
		replyError(c, errPushEndpoint)
		return "", false
	}
	return body.Endpoint, true
}
func replyError(c *gin.Context, err error) {
	status := http.StatusServiceUnavailable
	message := "notifications unavailable"
	switch {
	case errors.Is(err, errPushEndpoint):
		status = http.StatusBadRequest
		message = "invalid or unsupported push subscription"
	case errors.Is(err, errConflict):
		status = http.StatusConflict
		message = "subscription key conflict"
	case errors.Is(err, errNotFound):
		status = http.StatusNotFound
		message = "subscription not registered"
	case errors.Is(err, errCapacity), errors.Is(err, errRateLimit):
		status = http.StatusTooManyRequests
		message = "notification limit reached"
	}
	// Error values may originate from persistence or providers. Never echo them.
	c.AbortWithStatusJSON(status, gin.H{"error": strings.TrimSpace(message)})
}
