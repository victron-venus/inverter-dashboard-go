package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const baselineRequestID = "fixture-request"
const baselineStaleController = "stale-controller"

type baselineCommandCase struct {
	name    string
	body    map[string]interface{}
	wantRaw string
}

func baselineCommandCases() []baselineCommandCase {
	return []baselineCommandCase{
		{"toggle", map[string]interface{}{"entity": "input_boolean.only_charging", "state": false}, `{"entity":"only_charging","state":"off"}`},
		{"dry_run", map[string]interface{}{"value": false}, `{"value":false}`},
		{"ess_mode", map[string]interface{}{}, `{}`},
	}
}

// These are the optional fields of the real IGW Capabilities schema. The
// baseline toggle, dry_run and ess_mode commands have no capability fields.
func baselineCapabilities() map[string]bool {
	return map[string]bool{
		"set_ess_mode": true, "setpoint_override": true,
		"electricity_tariff": true, "water_mode": true,
	}
}

func baselineSnapshot(t *testing.T, capabilities map[string]bool) []byte {
	t.Helper()
	snapshot := map[string]interface{}{
		"capabilities": capabilities,
		"inverter": map[string]interface{}{
			"dry_run":  false,
			"booleans": map[string]interface{}{"only_charging": false},
			"ess_mode": map[string]interface{}{"selection_supported": true, "selected": "external_control"},
			"setpoint_override": map[string]interface{}{
				"value": 42, "request_id": baselineRequestID, "last_error": nil,
			},
			"ui_config": map[string]interface{}{
				"electricity_tariff_status": map[string]interface{}{"writable": true, "revision": strings.Repeat("a", 64)},
			},
		},
		"pump": map[string]interface{}{"1/Mode": 0},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type baselinePublication struct {
	path string
	body map[string]interface{}
}

type baselinePeer struct {
	gets  atomic.Int32
	mu    sync.Mutex
	posts []baselinePublication
}

func (p *baselinePeer) publications() []baselinePublication {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]baselinePublication(nil), p.posts...)
}

// Trust only this loopback server's certificate; preserve client redirects,
// deadlines and source-retirement handling from NewClient.
func newBaselineClient(t *testing.T, snapshot func(http.ResponseWriter, *http.Request)) (*Client, *baselinePeer) {
	t.Helper()
	peer := &baselinePeer{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing fixture authentication")
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v1/snapshot" {
			peer.gets.Add(1)
			w.Header().Set("Content-Type", "application/json")
			snapshot(w, r)
			return
		}
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/v1/commands/") {
			t.Error("unexpected fixture request")
			http.NotFound(w, r)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("command content type changed")
		}
		peer.mu.Lock()
		peer.posts = append(peer.posts, baselinePublication{r.URL.Path, body})
		peer.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(Config{URL: server.URL, APIToken: "fixture"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = server.Client().Transport
	t.Cleanup(client.Stop)
	return client, peer
}

func requireBaselinePublication(t *testing.T, peer *baselinePeer, name, wantRaw string) {
	t.Helper()
	posts := peer.publications()
	if len(posts) != 1 {
		t.Fatalf("expected exactly one publication, got %d", len(posts))
	}
	var want map[string]interface{}
	if err := json.Unmarshal([]byte(wantRaw), &want); err != nil {
		t.Fatal(err)
	}
	if posts[0].path != "/v1/commands/"+name || !reflect.DeepEqual(posts[0].body, want) {
		t.Fatalf("publication differs: path=%s body=%v", posts[0].path, posts[0].body)
	}
}

func TestBaselineCommandsUseRealGatewayCapabilitySchema(t *testing.T) {
	for _, tc := range baselineCommandCases() {
		t.Run(tc.name, func(t *testing.T) {
			raw := baselineSnapshot(t, baselineCapabilities())
			client, peer := newBaselineClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(raw) })
			if err := client.PostCommand(context.Background(), tc.name, tc.body); err != nil {
				t.Fatalf("baseline command rejected by real capability schema: %v", err)
			}
			if peer.gets.Load() != 1 {
				t.Fatalf("command did not use one fresh snapshot: %d", peer.gets.Load())
			}
			requireBaselinePublication(t, peer, tc.name, tc.wantRaw)
		})
	}
}

