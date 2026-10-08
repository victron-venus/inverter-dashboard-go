package main

import (
	"crypto/tls"
	"net/http"

	"github.com/victron-venus/inverter-dashboard-go/internal/tlsclient"
)

// newHTTPSServer validates the complete configured certificate chain before a
// listener is opened. Keep the loaded pair so serving cannot reload unchecked
// replacement files after validation.
func newHTTPSServer(handler http.Handler, addr, certFile, keyFile string) (*http.Server, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	if err := tlsclient.ValidateLocalCertificate(certificate); err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:    addr,
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
		},
	}, nil
}
