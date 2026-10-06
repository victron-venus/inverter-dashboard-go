package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const maxStoreBytes = 8 << 20

type diskState struct {
	SchemaVersion int                           `json:"schemaVersion"`
	PrivateKey    string                        `json:"privateKey"`
	PublicKey     string                        `json:"publicKey"`
	Epoch         string                        `json:"epoch"`
	Subscriptions map[string]storedSubscription `json:"subscriptions"`
	Dedupe        []string                      `json:"dedupe"`
	Queue         []delivery                    `json:"queue"`
	Tests         []int64                       `json:"tests"`
}

type Store struct {
	mu        sync.Mutex
	directory string
	lock      *os.File
	data      diskState
	closed    bool
	failed    bool
	active    map[string]context.CancelFunc
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func privateDirectory(directory string) error {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return errors.New("push data directory must be an absolute clean path")
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(directory, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe push data directory")
		}
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return errors.New("push data directory unavailable")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return errors.New("push data directory must have mode0700")
	}
	return nil
}

func OpenStore(directory string) (*Store, error) {
	if err := supportedStorePlatform(); err != nil {
		return nil, err
	}
	if err := privateDirectory(directory); err != nil {
		return nil, err
	}
	_, lockErr := os.Lstat(filepath.Join(directory, "writer.lock"))
	_, stateErr := os.Lstat(filepath.Join(directory, "state.json"))
	if lockErr == nil && errors.Is(stateErr, os.ErrNotExist) {
		return nil, errors.New("initialized push state is missing; application key was not regenerated")
	}
	lock, err := openPrivate(filepath.Join(directory, "writer.lock"), os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, err
	}
	if lockWriter(lock) != nil {
		_ = lock.Close()
		return nil, errors.New("push state already has a writer")
	}
	s := &Store{directory: directory, lock: lock, active: map[string]context.CancelFunc{}}
	if err := s.load(); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	path := filepath.Join(s.directory, "state.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		private, public, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return errors.New("cannot generate push application key")
		}
		s.data = diskState{SchemaVersion: 1, PrivateKey: private, PublicKey: public, Subscriptions: map[string]storedSubscription{}}
		return s.save(s.data)
	}
	file, err := openPrivate(path, os.O_RDONLY)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxStoreBytes+1))
	if err != nil || len(raw) > maxStoreBytes {
		return errors.New("push state is unreadable or oversized")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var data diskState
	if decoder.Decode(&data) != nil || decoder.Decode(new(any)) != io.EOF || !validDiskState(data) {
		return errors.New("push state is corrupt; application key was not regenerated")
	}
	s.data = data
	return nil
}

func validDiskState(data diskState) bool {
	if data.SchemaVersion != 1 || data.Subscriptions == nil || len(data.Subscriptions) > maxSubscriptions || len(data.Dedupe) > maxDedupe || len(data.Queue) > maxPending || len(data.Tests) > 10 {
		return false
	}
	private, err := base64.RawURLEncoding.DecodeString(data.PrivateKey)
	if err != nil {
		return false
	}
	key, err := ecdh.P256().NewPrivateKey(private)
	if err != nil || base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()) != data.PublicKey {
		return false
	}
	for id, sub := range data.Subscriptions {
		if id != subscriptionID(sub.Subscription.Endpoint) || validateSubscription(sub.Subscription) != nil {
			return false
		}
	}
	seen := map[string]bool{}
	for _, id := range data.Dedupe {
		if !digestID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	queued := map[string]bool{}
	for _, item := range data.Queue {
		_, exists := data.Subscriptions[item.SubscriptionID]
		if !exists || queued[item.ID] || item.ID == "" || !item.Payload.persistedValid() || item.Attempts < 0 || item.Attempts > 3 {
			return false
		}
		queued[item.ID] = true
	}
	return true
}

func digestID(text string) bool {
	raw, err := hex.DecodeString(text)
	return err == nil && len(raw) == 32 && text == strings.ToLower(text)
}

func (s *Store) save(data diskState) error {
	raw, err := json.Marshal(data)
	if err != nil || len(raw) > maxStoreBytes {
		return errors.New("push state limit exceeded")
	}
	file, err := os.CreateTemp(s.directory, ".state-*")
	if err != nil {
		return errors.New("push state cannot be saved")
	}
	name := file.Name()
	defer func() { _ = file.Close(); _ = os.Remove(name) }()
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.New("push state cannot be saved")
	}
	if err = os.Rename(name, filepath.Join(s.directory, "state.json")); err != nil {
		return errors.New("push state cannot be committed")
	}
	directory, err := os.Open(s.directory)
	if err != nil {
		return errors.New("push state directory unavailable")
	}
	defer func() { _ = directory.Close() }()
	if directory.Sync() != nil {
		return errors.New("push state cannot be synchronized")
	}
	return nil
}

