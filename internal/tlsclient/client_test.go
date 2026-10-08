package tlsclient

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
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func ecKey(t *testing.T, curve elliptic.Curve) crypto.Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func rsaKey(t *testing.T, bits int) crypto.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func issue(t *testing.T, name string, key crypto.Signer, parent *x509.Certificate, parentKey crypto.Signer, ca bool) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		BasicConstraintsValid: true, IsCA: ca, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if ca {
		template.KeyUsage |= x509.KeyUsageCertSign
	} else {
		template.DNSNames = []string{"local.example"}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func configs(t *testing.T, rootKey, intermediateKey, leafKey crypto.Signer) (*tls.Config, *tls.Config) {
	t.Helper()
	root := issue(t, "root", rootKey, nil, nil, true)
	intermediate := issue(t, "intermediate", intermediateKey, root, rootKey, true)
	leaf := issue(t, "leaf", leafKey, intermediate, intermediateKey, false)
	client := NewConfig()
	client.ServerName = "local.example"
	client.RootCAs = x509.NewCertPool()
	client.RootCAs.AddCert(root)
	server := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{
		Certificate: [][]byte{leaf.Raw, intermediate.Raw, root.Raw}, PrivateKey: leafKey,
	}}}
	return client, server
}

func handshake(t *testing.T, clientConfig, serverConfig *tls.Config) (tls.ConnectionState, error) {
	t.Helper()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if err := a.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := b.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	server, client := tls.Server(a, serverConfig), tls.Client(b, clientConfig)
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { done <- server.HandshakeContext(ctx) }()
	err := client.HandshakeContext(ctx)
	if err != nil {
		_ = b.Close()
	}
	serverErr := <-done
	if err == nil && serverErr != nil {
		t.Fatalf("server handshake: %v", serverErr)
	}
	return client.ConnectionState(), err
}

func TestTLSVerifiedChainKeyStrength(t *testing.T) {
	strong := ecKey(t, elliptic.P256())
	weakRSA, strongRSA := rsaKey(t, 1024), rsaKey(t, 2048)
	_, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                     string
		root, intermediate, leaf crypto.Signer
		wantErr                  bool
	}{
		{"rsa2048", strongRSA, strongRSA, strongRSA, false},
		{"ecdsa256", strong, strong, strong, false},
		{"ecdsa384", strong, strong, ecKey(t, elliptic.P384()), false},
		{"ecdsa521", strong, strong, ecKey(t, elliptic.P521()), false},
		{"ed25519", ed, ed, ed, false},
		{"ecdsa224_ca", ecKey(t, elliptic.P224()), strong, strong, false},
		{"rsa1024_leaf", strong, strong, weakRSA, true},
		{"rsa1024_intermediate", strong, weakRSA, strong, true},
		{"rsa1024_root", weakRSA, strong, strong, true},
	}
	for _, tc := range cases {
		for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
			t.Run(tc.name+"/"+tls.VersionName(version), func(t *testing.T) {
				client, server := configs(t, tc.root, tc.intermediate, tc.leaf)
				client.MinVersion, client.MaxVersion = version, version
				state, err := handshake(t, client, server)
				if tc.wantErr {
					if !errors.Is(err, ErrCertificateKey) {
						t.Fatalf("wanted key-strength rejection, got %v", err)
					}
				} else if err != nil || len(state.VerifiedChains) == 0 {
					t.Fatalf("trusted supported chain failed: %v", err)
				}
			})
		}
	}
}

func TestStandardTrustAndHostnameChecksStillRun(t *testing.T) {
	key := ecKey(t, elliptic.P256())
	for _, mode := range []string{"hostname", "untrusted"} {
		t.Run(mode, func(t *testing.T) {
			client, server := configs(t, key, key, key)
			if mode == "hostname" {
				client.ServerName = "wrong.example"
			} else {
				client.RootCAs = x509.NewCertPool()
			}
			called := false
			client.VerifyConnection = func(state tls.ConnectionState) error { called = true; return verifyConnection(state) }
			_, err := handshake(t, client, server)
			var verification *tls.CertificateVerificationError
			if !errors.As(err, &verification) || called {
				t.Fatalf("normal verification must fail before callback: %v, called=%v", err, called)
			}
		})
	}
}

