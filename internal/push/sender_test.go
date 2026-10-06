package push

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testHTTPClient func(*http.Request) (*http.Response, error)

func (f testHTTPClient) Do(r *http.Request) (*http.Response, error) { return f(r) }
func queuedDispatcher(t *testing.T) (*Store, *Dispatcher, delivery) {
	t.Helper()
	s := testStore(t)
	sub := validTestSubscription(t)
	_ = s.Register(sub, DefaultPreferences())
	_ = s.ResetEpoch("one")
	now := time.Now()
	_ = s.Enqueue(makePayload("native", "victron", "source-id", "Alarm", "private source body", now.Add(-250*time.Second), now), "one", false)
	d := newDispatcher(s, "https://example.org/contact")
	t.Cleanup(d.Close)
	return s, d, d.pending(now)[0]
}
func TestSenderEncryptsAndUsesRemainingSourceTTL(t *testing.T) {
	s, d, item := queuedDispatcher(t)
	calls := 0
	d.client = testHTTPClient(func(req *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(req.Body)
		ttl, _ := strconv.Atoi(req.Header.Get("TTL"))
		if strings.Contains(string(raw), "private source body") || len(raw) < 100 || req.Header.Get("Content-Encoding") != "aes128gcm" || !strings.HasPrefix(req.Header.Get("Authorization"), "vapid ") || ttl < 1 || ttl > 50 {
			t.Fatal("not standard encrypted bounded push")
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	d.deliver(item, time.Now())
	if calls != 1 || queueSize(s) != 0 {
		t.Fatal("delivery failed")
	}
}
func TestSenderFinalFenceAndEpochCancelsInFlightRequest(t *testing.T) {
	s, d, item := queuedDispatcher(t)
	calls := 0
	d.client = testHTTPClient(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected") })
	_ = s.ResetEpoch("two")
	request, _ := http.NewRequestWithContext(context.Background(), "POST", "https://fcm.googleapis.com/fcm/send/example", nil)
	if _, err := (guardedClient{d, item, d.client}).Do(request); err == nil || calls != 0 {
		t.Fatal("retired queue snapshot sent")
	}
	s, d, item = queuedDispatcher(t)
	started := make(chan struct{})
	done := make(chan struct{})
	d.client = testHTTPClient(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	go func() { d.deliver(item, time.Now()); close(done) }()
	<-started
	if err := s.ResetEpoch("next"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("epoch did not cancel HTTP request")
	}
	if queueSize(s) != 0 {
		t.Fatal("retired retry persisted")
	}
}
func TestSenderDeletedSubscriptionCancelsInFlight(t *testing.T) {
	s, d, item := queuedDispatcher(t)
	started := make(chan struct{})
	done := make(chan struct{})
	d.client = testHTTPClient(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	go func() { d.deliver(item, time.Now()); close(done) }()
	<-started
	_ = s.Delete("https://fcm.googleapis.com/fcm/send/example")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delete did not cancel HTTP request")
	}
}
func TestSenderTerminalRemovalAndBoundedRetry(t *testing.T) {
	for _, status := range []int{404, 410, 400, 429, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			s, d, item := queuedDispatcher(t)
			calls := 0
			d.client = testHTTPClient(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			for i := 0; i < 3; i++ {
				d.deliver(item, time.Now())
				items := d.pending(time.Now().Add(20 * time.Second))
				if len(items) == 0 {
					break
				}
				item = items[0]
			}
			expected := 1
			if status == 429 || status == 503 {
				expected = 3
			}
			if calls != expected || queueSize(s) != 0 {
				t.Fatal(calls, queueSize(s))
			}
			_, exists := s.SubscriptionStatus("https://fcm.googleapis.com/fcm/send/example")
			if exists == (status == 404 || status == 410) {
				t.Fatal("terminal subscription policy")
			}
		})
	}
}

func TestRetiredProviderResponseCannotDeleteReregisteredSubscription(t *testing.T) {
	s, d, item := queuedDispatcher(t)
	sub := s.data.Subscriptions[item.SubscriptionID].Subscription
	_ = s.Delete(sub.Endpoint)
	_ = s.Register(sub, DefaultPreferences())
	d.finish(item, 410, false, time.Now())
	if _, exists := s.SubscriptionStatus(sub.Endpoint); !exists {
		t.Fatal("retired response removed new registration")
	}
}
