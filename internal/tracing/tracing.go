package tracing

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds tracing configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OtlpEndpoint   string
	Insecure       bool
	EnableStdout   bool
	SampleRate     float64
}

// DefaultConfig returns a default tracing configuration
func DefaultConfig() Config {
	return Config{
		ServiceName:    "inverter-dashboard-go",
		ServiceVersion: "dev",
		Environment:    "development",
		SampleRate:     1.0,
		Insecure:       true,
	}
}

// Tracer wraps the OpenTelemetry tracer provider
type Tracer struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
}

// applyDefaults fills in default values for any unset configuration fields.
func applyDefaults(cfg Config) Config {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "inverter-dashboard-go"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "dev"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if cfg.SampleRate < 0 {
		cfg.SampleRate = 1.0
	}
	return cfg
}

// newOtlpExporter creates an OTLP HTTP span exporter for the given endpoint.
func newOtlpExporter(cfg Config) (sdktrace.SpanExporter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exp sdktrace.SpanExporter
	var err error
	if cfg.Insecure {
		exp, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.OtlpEndpoint),
			otlptracehttp.WithInsecure(),
		)
	} else {
		exp, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.OtlpEndpoint),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP HTTP exporter: %w", err)
	}
	log.Printf("OpenTelemetry OTLP HTTP exporter enabled: %s", cfg.OtlpEndpoint)
	return exp, nil
}

// newStdoutExporter creates a stdout span exporter for debugging.
func newStdoutExporter() (sdktrace.SpanExporter, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}
	log.Println("OpenTelemetry stdout exporter enabled")
	return exp, nil
}

// buildExporters constructs the list of span exporters requested by cfg.
func buildExporters(cfg Config) ([]sdktrace.SpanExporter, error) {
	var exporters []sdktrace.SpanExporter

	// OTLP HTTP exporter (for Grafana Tempo, Jaeger, etc.)
	if cfg.OtlpEndpoint != "" {
		exp, err := newOtlpExporter(cfg)
		if err != nil {
			return nil, err
		}
		exporters = append(exporters, exp)
	}

	// Stdout exporter for debugging
	if cfg.EnableStdout {
		exp, err := newStdoutExporter()
		if err != nil {
			return nil, err
		}
		exporters = append(exporters, exp)
	}

	return exporters, nil
}

// InitTracer initializes OpenTelemetry tracing with the given configuration
func InitTracer(cfg Config) (*Tracer, error) {
	cfg = applyDefaults(cfg)

	exporters, err := buildExporters(cfg)
	if err != nil {
		return nil, err
	}

	// If no exporters configured, return no-op tracer
	if len(exporters) == 0 {
		return &Tracer{
			tracer: otel.Tracer(cfg.ServiceName),
		}, nil
	}

	// Create resource with service metadata
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
			attribute.String("service.namespace", "victron-venus"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure sampler
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))

	// Build tracer provider with first exporter
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporters[0]),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Add additional exporters
	for i := 1; i < len(exporters); i++ {
		tp.RegisterSpanProcessor(sdktrace.NewBatchSpanProcessor(exporters[i]))
	}

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for trace context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		provider: tp,
		tracer:   tp.Tracer(cfg.ServiceName),
	}, nil
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context, tp *Tracer) error {
	if tp != nil {
		return tp.Shutdown(ctx)
	}
	return nil
}

// Tracer returns the underlying OpenTelemetry tracer
func (t *Tracer) Tracer() trace.Tracer {
	return t.tracer
}

// Shutdown gracefully shuts down the tracer provider
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span with the given name
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if t.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, name, opts...)
}

// StartSpanWithAttributes starts a new span with attributes
func (t *Tracer) StartSpanWithAttributes(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
	}
	return t.StartSpan(ctx, name, opts...)
}

// WrapWithSpan executes a function within a new span
func (t *Tracer) WrapWithSpan(ctx context.Context, name string, fn func(ctx context.Context) error, attrs ...attribute.KeyValue) error {
	ctx, span := t.StartSpanWithAttributes(ctx, name, attrs...)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.Bool("error", true))
	}
	return err
}

// AddSpanAttributes adds attributes to the current span
func AddSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err)
		span.SetAttributes(append(attrs, attribute.Bool("error", true))...)
	}
}

// GinMiddleware returns a Gin middleware for HTTP tracing
func GinMiddleware(tracer trace.Tracer, serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract trace context from incoming request
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		// Start a new span for the HTTP request
		spanName := c.FullPath()
		if spanName == "" {
			spanName = c.Request.URL.Path
		}

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(c.Request.Method),
				semconv.HTTPTarget(c.Request.URL.Path),
				semconv.HTTPScheme(c.Request.URL.Scheme),
				semconv.NetHostName(c.Request.Host),
				attribute.String("service.name", serviceName),
			),
		)
		defer span.End()

		// Replace request context with traced context
		c.Request = c.Request.WithContext(ctx)

		// Process request
		c.Next()

		// Add response attributes
		span.SetAttributes(
			semconv.HTTPStatusCode(c.Writer.Status()),
			semconv.HTTPResponseContentLength(c.Writer.Size()),
		)

		// Mark error if status >= 500
		if c.Writer.Status() >= 500 {
			span.SetAttributes(attribute.Bool("error", true))
		}
	}
}

// HTTPClientMiddleware returns a function that injects trace context into outgoing HTTP requests
func HTTPClientMiddleware() func(req *http.Request) {
	return func(req *http.Request) {
		otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
	}
}

// TraceContextFromRequest extracts trace context from an HTTP request
func TraceContextFromRequest(req *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header))
}

// InjectTraceContext injects trace context into an HTTP request
func InjectTraceContext(ctx context.Context, req *http.Request) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
}

// SpanFromContext returns the span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// IsRecording returns true if the span is recording
func IsRecording(ctx context.Context) bool {
	return trace.SpanFromContext(ctx).IsRecording()
}