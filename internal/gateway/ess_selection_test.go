package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestESSSelectionRechecksSnapshotBeforePost(t *testing.T) {
	for _, reason := range []string{"ok", "gateway_unsupported", "controller_unsupported", "dry", "unknown", "null", "stopped"} {
		t.Run(reason, func(t *testing.T) {
			var posts atomic.Int32
			var client *Client
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					controller := map[string]interface{}{"ess_mode": map[string]interface{}{"selection_supported": reason != "controller_unsupported", "selected": "external_control"}, "dry_run": reason == "dry"}
					if reason == "unknown" {
						delete(controller, "dry_run")
					}
					if reason == "null" {
						controller = nil
					}
					if reason == "stopped" {
						client.Stop()
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"capabilities": map[string]bool{"set_ess_mode": reason != "gateway_unsupported"}, "inverter": controller})
					return
				}
				posts.Add(1)
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/v1/commands/set_ess_mode" || body["mode"] != "off" || body["request_id"] != "new-request" {
					t.Error("ESS body/correlation changed")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()
			var err error
			client, err = NewClient(Config{URL: srv.URL, APIToken: "fixture"}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = srv.Client().Transport
			err = client.PostCommand(context.Background(), "set_ess_mode", map[string]interface{}{"mode": "off", "request_id": "new-request"})
			want := int32(0)
			if reason == "ok" {
				want = 1
			}
			if (err == nil) != (reason == "ok") || posts.Load() != want {
				t.Fatalf("%s error=%v posts=%d", reason, err, posts.Load())
			}
		})
	}
}
