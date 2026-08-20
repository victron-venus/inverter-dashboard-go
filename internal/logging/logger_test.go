package logging

import (
	"fmt"
	"testing"
	"log/slog"
)

func TestLoggerNew(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	if logger == nil { //nolint:staticcheck
		t.Error("New returned nil logger")
		return
	}
	if logger.service != "test-service" { //nolint:staticcheck
		t.Errorf("logger.service = %q, want %q", logger.service, "test-service")
		return
	}
	if logger.version != "1.0.0" {
		t.Errorf("logger.version = %q, want %q", logger.version, "1.0.0")
		return
	}
}

func TestLoggerWithContext(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	ctx := NewContext()
	ctx = ctx.With("key", "value")
	loggerWithCtx := logger.WithContext(ctx)
	if loggerWithCtx == nil {
		t.Error("WithContext returned nil")
		return
	}
}

func TestLoggerWith(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	loggerWith := logger.With("key", "value")
	if loggerWith == nil {
		t.Error("With returned nil")
		return
	}
}

func TestLoggerWithError(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	loggerWithErr := logger.WithError(fmt.Errorf("test error"))
	if loggerWithErr == nil {
		t.Error("WithError returned nil")
		return
	}
}

func TestLoggerInfo(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	ctx := NewContext()
	logger.Info(ctx, "info message")
	// Just ensure no panic
}

func TestLoggerError(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	ctx := NewContext()
	logger.Error(ctx, "error message")
	// Just ensure no panic
}

func TestLoggerWarn(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	ctx := NewContext()
	logger.Warn(ctx, "warn message")
	// Just ensure no panic
}

func TestLoggerDebug(t *testing.T) {
	logger := New("test-service", "1.0.0", slog.LevelInfo)
	ctx := NewContext()
	logger.Debug(ctx, "debug message")
	// Just ensure no panic
}

func TestNewContext(t *testing.T) {
	ctx := NewContext()
	if ctx.Data == nil {
		t.Error("NewContext returned nil map")
		return
	}
}

func TestContextWith(t *testing.T) {
	ctx := NewContext()
	ctx = ctx.With("key", "value")
	if ctx.Data["key"] != "value" {
		t.Errorf("Context.With failed: got %v, want value", ctx.Data["key"])
		return
	}
}

func TestDefaultContext(t *testing.T) {
	ctx := DefaultContext()
	if ctx.Data == nil {
		t.Error("DefaultContext returned nil map")
		return
	}
}