func TestStrongAlternateVerifiedChainIsAccepted(t *testing.T) {
	strong, weak := ecKey(t, elliptic.P256()), rsaKey(t, 1024)
	goodRoot := issue(t, "strong root", strong, nil, nil, true)
	weakRoot := issue(t, "weak root", weak, nil, nil, true)
	intermediateKey := ecKey(t, elliptic.P256())
	goodIntermediate := issue(t, "intermediate", intermediateKey, goodRoot, strong, true)
	weakIntermediate := issue(t, "intermediate", intermediateKey, weakRoot, weak, true)
	leaf := issue(t, "leaf", strong, goodIntermediate, intermediateKey, false)
	client := NewConfig()
	client.ServerName, client.RootCAs = "local.example", x509.NewCertPool()
	client.RootCAs.AddCert(goodRoot)
	client.RootCAs.AddCert(weakRoot)
	server := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{PrivateKey: strong,
		Certificate: [][]byte{leaf.Raw, weakIntermediate.Raw, goodIntermediate.Raw},
	}}}
	state, err := handshake(t, client, server)
	if err != nil || len(state.VerifiedChains) < 2 {
		t.Fatalf("alternate chain rejected or not exercised: %v, chains=%d", err, len(state.VerifiedChains))
	}
}

func TestResumedWeakChainIsRejected(t *testing.T) {
	strong, weak := ecKey(t, elliptic.P256()), rsaKey(t, 1024)
	client, server := configs(t, strong, strong, weak)
	client.MaxVersion, server.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	client.ClientSessionCache = tls.NewLRUClientSessionCache(1)
	client.VerifyConnection = nil // Model a session established before the stricter policy.
	if _, err := handshake(t, client, server); err != nil {
		t.Fatal(err)
	}
	resumed := false
	client.VerifyConnection = func(state tls.ConnectionState) error { resumed = state.DidResume; return verifyConnection(state) }
	_, err := handshake(t, client, server)
	if !resumed || !errors.Is(err, ErrCertificateKey) {
		t.Fatalf("resumed weak session not rejected: resumed=%v, err=%v", resumed, err)
	}
}

func TestMissingOrUnsupportedVerifiedKeysFailClosed(t *testing.T) {
	for _, state := range []tls.ConnectionState{
		{}, {VerifiedChains: [][]*x509.Certificate{{}}}, {VerifiedChains: [][]*x509.Certificate{{nil}}},
		{VerifiedChains: [][]*x509.Certificate{{{PublicKey: "unsupported"}}}},
		{VerifiedChains: [][]*x509.Certificate{{{PublicKey: ed25519.PublicKey{1, 2}}}}},
	} {
		if !errors.Is(verifyConnection(state), ErrCertificateKey) {
			t.Fatal("invalid verified chain accepted")
		}
	}
}

type unusableTransport struct{}

func (unusableTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("global transport must not be used")
}

func TestOwnedTransportPreservesPlainHTTP(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = unusableTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	transport := NewTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", response.StatusCode)
	}
}

func TestLocalClientCertificateChainKeys(t *testing.T) {
	strong, weak := ecKey(t, elliptic.P256()), rsaKey(t, 1024)
	for _, tc := range []struct {
		name                     string
		root, intermediate, leaf crypto.Signer
		reject                   bool
	}{
		{"strong", strong, strong, strong, false},
		{"weak-leaf", strong, strong, weak, true},
		{"weak-intermediate", strong, weak, strong, true},
		{"weak-root", weak, strong, strong, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, server := configs(t, tc.root, tc.intermediate, tc.leaf)
			err := ValidateLocalCertificate(server.Certificates[0])
			if tc.reject {
				if !errors.Is(err, ErrCertificateKey) {
					t.Fatalf("expected key rejection: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, certificate := range []tls.Certificate{{}, {Certificate: [][]byte{[]byte("invalid DER")}}} {
		if !errors.Is(ValidateLocalCertificate(certificate), ErrCertificateKey) {
			t.Fatal("malformed local chain was accepted")
		}
	}
}
