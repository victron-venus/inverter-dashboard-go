package gateway

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewClientRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"", "gateway.example", "//gateway.example", "http://gateway.example",
		"http://127.0.0.1:9150", "https:///path", "https://:443", "https:gateway.example",
		"https://gateway.example:0", "https://gateway.example:65536", "https://gateway.example:",
		"https://gateway.example:invalid", "https://user:do-not-log@gateway.example",
		"https://gateway.example?token=do-not-log", "https://gateway.example?",
		"https://gateway.example#do-not-log", "https://gateway.example#",
		"https://gateway.example\n/do-not-log", "https://gateway.example/%zz-do-not-log",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := NewClient(Config{URL: raw, APIToken: "token"}, nil, nil)
			if err == nil {
				t.Fatal("unsafe URL accepted")
			}
			if strings.Contains(err.Error(), "do-not-log") {
				t.Fatal("validation error exposed URL credentials")
			}
		})
	}
}

func TestNewClientRejectsIncompleteCredentials(t *testing.T) {
	for _, cfg := range []Config{
		{}, {AccessClientID: "id"}, {AccessClientSecret: "secret"},
		{APIToken: "token", AccessClientID: "id"},
		{APIToken: "token", AccessClientSecret: "secret"},
		{APIToken: " ", AccessClientID: " ", AccessClientSecret: " "},
	} {
		cfg.URL = "https://igw.example:9151"
		if _, err := NewClient(cfg, nil, nil); err == nil {
			t.Fatal("incomplete credentials accepted")
		}
	}
}

func TestHTTPSAuthenticationAndAPIContract(t *testing.T) {
	for _, auth := range []struct {
		name string
		cfg  Config
	}{
		{"native bearer", Config{APIToken: "token"}},
		{"cloudflare and bearer", Config{APIToken: "token", AccessClientID: "id.access", AccessClientSecret: "secret"}},
		{"cloudflare only", Config{AccessClientID: "id.access", AccessClientSecret: "secret"}},
	} {
		t.Run(auth.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.TLS == nil {
					t.Error("request did not use TLS")
				}
				for name, want := range map[string]string{
					"CF-Access-Client-Id":     auth.cfg.AccessClientID,
					"CF-Access-Client-Secret": auth.cfg.AccessClientSecret,
				} {
					if r.Header.Get(name) != want || (want == "" && len(r.Header.Values(name)) != 0) {
						t.Errorf("unexpected presence/value of %s", name)
					}
				}
				wantBearer := ""
				if auth.cfg.APIToken != "" {
					wantBearer = "Bearer " + auth.cfg.APIToken
				}
				if r.Header.Get("Authorization") != wantBearer {
					t.Error("unexpected bearer authorization")
				}
				if r.Header.Get("User-Agent") != "inverter-dashboard-go/gateway" {
					t.Error("gateway User-Agent changed")
				}
				switch r.URL.Path {
				case "/prefix/v1/snapshot":
					if r.Method != http.MethodGet {
						t.Errorf("snapshot method = %s", r.Method)
					}
					_, _ = io.WriteString(w, `{"battery":{"0/Soc":42}}`)
				case "/prefix/v1/commands/silence_alarm":
					body, err := io.ReadAll(r.Body)
					if err != nil || string(body) != "{}" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
						t.Error("command method, body, or content type changed")
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected API path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			auth.cfg.URL = "  " + server.URL + "/prefix///  "
			client, err := NewClient(auth.cfg, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Trust only the fixture certificate; retain production redirect policy.
			client.http.Transport = server.Client().Transport
			snapshot, err := client.FetchSnapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if string(snapshot.Battery["0/Soc"]) != "42" {
				t.Fatal("snapshot payload changed")
			}
			if err := client.PostCommand(context.Background(), "silence_alarm", nil); err != nil {
				t.Fatal(err)
			}
			if err := client.PostCommand(context.Background(), "setpoint", nil); !errors.Is(err, ErrCommandNotOnGateway) {
				t.Fatal("non-whitelisted command was accepted")
			}
			if requests.Load() != 2 {
				t.Fatalf("received %d requests, want snapshot and allowed command only", requests.Load())
			}
		})
	}
}

func TestAuthenticatedRequestsNeverFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, destination := range []string{"same origin", "other HTTPS origin", "HTTP downgrade"} {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				t.Run(fmt.Sprintf("%d/%s/%s", status, destination, method), func(t *testing.T) {
					var targetRequests, originRequests atomic.Int32
					targetHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						targetRequests.Add(1)
						_, _ = io.WriteString(w, "{}")
					})
					target := httptest.NewTLSServer(targetHandler)
					defer target.Close()
					plain := httptest.NewServer(targetHandler)
					defer plain.Close()
					location := "/redirected"
					switch destination {
					case "other HTTPS origin":
						location = target.URL + "/redirected"
					case "HTTP downgrade":
						location = plain.URL + "/redirected"
					}
					origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/redirected" {
							targetHandler.ServeHTTP(w, r)
							return
						}
						originRequests.Add(1)
						if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("CF-Access-Client-Id") != "id.access" || r.Header.Get("CF-Access-Client-Secret") != "secret" {
							t.Error("original request was not authenticated")
						}
						w.Header().Set("Location", location)
						w.WriteHeader(status)
					}))
					defer origin.Close()
					client, err := NewClient(Config{URL: origin.URL, APIToken: "token", AccessClientID: "id.access", AccessClientSecret: "secret"}, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					client.http.Transport = origin.Client().Transport
					if method == http.MethodGet {
						_, err = client.FetchSnapshot(context.Background())
					} else {
						err = client.PostCommand(context.Background(), "silence_alarm", map[string]bool{"value": true})
					}
					if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
						t.Fatalf("expected redirect status failure, got %v", err)
					}
					if targetRequests.Load() != 0 || originRequests.Load() != 1 {
						t.Fatalf("redirect followed: origin=%d target=%d", originRequests.Load(), targetRequests.Load())
					}
				})
			}
		}
	}
}

func TestTLSVerificationCannotBeSkipped(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "{}")
	}))
	defer server.Close()
	client, err := NewClient(Config{URL: server.URL, APIToken: "token"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.FetchSnapshot(context.Background())
	var unknownCA x509.UnknownAuthorityError
	if !errors.As(err, &unknownCA) {
		t.Fatalf("untrusted certificate accepted or wrong error: %v", err)
	}
	client.cfg.URL = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	client.http.Transport = server.Client().Transport
	_, err = client.FetchSnapshot(context.Background())
	var hostname x509.HostnameError
	if !errors.As(err, &hostname) {
		t.Fatalf("wrong certificate hostname accepted or wrong error: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("credentials reached an endpoint with an invalid certificate")
	}
}
