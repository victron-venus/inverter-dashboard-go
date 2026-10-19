package gateway

import (
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

type recoveryConnectionKey struct{}

// Use the production transport, including TLS verification and HTTP/2, with
// only the test server's certificate added to its trust roots.
func newRecoveryClient(t *testing.T, handler http.Handler, apply func(*state.State), status func(bool)) (*Client, *atomic.Int32) {
	t.Helper()
	connections := new(atomic.Int32)
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.Config.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
		return context.WithValue(ctx, recoveryConnectionKey{}, connections.Add(1))
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	client, err := NewClient(Config{URL: server.URL, APIToken: "test", PollInterval: 100 * time.Millisecond}, apply, status)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.http.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	client.http.Timeout = 150 * time.Millisecond
	t.Cleanup(client.Stop)
	return client, connections
}

func TestPollRecoversOnNewHTTP2ConnectionAfterTimeout(t *testing.T) {
	for _, tc := range []struct {
		name           string
		partialBody    bool
		initialFailure bool
	}{
		{name: "response headers"},
		{name: "response body", partialBody: true},
		{name: "initial poll", initialFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var firstConnectionRequests atomic.Int32
			var wrongProtocol atomic.Bool
			applied := make(chan struct{}, 1)
			disconnected := make(chan struct{}, 1)
			client, connections := newRecoveryClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 {
					wrongProtocol.Store(true)
				}
				id := r.Context().Value(recoveryConnectionKey{}).(int32)
				if id == 1 && (tc.initialFailure || firstConnectionRequests.Add(1) > 1) {
					// This flow remains unusable, while any new connection works.
					// A request timeout alone must not count as recovery.
					if tc.partialBody {
						_, _ = io.WriteString(w, `{"meta":`)
						w.(http.Flusher).Flush()
					}
					<-r.Context().Done()
					return
				}
				_, _ = io.WriteString(w, `{}`)
			}), func(*state.State) {
				select {
				case applied <- struct{}{}:
				default:
				}
			}, func(connected bool) {
				if !connected {
					select {
					case disconnected <- struct{}{}:
					default:
					}
				}
			})
			client.Start()
			if !tc.initialFailure {
				awaitRecoveryEvent(t, applied, "initial snapshot")
			}
			awaitRecoveryEvent(t, disconnected, "failed poll")
			awaitRecoveryEvent(t, applied, "snapshot on a new connection")
			client.Stop()
			if connections.Load() < 2 || wrongProtocol.Load() {
				t.Fatalf("recovery must use a new HTTP/2 connection: connections=%d wrongProtocol=%v", connections.Load(), wrongProtocol.Load())
			}
		})
	}
}

func TestStopDuringPollRecoveryWait(t *testing.T) {
	disconnected := make(chan struct{}, 1)
	client, _ := newRecoveryClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}), nil, func(connected bool) {
		if !connected {
			disconnected <- struct{}{}
		}
	})
	client.cfg.PollInterval = time.Hour
	client.Start()
	awaitRecoveryEvent(t, disconnected, "initial poll failure")
	stopped := make(chan struct{})
	go func() { client.Stop(); close(stopped) }()
	awaitRecoveryEvent(t, stopped, "Stop during recovery wait")
}

func awaitRecoveryEvent(t *testing.T, event <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func TestPollApplicationErrorsKeepHTTP2Connection(t *testing.T) {
	for _, fault := range []string{"unauthorized", "unavailable", "invalid JSON"} {
		t.Run(fault, func(t *testing.T) {
			var requests atomic.Int32
			client, connections := newRecoveryClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 2 {
					switch fault {
					case "unauthorized":
						w.WriteHeader(http.StatusUnauthorized)
					case "unavailable":
						w.WriteHeader(http.StatusServiceUnavailable)
					default:
						_, _ = io.WriteString(w, `not JSON`)
					}
					return
				}
				_, _ = io.WriteString(w, `{}`)
			}), nil, nil)
			client.pollOnce()
			if !client.IsConnected() {
				t.Fatal("initial poll failed")
			}
			client.pollOnce()
			if client.IsConnected() {
				t.Fatal("application error was treated as a valid snapshot")
			}
			client.pollOnce()
			if !client.IsConnected() || connections.Load() != 1 {
				t.Fatalf("application error should preserve the working connection: connected=%v connections=%d", client.IsConnected(), connections.Load())
			}
		})
	}
}

func TestPollRecoveryDoesNotInterruptOrReplayCommand(t *testing.T) {
	var commands atomic.Int32
	commandStarted := make(chan struct{})
	finishCommand := make(chan struct{})
	defer func() {
		select {
		case <-finishCommand:
		default:
			close(finishCommand)
		}
	}()
	commandCancelled := make(chan struct{}, 1)
	client, _ := newRecoveryClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if commands.Add(1) == 1 {
				close(commandStarted)
			}
			select {
			case <-finishCommand:
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
				commandCancelled <- struct{}{}
			}
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}), nil, nil)
	transport := &failingSnapshotTransport{Transport: client.http.Transport.(*http.Transport)}
	client.http.Transport = transport
	client.http.Timeout = 0
	client.pollOnce()
	result := make(chan error, 1)
	go func() { result <- client.PostCommand(context.Background(), "silence_alarm", nil) }()
	awaitRecoveryEvent(t, commandStarted, "command start")
	// Fail only the snapshot, while the real HTTP/2 command is still active.
	transport.failNext.Store(true)
	client.pollOnce()
	if client.IsConnected() {
		t.Fatal("failed poll was treated as connected")
	}
	client.pollOnce()
	if !client.IsConnected() {
		t.Fatal("poll did not recover while the command was active")
	}
	select {
	case <-commandCancelled:
		t.Fatal("poll recovery interrupted the active command")
	default:
	}
	close(finishCommand)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("active command failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("command did not complete")
	}
	if commands.Load() != 1 {
		t.Fatalf("command was replayed %d times", commands.Load())
	}
}

type failingSnapshotTransport struct {
	*http.Transport
	failNext atomic.Bool
}

func (t *failingSnapshotTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet && t.failNext.Swap(false) {
		return nil, context.DeadlineExceeded
	}
	return t.Transport.RoundTrip(req)
}
