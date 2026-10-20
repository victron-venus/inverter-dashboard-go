package push

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type admissionSnapshot struct {
	FailureCount        uint64 `json:"failureCount"`
	LastFailureCategory string `json:"lastFailureCategory"`
	LastFailureAtMS     int64  `json:"lastFailureAtMs"`
}

type serviceStatusSnapshot struct {
	Available            bool               `json:"available"`
	PublicKey            *string            `json:"publicKey"`
	Reason               *string            `json:"reason"`
	AdmissionDiagnostics *admissionSnapshot `json:"admissionDiagnostics"`
}

func admissionStatus(router http.Handler) (serviceStatusSnapshot, string, error) {
	rec := apiRequest(router, http.MethodGet, "/api/notifications/status", "", nil)
	var status serviceStatusSnapshot
	if rec.Code != http.StatusOK {
		return status, "", fmt.Errorf("status endpoint returned %d", rec.Code)
	}
	raw := rec.Body.String()
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		return status, raw, err
	}
	if status.AdmissionDiagnostics == nil {
		return status, raw, fmt.Errorf("admissionDiagnostics is missing")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		return status, raw, err
	}
	var diagnostics map[string]json.RawMessage
	if err := json.Unmarshal(fields["admissionDiagnostics"], &diagnostics); err != nil {
		return status, raw, err
	}
	if len(diagnostics) != 3 {
		return status, raw, fmt.Errorf("unexpected admission diagnostic fields")
	}
	for _, key := range []string{"failureCount", "lastFailureCategory", "lastFailureAtMs"} {
		if value, ok := diagnostics[key]; !ok || string(value) == "null" {
			return status, raw, fmt.Errorf("missing admission diagnostic field %s", key)
		}
	}
	return status, raw, nil
}

func requireAdmissionStatus(t *testing.T, router http.Handler, want admissionSnapshot) (serviceStatusSnapshot, string) {
	t.Helper()
	status, raw, err := admissionStatus(router)
	if err != nil {
		t.Fatal(err)
	}
	if *status.AdmissionDiagnostics != want {
		t.Fatalf("diagnostics = %+v, want %+v", *status.AdmissionDiagnostics, want)
	}
	return status, raw
}

