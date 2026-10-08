package main

import (
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
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/victron-venus/inverter-dashboard-go/internal/config"
	"github.com/victron-venus/inverter-dashboard-go/internal/logging"
	"github.com/victron-venus/inverter-dashboard-go/internal/tlsclient"
)

func httpsRSA(t *testing.T, bits int) crypto.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func httpsEC(t *testing.T) crypto.Signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func httpsEd(t *testing.T) crypto.Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

type httpsFixture struct {
	certFile, keyFile string
	roots             *x509.CertPool
	leaf              []byte
}

func httpsIdentity(t *testing.T, rootKey, middleKey, leafKey crypto.Signer) httpsFixture {
	t.Helper()
	now := time.Now()
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, rootKey.Public(), rootKey)
	if err != nil {
		t.Fatal(err)
	}
	root, err = x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	parent, signer := root, rootKey
	chain := [][]byte{rootDER}
	if middleKey != nil {
		middle := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "test intermediate"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
		der, err := x509.CreateCertificate(rand.Reader, middle, root, middleKey.Public(), rootKey)
		if err != nil {
			t.Fatal(err)
		}
		parent, err = x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		signer = middleKey
		chain = append([][]byte{der}, chain...)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, parent, leafKey.Public(), signer)
	if err != nil {
		t.Fatal(err)
	}
	chain = append([][]byte{leafDER}, chain...)
	var certPEM []byte
	for _, der := range chain {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "server.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	return httpsFixture{certFile, keyFile, roots, leafDER}
}

func httpsRequest(t *testing.T, server *http.Server, roots *x509.CertPool, version uint16) {
	t.Helper()
	hosted := httptest.NewUnstartedServer(server.Handler)
	hosted.Config, hosted.TLS = server, server.TLSConfig.Clone()
	hosted.StartTLS()
	defer hosted.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: version, MaxVersion: version}}, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Get(hosted.URL + "/proof")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "same gin handler" || resp.StatusCode != 200 || len(resp.TLS.VerifiedChains) == 0 {
		t.Fatalf("unexpected verified response: %d %q", resp.StatusCode, body)
	}
}

func httpsHandler() http.Handler {
	engine := gin.New()
	engine.GET("/proof", func(c *gin.Context) { c.String(http.StatusOK, "same gin handler") })
	return engine.Handler()
}

func TestHTTPSServerRejectsUndersizedKeysBeforeListen(t *testing.T) {
	strong, weak := httpsRSA(t, 2048), httpsRSA(t, 1024)
	for _, tc := range []struct {
		name               string
		root, middle, leaf crypto.Signer
	}{
		{"leaf", strong, nil, weak}, {"intermediate", strong, weak, strong}, {"root", weak, nil, strong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := httpsIdentity(t, tc.root, tc.middle, tc.leaf)
			server, err := newHTTPSServer(httpsHandler(), "127.0.0.1:0", fixture.certFile, fixture.keyFile)
			if !errors.Is(err, tlsclient.ErrCertificateKey) || server != nil {
				t.Fatalf("weak configured chain accepted: server=%v error=%v", server, err)
			}
		})
	}
}

func TestHTTPSServerStrongKeysAndStandardVerification(t *testing.T) {
	root := httpsRSA(t, 2048)
	for _, tc := range []struct {
		name string
		key  crypto.Signer
	}{
		{"RSA2048", httpsRSA(t, 2048)}, {"P256", httpsEC(t)}, {"Ed25519", httpsEd(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := httpsIdentity(t, root, nil, tc.key)
			for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
				server, err := newHTTPSServer(httpsHandler(), "127.0.0.1:0", fixture.certFile, fixture.keyFile)
				if err != nil {
					t.Fatal(err)
				}
				httpsRequest(t, server, fixture.roots, version)
			}
		})
	}
}

func TestHTTPSServerKeepsValidatedPairAfterFilesChange(t *testing.T) {
	key := httpsRSA(t, 2048)
	fixture := httpsIdentity(t, key, nil, key)
	server, err := newHTTPSServer(httpsHandler(), "127.0.0.1:0", fixture.certFile, fixture.keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.certFile, []byte("replaced after validation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.keyFile); err != nil {
		t.Fatal(err)
	}
	httpsRequest(t, server, fixture.roots, tls.VersionTLS13)
}

func TestHTTPSServerRejectsMalformedAndMismatchedPair(t *testing.T) {
	key := httpsRSA(t, 2048)
	fixture := httpsIdentity(t, key, nil, key)
	other := httpsIdentity(t, key, nil, httpsRSA(t, 2048))
	server, err := newHTTPSServer(httpsHandler(), "127.0.0.1:0", fixture.certFile, other.keyFile)
	if err == nil || server != nil {
		t.Fatal("mismatched pair accepted")
	}
	if err := os.WriteFile(fixture.certFile, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	server, err = newHTTPSServer(httpsHandler(), "127.0.0.1:0", fixture.certFile, fixture.keyFile)
	if err == nil || server != nil {
		t.Fatal("malformed pair accepted")
	}
}

func TestStartServerRejectsWeakPairBeforeBinding(t *testing.T) {
	if os.Getenv("INVERTER_TEST_HTTPS_KEY_POLICY") == "child" {
		port, err := strconv.Atoi(os.Getenv("INVERTER_TEST_HTTPS_PORT"))
		if err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{}
		cfg.Web.Host = "127.0.0.1"
		cfg.Web.Port = port
		startServer(gin.New(), cfg, os.Getenv("INVERTER_TEST_HTTPS_CERT"), os.Getenv("INVERTER_TEST_HTTPS_KEY"), logging.New("test", "test", slog.LevelError))
		t.Fatal("weak HTTPS startup unexpectedly returned")
	}
	strong, weak := httpsRSA(t, 2048), httpsRSA(t, 1024)
	fixture := httpsIdentity(t, strong, nil, weak)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestStartServerRejectsWeakPairBeforeBinding$")
	command.Env = append(os.Environ(), "INVERTER_TEST_HTTPS_KEY_POLICY=child", "INVERTER_TEST_HTTPS_PORT="+strconv.Itoa(occupied.Addr().(*net.TCPAddr).Port), "INVERTER_TEST_HTTPS_CERT="+fixture.certFile, "INVERTER_TEST_HTTPS_KEY="+fixture.keyFile)
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("expected startup exit1, got %v: %s", err, output)
	}
	if !strings.Contains(string(output), tlsclient.ErrCertificateKey.Error()) || strings.Contains(string(output), "address already in use") {
		t.Fatalf("certificate was not rejected before attempting bind: %s", output)
	}
}
