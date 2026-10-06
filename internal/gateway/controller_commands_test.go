package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOverrideSingleDispatchRequiresExactAckUnderOneDeadline(t *testing.T) {
	for _, reason := range []string{"set", "stop", "wrongvalue", "wrongid", "error", "unsupported", "stopped", "slowpreflight", "slowack"} {
		t.Run(reason, func(t *testing.T) {
			var posts atomic.Int32
			var gets atomic.Int32
			var client *Client
			var value any = 42
			if reason == "stop" {
				value = nil
			}
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
					var m map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&m)
					if m["request_id"] != "request" || r.URL.Path != "/v1/commands/setpoint_override" {
						t.Error("correlation/path changed")
					}
					w.WriteHeader(204)
					return
				}
				n := gets.Add(1)
				if reason == "slowpreflight" || reason == "slowack" && n > 1 {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(time.Second):
					}
					return
				}
				status := map[string]interface{}{"value": nil, "request_id": nil, "last_error": nil}
				if n > 1 {
					status["value"] = value
					status["request_id"] = "request"
					switch reason {
					case "wrongvalue":
						status["value"] = 43
					case "wrongid":
						status["request_id"] = "other"
					case "error":
						status["last_error"] = "controller refused"
					}
				}
				if reason == "stopped" {
					client.Stop()
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"capabilities": map[string]bool{"setpoint_override": reason != "unsupported"}, "inverter": map[string]interface{}{"setpoint_override": status, "dry_run": true}})
			}))
			defer srv.Close()
			var err error
			client, err = NewClient(Config{URL: srv.URL, APIToken: "fixture"}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = srv.Client().Transport
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			start := time.Now()
			err = client.PostCommand(ctx, "setpoint_override", map[string]interface{}{"value": value, "request_id": "request"})
			want := reason == "set" || reason == "stop"
			if (err == nil) != want {
				t.Fatalf("error=%v", err)
			}
			if posts.Load() > 1 {
				t.Fatal("command retried")
			}
			if (reason == "unsupported" || reason == "stopped" || reason == "slowpreflight") && posts.Load() != 0 {
				t.Fatal("preflight failure dispatched")
			}
			if time.Since(start) > 700*time.Millisecond {
				t.Fatal("deadline restarted during acknowledgement")
			}
		})
	}
}

func TestGatewayPreservesGridEnvelopeOriginalMeasurementTime(t *testing.T) {
	var snap Snapshot
	raw := `{"capabilities":{},"inverter":{"daily_stats":{"grid_energy":{"complete":false,"source":"native","generated_at":100}},"grid_backup":{"enabled":true,"available":true,"measurement_time":123},"grid_backup_observed_at":99999999999},"system":{"0/Ac/Grid/L1/Power":0,"0/Ac/Grid/L2/Power":null},"grid":{"30/Ac/L2/Power":500}}`
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		st := SnapshotToState(&snap, MapOptions{})
		if st.GridBackupObservedAt == nil || *st.GridBackupObservedAt != 123 || st.GridL2Available == nil || *st.GridL2Available || st.GT != 0 {
			t.Fatal("provenance or null phase lost")
		}
		var energy map[string]interface{}
		if json.Unmarshal(st.DailyStats.GridEnergy, &energy) != nil || energy["complete"] != false {
			t.Fatal("daily grid envelope lost")
		}
	}
}