func primeAdmissionService(t *testing.T, service *Service, now time.Time) string {
	t.Helper()
	sub := validTestSubscription(t)
	if err := service.store.Register(sub, DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	service.Reset("gateway", true, now)
	service.Observe(Observation{Notifications: []Native{}}, now)
	return sub.Endpoint
}

func fillAdmissionQueue(t *testing.T, service *Service, endpoint string, now time.Time) {
	t.Helper()
	if err := service.store.mutate(func(data *diskState) (bool, error) {
		for i := 0; i < maxPending; i++ {
			payload := makePayload("native", "victron", fmt.Sprintf("existing-%d", i), "Existing", "Queued", now, now)
			data.Queue = append(data.Queue, delivery{
				ID: payload.EventKey + ":" + subscriptionID(endpoint), SubscriptionID: subscriptionID(endpoint),
				Epoch: data.Epoch, Payload: payload,
			})
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func requireOpaqueAdmissionStatus(t *testing.T, raw string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if value != "" && strings.Contains(raw, value) {
			t.Fatal("status exposed event, endpoint, or raw storage error data")
		}
	}
}

func TestServiceAdmissionDiagnosticsStartEmpty(t *testing.T) {
	router, service := apiRouter(t)
	status, _ := requireAdmissionStatus(t, router, admissionSnapshot{})
	if !status.Available || status.PublicKey == nil || *status.PublicKey != service.store.PublicKey() || status.Reason != nil {
		t.Fatal("new service did not preserve its available status and public key")
	}
}

func TestServiceAdmissionQueueFullRemainsAvailableAndRecovers(t *testing.T) {
	router, service := apiRouter(t)
	now := time.Now().UTC()
	endpoint := primeAdmissionService(t, service, now)
	publicKey := service.store.PublicKey()
	fillAdmissionQueue(t, service, endpoint, now)
	at := now.Add(time.Second)
	event := native("private-native-id", at)
	event.Title, event.Body = "private-native-title", "private-native-body"
	observation := Observation{Notifications: []Native{event}}
	service.Observe(observation, at)
	want := admissionSnapshot{FailureCount: 1, LastFailureCategory: "queue_full", LastFailureAtMS: at.UnixMilli()}
	status, raw := requireAdmissionStatus(t, router, want)
	if !service.Available() || !status.Available || status.PublicKey == nil || *status.PublicKey != publicKey || status.Reason != nil {
		t.Fatal("queue capacity disabled the service or removed its public key")
	}
	if queueSize(service.store) != maxPending {
		t.Fatal("failed admission changed the full queue")
	}
	requireOpaqueAdmissionStatus(t, raw, endpoint, event.ID, event.Title, event.Body, errCapacity.Error())

	if err := service.store.mutate(func(data *diskState) (bool, error) {
		data.Queue = data.Queue[1:]
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	// Retry the same event: a rejected admission must not consume its dedupe key.
	service.Observe(observation, at.Add(time.Second))
	if queueSize(service.store) != maxPending {
		t.Fatal("event was not admitted after queue space became available")
	}
	service.store.mu.Lock()
	last := service.store.data.Queue[len(service.store.data.Queue)-1]
	service.store.mu.Unlock()
	if last.Payload.Title != event.Title || last.Payload.Body != event.Body {
		t.Fatal("recovery did not enqueue the original event")
	}
	service.Observe(observation, at.Add(2*time.Second))
	requireAdmissionStatus(t, router, want)
	if queueSize(service.store) != maxPending {
		t.Fatal("duplicate observation changed the queue")
	}
}

func TestServiceAdmissionPersistenceFailureRemainsFailClosed(t *testing.T) {
	router, service := apiRouter(t)
	now := time.Now().UTC()
	endpoint := primeAdmissionService(t, service, now)
	path := filepath.Join(service.store.directory, "state.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	at := now.Add(time.Second)
	event := native("private-failed-id", at)
	event.Body = "private-failed-body"
	observation := Observation{Notifications: []Native{event}}
	service.Observe(observation, at)
	want := admissionSnapshot{FailureCount: 1, LastFailureCategory: "processing_error", LastFailureAtMS: at.UnixMilli()}
	status, raw := requireAdmissionStatus(t, router, want)
	if service.Available() || status.Available || status.PublicKey != nil || status.Reason == nil {
		t.Fatal("failed durable storage must keep the service unavailable")
	}
	if queueSize(service.store) != 0 {
		t.Fatal("failed durable admission changed the queue")
	}
	requireOpaqueAdmissionStatus(t, raw, endpoint, event.ID, event.Body, path, "push state cannot be committed")
	service.Observe(observation, at.Add(time.Second))
	requireAdmissionStatus(t, router, want)
}

func TestServiceAdmissionCounterSaturatesAndKeepsLatestFailure(t *testing.T) {
	router, service := apiRouter(t)
	now := time.Now().UTC()
	endpoint := primeAdmissionService(t, service, now)
	fillAdmissionQueue(t, service, endpoint, now)
	const maxSafeInteger uint64 = 9007199254740991
	service.admissionMu.Lock()
	service.admission = admissionDiagnostics{
		FailureCount: maxSafeInteger - 1, LastFailureCategory: "processing_error", LastFailureAtMs: now.UnixMilli(),
	}
	service.admissionMu.Unlock()
	at := now.Add(time.Second)
	observation := Observation{Notifications: []Native{native("saturation", at)}}
	for i := 0; i < 2; i++ {
		observedAt := at.Add(time.Duration(i) * time.Second)
		service.Observe(observation, observedAt)
		requireAdmissionStatus(t, router, admissionSnapshot{
			FailureCount: maxSafeInteger, LastFailureCategory: "queue_full", LastFailureAtMS: observedAt.UnixMilli(),
		})
	}
}

func TestServiceAdmissionConcurrentObserveAndStatus(t *testing.T) {
	router, service := apiRouter(t)
	now := time.Now().UTC()
	endpoint := primeAdmissionService(t, service, now)
	fillAdmissionQueue(t, service, endpoint, now)
	publicKey := service.store.PublicKey()
	at := now.Add(time.Second)
	observation := Observation{Notifications: []Native{native("concurrent", at)}}
	const workers, attempts, readers = 8, 8, 4
	start := make(chan struct{})
	failures := make(chan error, readers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for j := 0; j < attempts; j++ {
				service.Observe(observation, at)
			}
		}()
	}
	for i := 0; i < readers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for j := 0; j < workers*attempts; j++ {
				status, _, err := admissionStatus(router)
				if err != nil {
					failures <- err
					return
				}
				if !status.Available || status.PublicKey == nil || *status.PublicKey != publicKey {
					failures <- fmt.Errorf("concurrent queue failure changed service availability")
					return
				}
				got := *status.AdmissionDiagnostics
				if got.FailureCount == 0 {
					if got.LastFailureCategory != "" || got.LastFailureAtMS != 0 {
						failures <- fmt.Errorf("inconsistent empty diagnostic snapshot")
						return
					}
				} else if got.FailureCount > workers*attempts || got.LastFailureCategory != "queue_full" || got.LastFailureAtMS != at.UnixMilli() {
					failures <- fmt.Errorf("inconsistent failure diagnostic snapshot")
					return
				}
			}
		}()
	}
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	requireAdmissionStatus(t, router, admissionSnapshot{
		FailureCount: workers * attempts, LastFailureCategory: "queue_full", LastFailureAtMS: at.UnixMilli(),
	})
}
