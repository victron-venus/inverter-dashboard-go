package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func validTestSubscription(t *testing.T) webpush.Subscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return webpush.Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/example", Keys: webpush.Keys{
		P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth:   base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
	}}
}

func TestPushEndpointRejectsArbitraryTargets(t *testing.T) {
	for _, endpoint := range []string{
		"http://fcm.googleapis.com/push", "https://fcm.googleapis.com:8443/push",
		"https://fcm.googleapis.com.evil.example/push", "https://evil.example/?to=fcm.googleapis.com",
		"https://fcm.googleapis.com@127.0.0.1/", "https://user:password@fcm.googleapis.com/push",
		"https://127.0.0.1/push", "https://[::1]/push", "https://fcm.googleapis.com/push#fragment",
		"https://fcm.googleapis.com:/push", "https://bad_label.push.apple.com/path", "https://-bad.push.apple.com/path", "https://é.push.apple.com/path", "https://web.push.apple.com", " https://fcm.googleapis.com/push", "https://notify.windows.com.evil.example/",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := validateEndpoint(endpoint); err == nil {
				t.Fatal("unsafe endpoint accepted")
			}
		})
	}
	for _, endpoint := range []string{"https://fcm.googleapis.com/fcm/send/opaque", "https://updates.push.services.mozilla.com/wpush/v2/opaque", "https://web.push.apple.com/opaque", "https://wns2.notify.windows.com/?token=opaque"} {
		if _, err := validateEndpoint(endpoint); err != nil {
			t.Fatalf("known provider rejected: %v", err)
		}
	}
}

func TestPushSubscriptionKeyValidation(t *testing.T) {
	s := validTestSubscription(t)
	if err := validateSubscription(s); err != nil {
		t.Fatal(err)
	}
	s.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 15))
	if validateSubscription(s) == nil {
		t.Fatal("short auth accepted")
	}
	s = validTestSubscription(t)
	s.Keys.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	if validateSubscription(s) == nil {
		t.Fatal("off-curve key accepted")
	}
}

func TestPushDialRejectsPrivateAndMixedDNSWithoutDialing(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2001:db8::1", "198.18.0.1", "0.0.0.0", "224.0.0.1", "3fff::1", "64:ff9b:1::a00:1", "4000::1"} {
		t.Run(address, func(t *testing.T) {
			calls := 0
			dial := validatedDial(func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(address)}, nil
			}, func(context.Context, string, string) (net.Conn, error) {
				calls++
				return nil, errors.New("must not dial")
			})
			if _, err := dial(context.Background(), "tcp", "fcm.googleapis.com:443"); err == nil || calls != 0 {
				t.Fatal("prohibited DNS result reached dialer")
			}
		})
	}
}

func TestPushDialPinsValidatedIPAndRetainsContext(t *testing.T) {
	lookupCalls, dialCalls := 0, 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dial := validatedDial(func(got context.Context, network, host string) ([]netip.Addr, error) {
		lookupCalls++
		if got != ctx || host != "fcm.googleapis.com" || network != "ip" {
			t.Fatal("unexpected lookup")
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}, func(got context.Context, network, address string) (net.Conn, error) {
		dialCalls++
		if got != ctx || address != "8.8.8.8:443" || network != "tcp" {
			t.Fatal("DNS was not pinned")
		}
		return nil, errors.New("test-only refusal")
	})
	if _, err := dial(ctx, "tcp", "fcm.googleapis.com:443"); err == nil || lookupCalls != 1 || dialCalls != 1 {
		t.Fatal("unexpected dial outcome")
	}
}

func TestPushClientHasNoProxyOrRedirect(t *testing.T) {
	client := newPushHTTPClient()
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil || client.Timeout <= 0 || transport.ResponseHeaderTimeout <= 0 || transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("unsafe HTTP client")
	}
	if err := client.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatal("redirect allowed")
	}
}
