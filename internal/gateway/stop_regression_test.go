package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestStopCancelsInFlightPoll(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	c, err := NewClient(Config{URL: server.URL, APIToken: "test", PollInterval: time.Hour}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.Start()
	defer c.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("poll did not start")
	}
	c.Stop()
	select {
	case <-cancelled:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Stop left snapshot HTTP request running")
	}
}

func TestStopDoesNotPublishLateSnapshot(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	applied := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	c, err := NewClient(Config{URL: server.URL, APIToken: "test", PollInterval: time.Hour}, func(*state.State) { applied <- struct{}{} }, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.Start()
	defer c.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("poll did not start")
	}
	c.Stop()
	// Dual-path main switches back to native MQTT immediately after this call.
	close(release)
	select {
	case <-applied:
		t.Fatal("stopped IGW overwrote state with a late snapshot")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestStopCancelsSnapshotBodyRead(t *testing.T) {
	bodyStarted := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"meta":`)
		w.(http.Flusher).Flush()
		close(bodyStarted)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	c, err := NewClient(Config{URL: server.URL, APIToken: "test"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.Start()
	defer c.Stop()
	select {
	case <-bodyStarted:
	case <-time.After(time.Second):
		t.Fatal("body did not start")
	}
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the body read")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("request context was not cancelled")
	}
}

func TestStopJoinsActiveCallback(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	c, err := NewClient(Config{URL: server.URL, APIToken: "test"}, func(*state.State) {
		close(entered)
		<-release
		close(exited)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.Start()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		c.Stop()
		t.Fatal("callback did not start")
	}
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	select {
	case <-stopped:
		close(release)
		t.Fatal("Stop returned while the old source callback was still active")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not join callback")
	}
	select {
	case <-exited:
	default:
		t.Fatal("callback still running after Stop")
	}
	if c.IsConnected() {
		t.Fatal("stopped client still connected")
	}
}

func TestStopDuringStatusCallbackDoesNotApply(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	applied := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	c, err := NewClient(Config{URL: server.URL, APIToken: "test"}, func(*state.State) { applied <- struct{}{} }, func(bool) { close(entered); <-release })
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.Start()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		c.Stop()
		t.Fatal("status callback did not start")
	}
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	select {
	case <-c.ctx.Done():
	case <-time.After(time.Second):
		close(release)
		t.Fatal("Stop did not cancel context")
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not join status callback")
	}
	select {
	case <-applied:
		t.Fatal("cancelled poll applied state after status callback")
	default:
	}
}

func TestStopBeforeStartAndRepeatedStop(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	c, err := NewClient(Config{URL: server.URL, APIToken: "test"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	c.Stop()
	c.Start()
	c.Stop()
	select {
	case <-requests:
		t.Fatal("stopped client issued a request")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestConcurrentStartStopHasSinglePoller(t *testing.T) {
	var requests atomic.Int32
	entered := make(chan struct{}, 32)
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	c, err := NewClient(Config{URL: server.URL, APIToken: "test"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	var starts sync.WaitGroup
	for range 16 {
		starts.Go(c.Start)
	}
	starts.Wait()
	select {
	case <-entered:
	case <-time.After(time.Second):
		c.Stop()
		t.Fatal("poll did not start")
	}
	var stops sync.WaitGroup
	for range 16 {
		stops.Go(c.Stop)
	}
	stops.Wait()
	c.Start()
	c.Stop()
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected single request, got %d", got)
	}
	if c.IsConnected() {
		t.Fatal("stopped client still connected")
	}
}

func TestStartRacesStop(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	for range 25 {
		c, err := NewClient(Config{URL: server.URL, APIToken: "test"}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		c.http = server.Client()
		ready := make(chan struct{})
		var racers sync.WaitGroup
		for range 8 {
			racers.Go(func() { <-ready; c.Start() })
			racers.Go(func() { <-ready; c.Stop() })
		}
		close(ready)
		finished := make(chan struct{})
		go func() { racers.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("concurrent Start/Stop did not complete")
		}
		c.Start()
		c.Stop()
		if c.IsConnected() {
			t.Fatal("stopped client still connected")
		}
	}
}
