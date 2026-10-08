package tracing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/tlsclient"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// referenceOTLPExporter characterizes the pinned library with the exact options
// used before the application's certificate-key policy is added.
func referenceOTLPExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	options := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.OtlpEndpoint)}
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	return otlptracehttp.New(ctx, options...)
}

type otlpIdentity struct {
	certificate       *x509.Certificate
	key               crypto.Signer
	pair              tls.Certificate
	certPath, keyPath string
}

func issueOTLPIdentity(t *testing.T, dir, name string, parent *otlpIdentity, ca bool) *otlpIdentity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return issueOTLPIdentityWithKey(t, dir, name, parent, ca, key)
}

func issueOTLPIdentityWithKey(t *testing.T, dir, name string, parent *otlpIdentity, ca bool, key crypto.Signer) *otlpIdentity {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		BasicConstraintsValid: true, IsCA: ca,
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if ca {
		template.KeyUsage |= x509.KeyUsageCertSign
	} else {
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}
	issuer, signer := template, key
	if parent != nil {
		issuer, signer = parent.certificate, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, key.Public(), signer)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	identity := &otlpIdentity{cert, key, pair, filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")}
	for path, data := range map[string][]byte{identity.certPath: certPEM, identity.keyPath: keyPEM} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return identity
}

func resetOTLPEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "OTEL_") {
			t.Setenv(key, "")
		}
	}
}

