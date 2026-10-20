package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestStopJoinsActiveOverrideStateCallback(t *testing.T) {
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"capabilities":{"setpoint_override":true},"inverter":{"setpoint_override":{"value":42,"request_id":"request","last_error":null},"dry_run":true}}`)
	}))
	defer server.Close()
	c, err := NewClient(Config{URL: server.URL, APIToken: "fixture"}, func(*state.State) {
		close(entered)
		<-release
		close(exited)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.http = server.Client()
	result := make(chan error, 1)
	go func() {
		result <- c.awaitOverride(context.Background(), map[string]interface{}{"value": 42, "request_id": "request"})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		c.Stop()
		t.Fatal("override callback did not start")
	}
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	select {
	case <-c.ctx.Done():
	case <-time.After(time.Second):
		close(release)
		t.Fatal("Stop did not cancel the client")
	}
	select {
	case <-stopped:
		close(release)
		<-result
		t.Fatal("Stop returned before retired override callback finished")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not join override callback")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("override callback still active after Stop")
	}
}