func (s *Store) mutate(change func(*diskState) (bool, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed {
		return errors.New("push state unavailable")
	}
	raw, err := json.Marshal(s.data)
	if err != nil {
		return errors.New("push state unavailable")
	}
	var next diskState
	if json.Unmarshal(raw, &next) != nil {
		return errors.New("push state unavailable")
	}
	changed, err := change(&next)
	if err != nil || !changed {
		return err
	}
	if err := s.save(next); err != nil {
		s.failed = true
		for _, cancel := range s.active {
			cancel()
		}
		return err
	}
	s.data = next
	queued := map[string]bool{}
	for _, item := range next.Queue {
		queued[item.ID] = true
	}
	for id, cancel := range s.active {
		if !queued[id] {
			cancel()
			delete(s.active, id)
		}
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for _, cancel := range s.active {
		cancel()
	}
	_ = unlockWriter(s.lock)
	return s.lock.Close()
}

func (s *Store) PublicKey() string { s.mu.Lock(); defer s.mu.Unlock(); return s.data.PublicKey }

func (s *Store) Available() bool { s.mu.Lock(); defer s.mu.Unlock(); return !s.closed && !s.failed }

func (s *Store) SubscriptionStatus(endpoint string) (Preferences, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.data.Subscriptions[subscriptionID(endpoint)]
	if !ok || sub.Subscription.Endpoint != endpoint {
		return DefaultPreferences(), false
	}
	return sub.Preferences, true
}

func (s *Store) Register(subscription webpush.Subscription, preferences Preferences) error {
	var err error
	subscription, err = canonicalSubscription(subscription)
	if err != nil {
		return err
	}
	return s.mutate(func(data *diskState) (bool, error) {
		id := subscriptionID(subscription.Endpoint)
		old, exists := data.Subscriptions[id]
		if exists && old.Subscription != subscription {
			return false, errConflict
		}
		if !exists && len(data.Subscriptions) >= maxSubscriptions {
			return false, errCapacity
		}
		data.Subscriptions[id] = storedSubscription{Subscription: subscription, Preferences: preferences, LastTestMS: old.LastTestMS}
		kept := data.Queue[:0]
		for _, item := range data.Queue {
			if item.SubscriptionID != id || preferences.allows(item.Payload.Kind) {
				kept = append(kept, item)
			}
		}
		data.Queue = kept
		return true, nil
	})
}

func (s *Store) Delete(endpoint string) error {
	return s.mutate(func(data *diskState) (bool, error) {
		id := subscriptionID(endpoint)
		if _, ok := data.Subscriptions[id]; !ok {
			return false, nil
		}
		delete(data.Subscriptions, id)
		kept := data.Queue[:0]
		for _, item := range data.Queue {
			if item.SubscriptionID != id {
				kept = append(kept, item)
			}
		}
		data.Queue = kept
		return true, nil
	})
}

func (s *Store) ResetEpoch(epoch string) error {
	return s.mutate(func(data *diskState) (bool, error) {
		data.Epoch = epoch
		kept := data.Queue[:0]
		for _, item := range data.Queue {
			if item.Payload.Kind == "test" {
				kept = append(kept, item)
			}
		}
		data.Queue = kept
		return true, nil
	})
}

func remember(data *diskState, key string) bool {
	for _, old := range data.Dedupe {
		if old == key {
			return false
		}
	}
	data.Dedupe = append(data.Dedupe, key)
	if len(data.Dedupe) > maxDedupe {
		data.Dedupe = data.Dedupe[len(data.Dedupe)-maxDedupe:]
	}
	return true
}

func (s *Store) Enqueue(payload Payload, epoch string, prime bool) error {
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > maxPayloadBytes {
		return errors.New("notification payload is too large")
	}
	// Native snapshots often repeat every known event. Avoid copying the entire
	// durable queue for those no-ops; mutate still rechecks after taking its lock.
	if skip, err := s.skipEnqueue(payload.EventKey, epoch); err != nil || skip {
		return err
	}
	return s.mutate(func(data *diskState) (bool, error) {
		if data.Epoch != epoch || !remember(data, payload.EventKey) {
			return false, nil
		}
		if prime {
			return true, nil
		}
		for id, sub := range data.Subscriptions {
			if !sub.Preferences.allows(payload.Kind) {
				continue
			}
			if len(data.Queue) >= maxPending {
				return false, errCapacity
			}
			data.Queue = append(data.Queue, delivery{ID: payload.EventKey + ":" + id, SubscriptionID: id, Epoch: epoch, Payload: payload})
		}
		return true, nil
	})
}

func (s *Store) skipEnqueue(key, epoch string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed {
		return false, errors.New("push state unavailable")
	}
	return s.data.Epoch != epoch || slices.Contains(s.data.Dedupe, key), nil
}

func (s *Store) QueueTest(endpoint string, now time.Time) error {
	id, err := randomID()
	if err != nil {
		return errors.New("notification unavailable")
	}
	payload := makePayload("test", "system", id, "Test notification", "Web Push is working for this browser.", now, now)
	return s.mutate(func(data *diskState) (bool, error) {
		key := subscriptionID(endpoint)
		sub, ok := data.Subscriptions[key]
		if !ok {
			return false, errNotFound
		}
		if len(data.Queue) >= maxPending {
			return false, errCapacity
		}
		tests := data.Tests[:0]
		for _, at := range data.Tests {
			if now.UnixMilli()-at < 60_000 {
				tests = append(tests, at)
			}
		}
		if len(tests) >= 10 || (sub.LastTestMS != 0 && now.UnixMilli()-sub.LastTestMS < 60_000) {
			return false, errRateLimit
		}
		data.Tests = append(tests, now.UnixMilli())
		sub.LastTestMS = now.UnixMilli()
		data.Subscriptions[key] = sub
		data.Queue = append(data.Queue, delivery{ID: payload.EventKey + ":" + key, SubscriptionID: key, Payload: payload})
		return true, nil
	})
}
