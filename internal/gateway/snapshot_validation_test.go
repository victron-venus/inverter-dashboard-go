package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSnapshotRejectsNullAndOversizedPayload(t *testing.T) {
	const limit = 8 << 20
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"empty object", `{}`, true},
		{"explicit controller clear", `{"inverter":null}`, true},
		{"null", `null`, false},
		{"array", `[]`, false},
		{"scalar", `false`, false},
		{"exact size", `{}` + strings.Repeat(" ", limit-2), true},
		{"truncated trailing garbage", `{}` + strings.Repeat(" ", limit-2) + "x", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.payload)
			}))
			defer server.Close()
			client, err := NewClient(Config{URL: server.URL, APIToken: "test"}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = server.Client().Transport
			snapshot, err := client.FetchSnapshot(context.Background())
			if (err == nil) != tc.valid || (!tc.valid && snapshot != nil) {
				t.Fatalf("valid=%v, snapshot=%v, error=%v", tc.valid, snapshot != nil, err)
			}
		})
	}
}
