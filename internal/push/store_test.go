package push

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func testDirectory(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("private Unix storage")
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "notifications")
}
func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(testDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func queueSize(s *Store) int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.data.Queue) }
func TestStorePersistsKeysSubscriptionsDedupeAndLocks(t *testing.T) {
	dir := testDirectory(t)
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sub := validTestSubscription(t)
	if err = s.Register(sub, DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(dir); err == nil {
		t.Fatal("second writer allowed")
	}
	key := s.PublicKey()
	now := time.Now()
	_ = s.ResetEpoch("one")
	payload := makePayload("native", "victron", "alarm", "Warning", "Body", now, now)
	if err = s.Enqueue(payload, "one", false); err != nil {
		t.Fatal(err)
	}
	if err = s.Enqueue(payload, "one", false); err != nil {
		t.Fatal(err)
	}
	if queueSize(s) != 1 {
		t.Fatal("dedupe failed")
	}
	_ = s.Close()
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if s.PublicKey() != key || queueSize(s) != 1 {
		t.Fatal("state was not durable")
	}
	if _, ok := s.SubscriptionStatus(sub.Endpoint); !ok {
		t.Fatal("subscription lost")
	}
	if err = s.ResetEpoch("two"); err != nil {
		t.Fatal(err)
	}
	if queueSize(s) != 0 {
		t.Fatal("old epoch survived")
	}
	if err = s.Enqueue(payload, "two", false); err != nil {
		t.Fatal(err)
	}
	if queueSize(s) != 0 {
		t.Fatal("restart replay")
	}
	for _, name := range []string{"state.json", "writer.lock"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private mode")
		}
	}
}

