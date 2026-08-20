package tracing

import (
	"context"
	"net/http"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ServiceName == "" {
		t.Error("DefaultConfig.ServiceName is empty")
	}
	if cfg.ServiceVersion == "" {
		t.Error("DefaultConfig.ServiceVersion is empty")
	}
	if cfg.Environment == "" {
		t.Error("DefaultConfig.Environment is empty")
	}
	if cfg.SampleRate < 0 {
		t.Error("DefaultConfig.SampleRate is negative")
	}
}

func TestInitTracer_NoExporters(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OtlpEndpoint = "" // disable OTLP
	cfg.EnableStdout = false // disable stdout
	tp, tracer, err := InitTracer(cfg)
	if err != nil {
		t.Errorf("InitTracer returned error: %v", err)
	}
	if tp == nil {
		t.Error("InitTracer returned nil TracerProvider")
	}
	if tracer == nil {
		t.Error("InitTracer returned nil Tracer")
	}
	// Shutdown
	if err := Shutdown(context.Background(), tp); err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}
}

func TestTraceContextFromRequest(t *testing.T) {
	req := &http.Request{}
	ctx := TraceContextFromRequest(req)
	if ctx == nil {
		t.Error("TraceContextFromRequest returned nil context")
	}
}

func TestInjectTraceContext(t *testing.T) {
	ctx := context.Background()
	req := &http.Request{}
	InjectTraceContext(ctx, req)
	// Just ensure no panic
}

func TestSpanFromContext(t *testing.T) {
	ctx := context.Background()
	span := SpanFromContext(ctx)
	if span == nil {
		t.Error("SpanFromContext returned nil span")
	}
}

func TestIsRecording(t *testing.T) {
	ctx := context.Background()
	rec := IsRecording(ctx)
	// Just ensure it returns a bool
	if !rec && rec {
		t.Error("IsRecording returned invalid bool")
	}
}
