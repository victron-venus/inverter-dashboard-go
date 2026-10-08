package tracing

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"os"
	"strings"

	"github.com/victron-venus/inverter-dashboard-go/internal/tlsclient"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func otlpEnv(name string) string {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_" + name))
}

// otlpUsesHTTP follows the pinned exporter's endpoint, then boolean environment
// precedence. The application's explicit WithInsecure option is applied last.
func otlpUsesHTTP(cfg Config) bool {
	if cfg.Insecure {
		return true
	}
	insecure := false
	for _, name := range []string{"ENDPOINT", "TRACES_ENDPOINT"} {
		if value := otlpEnv(name); value != "" {
			if endpoint, err := url.Parse(value); err == nil {
				scheme := strings.ToLower(endpoint.Scheme)
				insecure = scheme == "http" || scheme == "unix"
			}
		}
	}
	for _, name := range []string{"INSECURE", "TRACES_INSECURE"} {
		if value := otlpEnv(name); value != "" {
			insecure = strings.EqualFold(value, "true")
		}
	}
	return insecure
}

// otlpTLSConfig preserves the exporter's generic-to-traces CA and complete-pair
// selection, including fallback after invalid values. The exporter still reads
// the environment itself and reports its normal configuration diagnostics.
func otlpTLSConfig() (*tls.Config, error) {
	config := tlsclient.NewConfig()
	for _, name := range []string{"CERTIFICATE", "TRACES_CERTIFICATE"} {
		if path := otlpEnv(name); path != "" {
			if pem, err := os.ReadFile(path); err == nil {
				pool := x509.NewCertPool()
				if pool.AppendCertsFromPEM(pem) {
					config.RootCAs = pool
				}
			}
		}
	}
	for _, prefix := range []string{"", "TRACES_"} {
		certPath, keyPath := otlpEnv(prefix+"CLIENT_CERTIFICATE"), otlpEnv(prefix+"CLIENT_KEY")
		if certPath != "" && keyPath != "" {
			if certificate, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
				config.Certificates = []tls.Certificate{certificate}
			}
		}
	}
	for _, certificate := range config.Certificates {
		if err := tlsclient.ValidateLocalCertificate(certificate); err != nil {
			return nil, err
		}
	}
	return config, nil
}

func newOTLPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	options := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OtlpEndpoint)}
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	if !otlpUsesHTTP(cfg) {
		config, err := otlpTLSConfig()
		if err != nil {
			return nil, err
		}
		options = append(options, otlptracehttp.WithTLSClientConfig(config))
	}
	return otlptracehttp.New(ctx, options...)
}