func TestEnqueueReplayAllocationDoesNotGrowWithStoredQueue(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	payload := makePayload("native", "victron", "seen", "Alarm", "Body", now, now)
	if err := s.ResetEpoch("current"); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(payload, "current", true); err != nil {
		t.Fatal(err)
	}
	allocations := func(epoch string) float64 {
		t.Helper()
		return testing.AllocsPerRun(10, func() {
			if err := s.Enqueue(payload, epoch, false); err != nil {
				t.Fatal(err)
			}
		})
	}
	smallDuplicate := allocations("current")
	smallRetired := allocations("retired")
	sub := validTestSubscription(t)
	if err := s.Register(sub, DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	if err := s.mutate(func(data *diskState) (bool, error) {
		for i := 1; i < maxDedupe; i++ {
			data.Dedupe = append(data.Dedupe, eventKey("native", "victron", strconv.Itoa(i), now.UnixMilli()))
		}
		for i := 0; i < maxPending; i++ {
			data.Queue = append(data.Queue, delivery{ID: strconv.Itoa(i), SubscriptionID: subscriptionID(sub.Endpoint), Epoch: "current", Payload: payload})
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	for epoch, small := range map[string]float64{"current": smallDuplicate, "retired": smallRetired} {
		large := allocations(epoch)
		if large > small+5 {
			t.Errorf("%s replay allocations grew with persisted queue: small=%v full=%v", epoch, small, large)
		}
	}
}

func TestEnqueueNoopStillRejectsUnavailableStore(t *testing.T) {
	for _, unavailable := range []string{"closed", "failed"} {
		t.Run(unavailable, func(t *testing.T) {
			s := testStore(t)
			now := time.Now()
			payload := makePayload("native", "victron", "seen", "Alarm", "Body", now, now)
			if err := s.ResetEpoch("current"); err != nil {
				t.Fatal(err)
			}
			if err := s.Enqueue(payload, "current", true); err != nil {
				t.Fatal(err)
			}
			if unavailable == "closed" {
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(s.directory, "state.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := s.ResetEpoch("new"); err == nil {
					t.Fatal("expected persistence failure")
				}
			}
			for _, epoch := range []string{"current", "retired"} {
				if err := s.Enqueue(payload, epoch, false); err == nil {
					t.Fatal("unavailable store accepted no-op", epoch)
				}
			}
		})
	}
}

func TestStoreFailsClosedCorruptionMissingKeyAndSymlink(t *testing.T) {
	for _, mode := range []string{"corrupt", "missing", "symlink", "permissions"} {
		t.Run(mode, func(t *testing.T) {
			dir := testDirectory(t)
			s, err := OpenStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			path := filepath.Join(dir, "state.json")
			switch mode {
			case "corrupt":
				err = os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0600)
			case "missing":
				err = os.Remove(path)
			case "symlink":
				err = os.Remove(path)
				if err == nil {
					err = os.Symlink(filepath.Join(dir, "writer.lock"), path)
				}
			case "permissions":
				err = os.Chmod(path, 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if reopened, err := OpenStore(dir); err == nil {
				_ = reopened.Close()
				t.Fatal("unsafe state accepted")
			}
		})
	}
}
func TestRegistrationConflictsCanonicalKeysAndPreferenceCancellation(t *testing.T) {
	s := testStore(t)
	sub := validTestSubscription(t)
	_ = s.Register(sub, DefaultPreferences())
	padded := sub
	padded.Keys.Auth += "=="
	padded.Keys.P256dh += "="
	if err := s.Register(padded, DefaultPreferences()); err != nil {
		t.Fatal("equivalent key rejected", err)
	}
	different := validTestSubscription(t)
	if err := s.Register(different, DefaultPreferences()); !errors.Is(err, errConflict) {
		t.Fatal("replacement key accepted")
	}
	now := time.Now()
	_ = s.ResetEpoch("a")
	_ = s.Enqueue(makePayload("native", "victron", "a", "Alarm", "", now, now), "a", false)
	prefs := DefaultPreferences()
	prefs.Native = false
	if err := s.Register(sub, prefs); err != nil {
		t.Fatal(err)
	}
	if queueSize(s) != 0 {
		t.Fatal("disabled delivery remains")
	}
	if err := s.QueueTest(sub.Endpoint, now); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueTest(sub.Endpoint, now.Add(time.Second)); !errors.Is(err, errRateLimit) {
		t.Fatal("test flood allowed")
	}
	if err := s.Delete(sub.Endpoint); err != nil {
		t.Fatal(err)
	}
	if queueSize(s) != 0 {
		t.Fatal("deleted delivery remains")
	}
}
func TestPreferencesStrict(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"native":null,"ev":true,"water":true,"lowBattery":true}`, `{"native":true,"ev":true,"water":true,"lowBattery":true,"other":false}`} {
		var p Preferences
		if json.Unmarshal([]byte(raw), &p) == nil {
			t.Fatal("invalid preferences accepted", raw)
		}
	}
}

func TestStoreLimitsAreAtomicAndDedupeIsBounded(t *testing.T) {
	s := testStore(t)
	sub := validTestSubscription(t)
	for i := 0; i < maxSubscriptions; i++ {
		copy := sub
		copy.Endpoint += "/" + strconv.Itoa(i)
		if err := s.Register(copy, DefaultPreferences()); err != nil {
			t.Fatal(err)
		}
	}
	sub.Endpoint += "/overflow"
	if err := s.Register(sub, DefaultPreferences()); !errors.Is(err, errCapacity) {
		t.Fatal("subscription limit")
	}
	_ = s.ResetEpoch("epoch")
	now := time.Now()
	payload := makePayload("native", "victron", "event", "Alarm", "", now, now)
	if err := s.mutate(func(data *diskState) (bool, error) {
		for i := 0; i < maxDedupe; i++ {
			data.Dedupe = append(data.Dedupe, eventKey("native", "victron", string(rune(i)), int64(i+1)))
		}
		for id := range data.Subscriptions {
			for i := 0; i < 16; i++ {
				data.Queue = append(data.Queue, delivery{ID: id + ":" + string(rune(i)), SubscriptionID: id, Epoch: "epoch", Payload: payload})
			}
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue(payload, "epoch", false); !errors.Is(err, errCapacity) {
		t.Fatal("outbox limit")
	}
	if queueSize(s) != maxPending || len(s.data.Dedupe) != maxDedupe {
		t.Fatal("non-atomic overflow")
	}
	if err := s.Enqueue(payload, "epoch", true); err != nil {
		t.Fatal(err)
	}
	if len(s.data.Dedupe) != maxDedupe || s.data.Dedupe[len(s.data.Dedupe)-1] != payload.EventKey {
		t.Fatal("dedupe bound")
	}
}

func TestGlobalTestRateIsPersistedAcrossEndpoints(t *testing.T) {
	s := testStore(t)
	sub := validTestSubscription(t)
	now := time.Now()
	for i := 0; i < 11; i++ {
		copy := sub
		copy.Endpoint += "/" + strconv.Itoa(i)
		if err := s.Register(copy, DefaultPreferences()); err != nil {
			t.Fatal(err)
		}
		err := s.QueueTest(copy.Endpoint, now)
		if i < 10 && err != nil {
			t.Fatal(err)
		}
		if i == 10 && !errors.Is(err, errRateLimit) {
			t.Fatal("global test rate bypass")
		}
	}
}

func TestMalformedPersistedDeliveryFailsClosedAtStartup(t *testing.T) {
	for _, change := range []struct {
		name   string
		update func(*Payload)
	}{
		{"source", func(p *Payload) { p.Source = "unknown" }},
		{"title", func(p *Payload) { p.Title = "" }},
		{"timestamp", func(p *Payload) { p.SourceTimestampMS = 0 }},
		{"observed", func(p *Payload) { p.ObservedAtMS = 0 }},
		{"body", func(p *Payload) { p.Body = string(make([]byte, 1001)) }},
		{"url", func(p *Payload) { p.URL = "https://example.com/" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			dir := testDirectory(t)
			s, err := OpenStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Register(validTestSubscription(t), DefaultPreferences())
			_ = s.ResetEpoch("epoch")
			now := time.Now()
			_ = s.Enqueue(makePayload("native", "victron", "id", "Alarm", "body", now, now), "epoch", false)
			data := s.data
			change.update(&data.Queue[0].Payload)
			_ = s.Close()
			raw, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "state.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			service := NewService(Config{Enabled: true, DataDir: dir})
			defer service.Close()
			if service.Available() {
				t.Fatal("malformed persisted DTO reached dispatcher")
			}
		})
	}
}
