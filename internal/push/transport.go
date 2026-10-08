// Package push implements opt-in, persistent Web Push notifications.
package push

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/tlsclient"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const maxEndpointBytes = 2048

var errPushEndpoint = errors.New("invalid push service endpoint")

var (
	errPushDNS           = errors.New("push service DNS unavailable")
	errPushDNSProhibited = errors.New("push service DNS returned a prohibited address")
	errPushConnection    = errors.New("push service connection failed")
)

// Provider domains are not user-configurable. A subscription is a destination
// capability, not permission to make requests to arbitrary network services.
func knownProvider(host string) bool {
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return host == "fcm.googleapis.com" ||
		host == "updates.push.services.mozilla.com" ||
		strings.HasSuffix(host, ".push.apple.com") ||
		strings.HasSuffix(host, ".notify.windows.com")
}

func validateEndpoint(endpoint string) (*url.URL, error) {
	if endpoint == "" || len(endpoint) > maxEndpointBytes || strings.TrimSpace(endpoint) != endpoint {
		return nil, errPushEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Fragment != "" || strings.Contains(endpoint, "#") || u.Path == "" {
		return nil, errPushEndpoint
	}
	host := u.Hostname()
	if host == "" || host != strings.ToLower(host) || !knownProvider(host) || (u.Port() != "" && u.Port() != "443") || strings.HasSuffix(u.Host, ":") {
		return nil, errPushEndpoint
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil, errPushEndpoint
	}
	return u, nil
}

func validateSubscription(s webpush.Subscription) error {
	if _, err := validateEndpoint(s.Endpoint); err != nil {
		return err
	}
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return errors.New("invalid push authentication key")
	}
	key, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil || len(key) != 65 {
		return errors.New("invalid push public key")
	}
	if _, err := ecdh.P256().NewPublicKey(key); err != nil {
		return errors.New("invalid push public key")
	}
	return nil
}

// In addition to Go's private/loopback helpers, exclude special-purpose and
// translation ranges. Pin the validated result when dialing to avoid a second
// DNS lookup (including DNS rebinding to private infrastructure).
var blockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("3fff::/20"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.Zone() != "" {
		return false
	}
	for _, network := range blockedNetworks {
		if network.Contains(address) {
			return false
		}
	}
	return true
}

type lookupIP func(context.Context, string, string) ([]netip.Addr, error)
type dialIP func(context.Context, string, string) (net.Conn, error)

func validatedDial(lookup lookupIP, dial dialIP) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || !knownProvider(host) {
			return nil, errPushEndpoint
		}
		addresses, err := lookup(ctx, "ip", host)
		if err != nil || len(addresses) == 0 || len(addresses) > 32 {
			return nil, errPushDNS
		}
		for _, ip := range addresses {
			if !publicAddress(ip) {
				return nil, errPushDNSProhibited
			}
		}
		for _, ip := range addresses {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			if ctx.Err() != nil {
				break
			}
		}
		return nil, errPushConnection
	}
}

func newPushHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:                  nil,
			DialContext:            validatedDial(net.DefaultResolver.LookupNetIP, dialer.DialContext),
			TLSClientConfig:        tlsclient.NewConfig(),
			TLSHandshakeTimeout:    5 * time.Second,
			ResponseHeaderTimeout:  5 * time.Second,
			MaxResponseHeaderBytes: 16 << 10,
			MaxConnsPerHost:        2,
			MaxIdleConns:           8,
			IdleConnTimeout:        30 * time.Second,
			ForceAttemptHTTP2:      true,
		},
	}
}

// Browser key encodings are equivalent with or without URL-base64 padding.
func canonicalSubscription(sub webpush.Subscription) (webpush.Subscription, error) {
	decode := func(raw string) (string, error) {
		if len(raw) > 128 {
			return "", errPushEndpoint
		}
		b, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			b, err = base64.URLEncoding.DecodeString(raw)
		}
		if err != nil {
			return "", errPushEndpoint
		}
		return base64.RawURLEncoding.EncodeToString(b), nil
	}
	var err error
	sub.Keys.P256dh, err = decode(sub.Keys.P256dh)
	if err != nil {
		return sub, err
	}
	sub.Keys.Auth, err = decode(sub.Keys.Auth)
	if err != nil {
		return sub, err
	}
	return sub, validateSubscription(sub)
}
