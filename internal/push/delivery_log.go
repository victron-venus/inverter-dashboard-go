package push

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

var (
	errNotificationRetired = errors.New("notification retired")
	errNotificationExpired = errors.New("notification expired")
)

// Log only bounded enums and integers. A provider response is not proof that the
// browser displayed an OS notification, and an error may contain its secret URL.
func (d *Dispatcher) logAttempt(item delivery, status int, err error) {
	kind := item.Payload.Kind
	switch kind {
	case "native", "ev", "water", "lowBattery", "test":
	default:
		kind = "unknown"
	}
	attempt := item.Attempts + 1
	if attempt < 1 || attempt > 3 {
		attempt = 0
	}
	if status < 100 || status > 599 {
		status = 0
	}
	d.logger.Info("Web Push attempt completed", "kind", kind, "attempt", attempt, "http_status", status, "error_category", deliveryErrorCategory(err))
}

func deliveryErrorCategory(err error) string {
	for _, value := range []struct {
		err      error
		category string
	}{
		{nil, "none"},
		{errNotificationRetired, "retired"},
		{errNotificationExpired, "expired"},
		{context.Canceled, "cancelled"},
		{context.DeadlineExceeded, "timeout"},
		{errPushDNSProhibited, "destination_blocked"},
		{errPushEndpoint, "destination_blocked"},
		{errPushDNS, "dns"},
		{errPushConnection, "connection"},
	} {
		if errors.Is(err, value.err) {
			return value.category
		}
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return "dns"
	}
	var verifyError *tls.CertificateVerificationError
	var headerError tls.RecordHeaderError
	var certificateError x509.CertificateInvalidError
	var authorityError x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	if errors.As(err, &verifyError) || errors.As(err, &headerError) || errors.As(err, &certificateError) || errors.As(err, &authorityError) || errors.As(err, &hostnameError) {
		return "tls"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) {
		return "connection"
	}
	return "other"
}
