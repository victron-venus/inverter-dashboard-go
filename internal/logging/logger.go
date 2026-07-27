package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// Logger wraps slog.Logger with additional context
type Logger struct {
	*slog.Logger
	service string
	version string
}

// New creates a new structured logger
func New(service, version string, level slog.Level) *Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Rename "level" to "severity" for compatibility with logging systems
			if a.Key == "level" {
				return slog.String("severity", a.Value.String())
			}
			// Add timestamp in RFC3339 format
			if a.Key == "time" {
				return slog.String("timestamp", time.Now().UTC().Format(time.RFC3339))
			}
			return a
		},
	}

	// Use JSON handler for production, text for development
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// Add service and version as default attributes
	logger := slog.New(handler).With(
		"service", service,
		"version", version,
	)

	return &Logger{
		Logger:  logger,
		service: service,
		version: version,
	}
}

// WithContext adds context attributes to the logger
func (l *Logger) WithContext(ctx Context) *Logger {
	attrs := make([]any, 0, len(ctx.Data)*2)
	for k, v := range ctx.Data {
		attrs = append(attrs, k, v)
	}
	return &Logger{
		Logger:  l.Logger.With(attrs...),
		service: l.service,
		version: l.version,
	}
}

// With adds key-value pairs to the logger
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger:  l.Logger.With(args...),
		service: l.service,
		version: l.version,
	}
}

// WithError adds an error to the logger context
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return l.With("error", err.Error())
}

// Error logs an error message with error details
func (l *Logger) Error(ctx Context, msg string, args ...any) {
	l.log(context.Background(), ctx, slog.LevelError, msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(ctx Context, msg string, args ...any) {
	l.log(context.Background(), ctx, slog.LevelWarn, msg, args...)
}

// Info logs an info message
func (l *Logger) Info(ctx Context, msg string, args ...any) {
	l.log(context.Background(), ctx, slog.LevelInfo, msg, args...)
}

// Debug logs a debug message
func (l *Logger) Debug(ctx Context, msg string, args ...any) {
	l.log(context.Background(), ctx, slog.LevelDebug, msg, args...)
}

func (l *Logger) log(ctx context.Context, logCtx Context, level slog.Level, msg string, args ...any) {
	// Add caller info for error/warn levels
	if level >= slog.LevelWarn {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			args = append(args, "caller", fmt.Sprintf("%s:%d", file, line))
		}
	}

	// Add log context attributes
	for k, v := range logCtx.Data {
		args = append(args, k, v)
	}

	l.Logger.Log(ctx, level, msg, args...)
}

// Context carries request-scoped context for logging
type Context struct {
	Data map[string]any
}

// NewContext creates a new logging context
func NewContext() Context {
	return Context{Data: make(map[string]any)}
}

// With adds a key-value pair to the context
func (c Context) With(key string, value any) Context {
	c.Data[key] = value
	return c
}

// WithTraceID adds a trace ID to the context
func (c Context) WithTraceID(traceID string) Context {
	return c.With("trace_id", traceID)
}

// WithSpanID adds a span ID to the context
func (c Context) WithSpanID(spanID string) Context {
	return c.With("span_id", spanID)
}

// WithRequestID adds a request ID to the context
func (c Context) WithRequestID(requestID string) Context {
	return c.With("request_id", requestID)
}

// ExtractTraceInfo extracts trace/span IDs from context if available
func ExtractTraceInfo(ctx Context) Context {
	// This would integrate with OpenTelemetry if needed
	return ctx
}

// NewFromContext creates a logger with context extracted from a request
func (l *Logger) NewFromContext(ctx Context) *Logger {
	return l.WithContext(ctx)
}

// DefaultContext returns a new empty context
func DefaultContext() Context {
	return NewContext()
}

// JSONEncoder provides a custom JSON encoder for structured logging
type JSONEncoder struct {
	writer io.Writer
}

// NewJSONEncoder creates a new JSON encoder
func NewJSONEncoder(w io.Writer) *JSONEncoder {
	return &JSONEncoder{writer: w}
}

// Encode writes a log entry as JSON
func (e *JSONEncoder) Encode(entry map[string]any) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = e.writer.Write(append(data, '\n'))
	return err
}