func TestPinnedOTLPEnvironmentSemantics(t *testing.T) {
	dir := t.TempDir()
	rootA := issueOTLPIdentity(t, dir, "root-a", nil, true)
	rootB := issueOTLPIdentity(t, dir, "root-b", nil, true)
	serverIdentity := issueOTLPIdentity(t, dir, "server", rootA, false)
	clientA := issueOTLPIdentity(t, dir, "client-a", rootA, false)
	clientB := issueOTLPIdentity(t, dir, "client-b", rootA, false)
	badPath := filepath.Join(dir, "invalid.pem")
	if err := os.WriteFile(badPath, []byte("invalid PEM\n"), 0600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dir, "missing.pem")
	cases := []struct {
		name                                      string
		env                                       map[string]string
		insecure, mtls, wantExport, wantInitError bool
		wantClient                                string
	}{
		{name: "generic-ca", env: map[string]string{"CERTIFICATE": rootA.certPath}, wantExport: true},
		{name: "traces-ca-overrides", env: map[string]string{"CERTIFICATE": rootB.certPath, "TRACES_CERTIFICATE": rootA.certPath}, wantExport: true},
		{name: "traces-ca-does-not-union", env: map[string]string{"CERTIFICATE": rootA.certPath, "TRACES_CERTIFICATE": rootB.certPath}},
		{name: "blank-traces-ca-falls-back", env: map[string]string{"CERTIFICATE": rootA.certPath, "TRACES_CERTIFICATE": " \t "}, wantExport: true},
		{name: "trimmed-ca-path", env: map[string]string{"CERTIFICATE": " \t" + rootA.certPath + "\n"}, wantExport: true},
		{name: "missing-traces-ca-falls-back", env: map[string]string{"CERTIFICATE": rootA.certPath, "TRACES_CERTIFICATE": missingPath}, wantExport: true},
		{name: "invalid-traces-ca-falls-back", env: map[string]string{"CERTIFICATE": rootA.certPath, "TRACES_CERTIFICATE": badPath}, wantExport: true},
		{name: "invalid-generic-valid-traces", env: map[string]string{"CERTIFICATE": badPath, "TRACES_CERTIFICATE": rootA.certPath}, wantExport: true},
		{name: "system-roots-reject-private", env: map[string]string{}},
		{name: "generic-client-pair", env: map[string]string{"CERTIFICATE": rootA.certPath, "CLIENT_CERTIFICATE": clientA.certPath, "CLIENT_KEY": clientA.keyPath}, mtls: true, wantExport: true, wantClient: "client-a"},
		{name: "traces-client-pair-overrides", env: map[string]string{"CERTIFICATE": rootA.certPath, "CLIENT_CERTIFICATE": clientA.certPath, "CLIENT_KEY": clientA.keyPath, "TRACES_CLIENT_CERTIFICATE": clientB.certPath, "TRACES_CLIENT_KEY": clientB.keyPath}, mtls: true, wantExport: true, wantClient: "client-b"},
		{name: "incomplete-traces-pair-falls-back", env: map[string]string{"CERTIFICATE": rootA.certPath, "CLIENT_CERTIFICATE": clientA.certPath, "CLIENT_KEY": clientA.keyPath, "TRACES_CLIENT_CERTIFICATE": clientB.certPath}, mtls: true, wantExport: true, wantClient: "client-a"},
		{name: "invalid-traces-pair-falls-back", env: map[string]string{"CERTIFICATE": rootA.certPath, "CLIENT_CERTIFICATE": clientA.certPath, "CLIENT_KEY": clientA.keyPath, "TRACES_CLIENT_CERTIFICATE": clientB.certPath, "TRACES_CLIENT_KEY": clientA.keyPath}, mtls: true, wantExport: true, wantClient: "client-a"},
		{name: "missing-client-half-not-mixed", env: map[string]string{"CERTIFICATE": rootA.certPath, "CLIENT_KEY": clientA.keyPath, "TRACES_CLIENT_CERTIFICATE": clientA.certPath}, mtls: true},
		{name: "explicit-http", insecure: true, wantExport: true},
		{name: "generic-insecure", env: map[string]string{"INSECURE": "true"}, wantExport: true},
		{name: "traces-insecure", env: map[string]string{"INSECURE": "false", "TRACES_INSECURE": " TrUe "}, wantExport: true},
		{name: "traces-false-overrides", env: map[string]string{"CERTIFICATE": rootA.certPath, "INSECURE": "true", "TRACES_INSECURE": "false"}, wantExport: true},
		{name: "boolean-other-is-false", env: map[string]string{"CERTIFICATE": rootA.certPath, "INSECURE": "true", "TRACES_INSECURE": "1"}, wantExport: true},
		{name: "http-endpoint", env: map[string]string{"ENDPOINT": "http://ignored.example/base"}, wantExport: true},
		{name: "traces-endpoint-https", env: map[string]string{"CERTIFICATE": rootA.certPath, "ENDPOINT": "http://ignored.example/base", "TRACES_ENDPOINT": "https://ignored.example/traces"}, wantExport: true},
		{name: "insecure-overrides-https-endpoint", env: map[string]string{"TRACES_ENDPOINT": "https://ignored.example/traces", "INSECURE": "true"}, wantExport: true},
		{name: "explicit-http-overrides-env-false", env: map[string]string{"TRACES_INSECURE": "false"}, insecure: true, wantExport: true},
		{name: "http-with-ca-rejected-at-start", env: map[string]string{"CERTIFICATE": rootA.certPath}, insecure: true, wantInitError: true},
	}
	for _, factory := range []struct {
		name   string
		create func(context.Context, Config) (sdktrace.SpanExporter, error)
	}{{"reference", referenceOTLPExporter}, {"policy", newOTLPExporter}} {
		for _, tc := range cases {
			t.Run(factory.name+"/"+tc.name, func(t *testing.T) {
				resetOTLPEnv(t)
				for key, value := range tc.env {
					t.Setenv("OTEL_EXPORTER_OTLP_"+key, value)
				}
				// These cases intentionally distinguish successful HTTP from HTTPS. A test
				// server's URL determines neither the library's scheme nor its env parsing.
				useHTTP := tc.insecure || tc.name == "generic-insecure" || tc.name == "traces-insecure" || tc.name == "http-endpoint" || tc.name == "insecure-overrides-https-endpoint"
				requests := make(chan string, 2)
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					client := ""
					if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
						client = r.TLS.PeerCertificates[0].Subject.CommonName
					}
					requests <- client
					w.Header().Set("Content-Type", "application/x-protobuf")
					w.WriteHeader(http.StatusOK)
				}))
				server.Config.ErrorLog = log.New(io.Discard, "", 0)
				if useHTTP {
					server.Start()
				} else {
					server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverIdentity.pair}}
					if tc.mtls {
						server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
						server.TLS.ClientCAs = x509.NewCertPool()
						server.TLS.ClientCAs.AddCert(rootA.certificate)
					}
					server.StartTLS()
				}
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				cfg := Config{OtlpEndpoint: strings.TrimPrefix(strings.TrimPrefix(server.URL, "https://"), "http://"), Insecure: tc.insecure}
				exporter, err := factory.create(ctx, cfg)
				if tc.wantInitError {
					if err == nil {
						t.Fatal("incompatible HTTP and TLS configuration was accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = exporter.Shutdown(context.Background()) }()
				span := tracetest.SpanStub{Name: "local-otlp-test", SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}}), StartTime: time.Now(), EndTime: time.Now()}.Snapshot()
				err = exporter.ExportSpans(ctx, []sdktrace.ReadOnlySpan{span})
				if tc.wantExport {
					if err != nil {
						t.Fatalf("export: %v", err)
					}
					select {
					case client := <-requests:
						if client != tc.wantClient {
							t.Fatalf("client=%q want=%q", client, tc.wantClient)
						}
					default:
						t.Fatal("no request received")
					}
				} else {
					if err == nil {
						t.Fatal("untrusted/missing client credentials unexpectedly exported")
					}
					select {
					case <-requests:
						t.Fatal("rejected export sent a request")
					default:
					}
				}
			})
		}
	}
}

