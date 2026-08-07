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

// InitTracer initializes OpenTelemetry tracing with the given configuration.
// Returns the TracerProvider (for shutdown) and a Tracer for creating spans.
func InitTracer(cfg Config) (*sdktrace.TracerProvider, trace.Tracer, error) {
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

	var exporters []sdktrace.SpanExporter

	// OTLP HTTP exporter (for Grafana Tempo, Jaeger, etc.)
	if cfg.OtlpEndpoint != "" {
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
			return nil, nil, fmt.Errorf("failed to create OTLP HTTP exporter: %w", err)
		}
		log.Printf("OpenTelemetry OTLP HTTP exporter enabled: %s", cfg.OtlpEndpoint)
		exporters = append(exporters, exp)
	}

	// Stdout exporter for debugging
	if cfg.EnableStdout {
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
		log.Println("OpenTelemetry stdout exporter enabled")
		exporters = append(exporters, exp)
	}

	// If no exporters configured, return no-op tracer provider
	if len(exporters) == 0 {
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, tp.Tracer(cfg.ServiceName), nil
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
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure sampler
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))

	// Build tracer provider
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

	return tp, tp.Tracer(cfg.ServiceName), nil
}

// Shutdown gracefully shuts down the tracer provider
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp != nil {
		return tp.Shutdown(ctx)
	}
	return nil
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