func TestBaselineCommandsRejectUnavailableCurrentSnapshot(t *testing.T) {
	for _, tc := range baselineCommandCases() {
		for _, reason := range []string{"missing-controller", baselineStaleController, "malformed", "fetch-failed", "retired-before", "retired-during-fetch"} {
			t.Run(tc.name+"/"+reason, func(t *testing.T) {
				testUnavailableBaselineSnapshot(t, tc, reason)
			})
		}
	}
}

func writeUnavailableBaselineSnapshot(w http.ResponseWriter, client *Client, reason string, raw []byte) {
	switch reason {
	case "missing-controller":
		_, _ = io.WriteString(w, `{"capabilities":{}}`)
	case baselineStaleController:
		// IGW expires an old controller observation as explicit null.
		_, _ = io.WriteString(w, `{"capabilities":{},"inverter":null}`)
	case "malformed":
		_, _ = io.WriteString(w, `{"inverter":[]}`)
	case "fetch-failed":
		w.WriteHeader(http.StatusServiceUnavailable)
	case "retired-during-fetch":
		client.Stop()
		_, _ = w.Write(raw)
	default:
		_, _ = w.Write(raw)
	}
}

func testUnavailableBaselineSnapshot(t *testing.T, tc baselineCommandCase, reason string) {
	t.Helper()
	raw := baselineSnapshot(t, baselineCapabilities())
	var client *Client
	var readyPoll atomic.Bool
	client, peer := newBaselineClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if readyPoll.Load() {
			_, _ = w.Write(raw)
			return
		}
		writeUnavailableBaselineSnapshot(w, client, reason, raw)
	})
	if reason == baselineStaleController {
		readyPoll.Store(true)
		client.pollOnce()
		if !client.IsConnected() {
			t.Fatal("initial ready snapshot did not connect")
		}
		readyPoll.Store(false)
	}
	if reason == "retired-before" {
		client.Stop()
	}
	if err := client.PostCommand(context.Background(), tc.name, tc.body); err == nil {
		t.Fatal("unavailable current source accepted a command")
	}
	if len(peer.publications()) != 0 {
		t.Fatal("unavailable current source sent a POST")
	}
}

func TestOptionalGatewayFeaturesStillRequireTheirCapability(t *testing.T) {
	revision := strings.Repeat("a", 64)
	cases := []baselineCommandCase{
		{"set_ess_mode", map[string]interface{}{"mode": "off", "request_id": baselineRequestID}, `{"mode":"off","request_id":"fixture-request"}`},
		{"setpoint_override", map[string]interface{}{"value": 42, "request_id": baselineRequestID}, `{"value":42,"request_id":"fixture-request"}`},
		{"electricity_tariff", map[string]interface{}{"plan": nil, "revision": revision, "request_id": baselineRequestID}, `{"plan":null,"revision":"` + revision + `","request_id":"fixture-request"}`},
		{"water_mode", map[string]interface{}{"instance": float64(1), "mode": float64(2)}, `{"instance":1,"mode":2}`},
	}
	for _, tc := range cases {
		for _, availability := range []string{"supported", "absent", "false"} {
			t.Run(tc.name+"/"+availability, func(t *testing.T) {
				testOptionalBaselineCapability(t, tc, availability)
			})
		}
	}
}

func testOptionalBaselineCapability(t *testing.T, tc baselineCommandCase, availability string) {
	t.Helper()
	capabilities := baselineCapabilities()
	switch availability {
	case "absent":
		delete(capabilities, tc.name)
	case "false":
		capabilities[tc.name] = false
	}
	// Keep all controller/native readiness fields identical to the
	// supported positive control; change only this capability.
	raw := baselineSnapshot(t, capabilities)
	client, peer := newBaselineClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(raw) })
	err := client.PostCommand(context.Background(), tc.name, tc.body)
	if availability == "supported" {
		if err != nil {
			t.Fatalf("supported feature rejected: %v", err)
		}
		requireBaselinePublication(t, peer, tc.name, tc.wantRaw)
		return
	}
	if err == nil || len(peer.publications()) != 0 {
		t.Fatalf("unsupported feature dispatched: error=%v posts=%d", err, len(peer.publications()))
	}
}