func TestOTLPKeyStrength(t *testing.T) {
	dir := t.TempDir()
	strong := issueOTLPIdentity(t, dir, "strong-root", nil, true)
	weakKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	strongRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	_, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	weakRoot := issueOTLPIdentityWithKey(t, dir, "weak-root", nil, true, weakKey)
	strongServer := issueOTLPIdentity(t, dir, "strong-server", strong, false)
	weakServer := issueOTLPIdentityWithKey(t, dir, "weak-server", strong, false, weakKey)
	weakRootServer := issueOTLPIdentity(t, dir, "weak-root-server", weakRoot, false)
	weakClient := issueOTLPIdentityWithKey(t, dir, "weak-client", strong, false, weakKey)
	rsaClient := issueOTLPIdentityWithKey(t, dir, "rsa-client", strong, false, strongRSA)
	edClient := issueOTLPIdentityWithKey(t, dir, "ed-client", strong, false, ed)
	for _, tc := range []struct {
		name                 string
		server, root, client *otlpIdentity
		reject               bool
	}{
		{"weak-server", weakServer, strong, nil, true},
		{"weak-server-root", weakRootServer, weakRoot, nil, true},
		{"weak-client", strongServer, strong, weakClient, true},
		{"rsa-client", strongServer, strong, rsaClient, false},
		{"ed25519-client", strongServer, strong, edClient, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetOTLPEnv(t)
			t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", tc.root.certPath)
			if tc.client != nil {
				t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", tc.client.certPath)
				t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", tc.client.keyPath)
			}
			// The ordinary library accepts every fixture. Only the explicit minimum-key
			// policy should reject the weak fixtures, before any HTTP headers/body arrive.
			for _, factory := range []struct {
				name   string
				create func(context.Context, Config) (sdktrace.SpanExporter, error)
			}{{"reference", referenceOTLPExporter}, {"policy", newOTLPExporter}} {
				t.Run(factory.name, func(t *testing.T) {
					requests := make(chan struct{}, 2)
					server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests <- struct{}{}
						w.Header().Set("Content-Type", "application/x-protobuf")
					}))
					server.Config.ErrorLog = log.New(io.Discard, "", 0)
					server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{tc.server.pair}}
					if tc.client != nil {
						server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
						server.TLS.ClientCAs = x509.NewCertPool()
						server.TLS.ClientCAs.AddCert(strong.certificate)
					}
					server.StartTLS()
					defer server.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					exporter, err := factory.create(ctx, Config{OtlpEndpoint: strings.TrimPrefix(server.URL, "https://")})
					if err == nil {
						defer func() { _ = exporter.Shutdown(context.Background()) }()
						span := tracetest.SpanStub{Name: "key-policy", StartTime: time.Now(), EndTime: time.Now()}.Snapshot()
						err = exporter.ExportSpans(ctx, []sdktrace.ReadOnlySpan{span})
					}
					reject := tc.reject && factory.name == "policy"
					if reject {
						if !errors.Is(err, tlsclient.ErrCertificateKey) {
							t.Fatalf("expected key rejection: %v", err)
						}
						select {
						case <-requests:
							t.Fatal("rejected key sent request")
						default:
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						select {
						case <-requests:
						default:
							t.Fatal("no export request")
						}
					}
				})
			}
		})
	}
}
