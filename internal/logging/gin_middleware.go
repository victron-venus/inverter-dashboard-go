package logging

import (
	"github.com/gin-gonic/gin"
	"time"
)

// NewStructuredMiddleware creates a Gin middleware for structured logging
func NewStructuredMiddleware(logger *Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()

		// Create request context, enriched with trace/span IDs if a
		// tracing middleware has already populated the request context.
		ctx := ExtractTraceInfo(c.Request.Context(), DefaultContext().
			With("component", "http").
			WithRequestID(c.GetHeader("X-Request-ID")))

		// Log request start
		logger.Info(ctx, "HTTP request started",
			"method", method,
			"path", path,
			"remote_ip", clientIP,
			"user_agent", c.Request.UserAgent(),
		)

		// Process request
		c.Next()

		// Log request completion
		latency := time.Since(start)
		status := c.Writer.Status()
		size := c.Writer.Size()

		// Add error info if any
		fields := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", float64(latency.Microseconds()) / 1000.0,
			"response_size", size,
		}

		if len(c.Errors) > 0 {
			fields = append(fields, "errors", c.Errors.String())
			logger.Error(ctx.With("component", "http"), "HTTP request completed with errors", fields...)
		} else if status >= 500 {
			logger.Error(ctx.With("component", "http"), "HTTP request completed with server error", fields...)
		} else if status >= 400 {
			logger.Warn(ctx.With("component", "http"), "HTTP request completed with client error", fields...)
		} else {
			logger.Info(ctx.With("component", "http"), "HTTP request completed", fields...)
		}
	}
}