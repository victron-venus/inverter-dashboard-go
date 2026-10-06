package push

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func capturedAttempt(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal("expected exactly one JSON attempt record", err)
	}
	allowed := map[string]bool{"time": true, "level": true, "msg": true, "kind": true, "attempt": true, "http_status": true, "error_category": true}
	if len(record) != len(allowed) {
		t.Fatal("unexpected log fields", record)
	}
	for key := range record {
		if !allowed[key] {
			t.Fatal("unexpected log field", key)
		}
	}
	if record["msg"] != "Web Push attempt completed" || record["level"] != "INFO" {
		t.Fatal("unexpected attempt message", record)
	}
	return record
}

func TestSenderReportsOnlySanitizedAttemptOutcome(t *testing.T) {
	const secret = "fake-private-provider-marker"
	for _, tc := range []struct {
		name     string
		status   int
		err      error
		category string
	}{
		{"accepted", 201, nil, "none"},
		{"rejected", 400, nil, "none"},
		{"rate-limited", 429, nil, "none"},
		{"server-error", 503, nil, "none"},
		{"deadline", 0, context.DeadlineExceeded, "timeout"},
		{"cancelled", 0, context.Canceled, "cancelled"},
		{"dns", 0, &net.DNSError{Name: secret, Err: secret}, "dns"},
		{"tls", 0, &tls.CertificateVerificationError{Err: errors.New(secret)}, "tls"},
		{"tls-record", 0, tls.RecordHeaderError{Msg: secret}, "tls"},
		{"tls-authority", 0, x509.UnknownAuthorityError{}, "tls"},
		{"connection", 0, &net.OpError{Op: secret, Net: secret, Err: errors.New(secret)}, "connection"},
		{"other", 0, errors.New(secret), "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, d, item := queuedDispatcher(t)
			var output bytes.Buffer
			d.logger = slog.New(slog.NewJSONHandler(&output, nil))
			d.client = testHTTPClient(func(*http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, &url.Error{Op: "Post", URL: "https://fcm.googleapis.com/" + secret, Err: tc.err}
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(secret)), Header: http.Header{"Location": []string{"https://example.org/" + secret}}}, nil
			})
			d.deliver(item, time.Now())
			record := capturedAttempt(t, output.Bytes())
			if record["kind"] != "native" || record["attempt"] != float64(1) || record["http_status"] != float64(tc.status) || record["error_category"] != tc.category {
				t.Fatal("incorrect outcome", record)
			}
			for _, forbidden := range []string{secret, "fcm.googleapis.com", "private source body", item.ID, item.SubscriptionID, "vapid "} {
				if strings.Contains(output.String(), forbidden) {
					t.Fatal("attempt log exposed sensitive context")
				}
			}
		})
	}
}

func TestAttemptLogsUseFixedVocabularyAndBounds(t *testing.T) {
	var output bytes.Buffer
	d := &Dispatcher{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	d.logAttempt(delivery{Attempts: 900, Payload: Payload{Kind: "https://example.org/secret"}}, 900, errPushDNSProhibited)
	record := capturedAttempt(t, output.Bytes())
	if record["kind"] != "unknown" || record["attempt"] != float64(0) || record["http_status"] != float64(0) || record["error_category"] != "destination_blocked" {
		t.Fatal("unbounded log attributes", record)
	}
	for err, want := range map[error]string{errPushEndpoint: "destination_blocked", errPushDNS: "dns", errPushConnection: "connection", errNotificationRetired: "retired", errNotificationExpired: "expired"} {
		if got := deliveryErrorCategory(&url.Error{Op: "Post", URL: "https://example.org/secret", Err: err}); got != want {
			t.Fatal("lost typed failure category", got, want)
		}
	}
}

func TestServiceInjectsLoggerForExplicitTestAttempt(t *testing.T) {
	var output bytes.Buffer
	s := NewService(Config{Enabled: true, DataDir: testDirectory(t), Logger: slog.New(slog.NewJSONHandler(&output, nil))})
	t.Cleanup(s.Close)
	if !s.Available() {
		t.Fatal("test service unavailable")
	}
	subscription := validTestSubscription(t)
	if err := s.store.Register(subscription, DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.store.QueueTest(subscription.Endpoint, now); err != nil {
		t.Fatal(err)
	}
	s.dispatcher.client = testHTTPClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	s.dispatcher.deliver(s.dispatcher.pending(now)[0], now)
	record := capturedAttempt(t, output.Bytes())
	if record["kind"] != "test" || record["http_status"] != float64(201) || queueSize(s.store) != 0 {
		t.Fatal("explicit test outcome missing or delivery behavior changed", record)
	}
}
