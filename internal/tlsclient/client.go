// Package tlsclient supplies the application's verified outbound TLS policy.
package tlsclient

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"time"
)

// ErrCertificateKey indicates that no verified chain meets the key-strength
// minimum. It deliberately omits certificate names and request destinations.
var ErrCertificateKey = errors.New("TLS certificate chain does not meet minimum key strength")

// NewConfig preserves normal trust-chain and hostname verification. The extra
// callback checks the already verified chains, including resumed connections.
func NewConfig() *tls.Config {
	return &tls.Config{
		MinVersion:       tls.VersionTLS12,
		VerifyConnection: verifyConnection,
	}
}

func verifyConnection(state tls.ConnectionState) error {
	for _, chain := range state.VerifiedChains {
		if strongChain(chain) {
			return nil
		}
	}
	return ErrCertificateKey
}

func strongChain(chain []*x509.Certificate) bool {
	if len(chain) == 0 {
		return false
	}
	for _, cert := range chain {
		if cert == nil || !strongPublicKey(cert.PublicKey) {
			return false
		}
	}
	return true
}

func strongPublicKey(key any) bool {
	switch key := key.(type) {
	case *rsa.PublicKey:
		return key != nil && key.N != nil && key.N.BitLen() >= 2048
	case *ecdsa.PublicKey:
		return key != nil && key.Curve != nil && key.Curve.Params().BitSize >= 224
	case ed25519.PublicKey:
		return len(key) == ed25519.PublicKeySize
	default:
		return false
	}
}

// NewTransport uses Go's standard HTTP transport defaults with our TLS policy.
// It owns its configuration; it neither mutates nor type-asserts the replaceable
// process-wide http.DefaultTransport.
func NewTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       NewConfig(),
	}
}
