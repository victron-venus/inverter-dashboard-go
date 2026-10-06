package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type Dispatcher struct {
	store    *Store
	subject  string
	client   webpush.HTTPClient
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	started  bool
	inflight map[string]bool
}

func newDispatcher(store *Store, subject string, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{store: store, subject: subject, client: newPushHTTPClient(), logger: logger, ctx: ctx, cancel: cancel, done: make(chan struct{}), inflight: map[string]bool{}}
}

func (d *Dispatcher) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started = true
	go d.run()
}

func (d *Dispatcher) Close() {
	d.cancel()
	d.mu.Lock()
	started := d.started
	d.mu.Unlock()
	if started {
		<-d.done
	}
	if client, ok := d.client.(*http.Client); ok {
		client.CloseIdleConnections()
	}
}

func (d *Dispatcher) run() {
	defer close(d.done)
	var workers sync.WaitGroup
	defer workers.Wait()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
		}
		for _, item := range d.pending(time.Now()) {
			d.mu.Lock()
			if len(d.inflight) >= 2 {
				d.mu.Unlock()
				break
			}
			if d.inflight[item.ID] {
				d.mu.Unlock()
				continue
			}
			d.inflight[item.ID] = true
			d.mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				d.deliver(item, time.Now())
				d.mu.Lock()
				delete(d.inflight, item.ID)
				d.mu.Unlock()
			}()
		}
	}
}

func (d *Dispatcher) pending(now time.Time) []delivery {
	var items []delivery
	err := d.store.mutate(func(data *diskState) (bool, error) {
		changed := false
		kept := data.Queue[:0]
		for _, item := range data.Queue {
			if !item.Payload.current(now) || item.Attempts >= 3 || (item.Payload.Kind != "test" && item.Epoch != data.Epoch) {
				changed = true
				continue
			}
			kept = append(kept, item)
			if item.NextAttemptMS <= now.UnixMilli() {
				items = append(items, item)
			}
		}
		data.Queue = kept
		return changed, nil
	})
	if err != nil {
		return nil
	}
	return items
}

// An authoritative read checks a retired epoch,
// removed subscription, changed preference or expired event cannot be dispatched
// from an earlier queue snapshot.
func (d *Dispatcher) current(item delivery, now time.Time) (storedSubscription, string, string, bool) {
	d.store.mu.Lock()
	defer d.store.mu.Unlock()
	if d.store.closed || d.store.failed || !item.Payload.current(now) {
		return storedSubscription{}, "", "", false
	}
	sub, exists := d.store.data.Subscriptions[item.SubscriptionID]
	if !exists || !sub.Preferences.allows(item.Payload.Kind) || (item.Payload.Kind != "test" && item.Epoch != d.store.data.Epoch) {
		return storedSubscription{}, "", "", false
	}
	for _, queued := range d.store.data.Queue {
		if queued.ID == item.ID && queued.Attempts == item.Attempts {
			return sub, d.store.data.PrivateKey, d.store.data.PublicKey, true
		}
	}
	return storedSubscription{}, "", "", false
}

func (d *Dispatcher) deliver(item delivery, now time.Time) {
	sub, private, public, ok := d.current(item, now)
	if !ok || d.ctx.Err() != nil {
		return
	}
	raw, err := json.Marshal(item.Payload)
	if err != nil || len(raw) > maxPayloadBytes {
		d.finish(item, 400, false, now)
		return
	}
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	d.store.mu.Lock()
	d.store.active[item.ID] = cancel
	d.store.mu.Unlock()
	defer func() { cancel(); d.store.mu.Lock(); delete(d.store.active, item.ID); d.store.mu.Unlock() }()
	ttl := int(time.Until(time.UnixMilli(item.Payload.SourceTimestampMS).Add(maxEventAge)).Seconds())
	if ttl < 1 {
		return
	}
	if ttl > 300 {
		ttl = 300
	}
	response, err := webpush.SendNotificationWithContext(ctx, raw, &sub.Subscription, &webpush.Options{
		HTTPClient: guardedClient{dispatcher: d, item: item, client: d.client}, Subscriber: d.subject, VAPIDPrivateKey: private, VAPIDPublicKey: public,
		TTL: ttl, Urgency: webpush.UrgencyNormal,
	})
	status := 0
	if response != nil {
		status = response.StatusCode
		// Never log or retain provider bodies; their URLs can contain secrets.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		_ = response.Body.Close()
	}
	transient := err != nil || status == http.StatusTooManyRequests || status >= 500
	d.logAttempt(item, status, err)
	d.finish(item, status, transient, time.Now())
}

func (d *Dispatcher) finish(item delivery, status int, transient bool, now time.Time) {
	_ = d.store.mutate(func(data *diskState) (bool, error) {
		present := false
		for _, queued := range data.Queue {
			if queued.ID == item.ID && queued.Attempts == item.Attempts {
				present = true
				break
			}
		}
		if !present {
			return false, nil
		}
		if status == http.StatusNotFound || status == http.StatusGone {
			delete(data.Subscriptions, item.SubscriptionID)
		}
		kept := data.Queue[:0]
		changed := false
		for _, queued := range data.Queue {
			if _, exists := data.Subscriptions[queued.SubscriptionID]; !exists {
				changed = true
				continue
			}
			if queued.ID != item.ID {
				kept = append(kept, queued)
				continue
			}
			changed = true
			if transient && queued.Attempts < 2 && queued.Payload.current(now) && (queued.Payload.Kind == "test" || queued.Epoch == data.Epoch) {
				queued.Attempts++
				queued.NextAttemptMS = now.Add(time.Duration(queued.Attempts*5) * time.Second).UnixMilli()
				kept = append(kept, queued)
			}
		}
		data.Queue = kept
		return changed, nil
	})
}

func validateSubject(subject string) error {
	if len(subject) > 512 {
		return errors.New("invalid Web Push subject")
	}
	// An HTTPS project/contact URL is interoperable across browser providers.
	request, err := http.NewRequest(http.MethodGet, subject, nil)
	if err != nil || request.URL.Scheme != "https" || request.URL.Hostname() == "" || request.URL.User != nil || request.URL.Fragment != "" {
		return errors.New("web push subject must be an HTTPS contact URL")
	}
	return nil
}

// The library calls this only after encryption. DNS/TLS/network work carries the
// registered delivery context, canceled atomically when its queue item retires.
type guardedClient struct {
	dispatcher *Dispatcher
	item       delivery
	client     webpush.HTTPClient
}

func (g guardedClient) Do(req *http.Request) (*http.Response, error) {
	if _, _, _, ok := g.dispatcher.current(g.item, time.Now()); !ok || req.Context().Err() != nil {
		return nil, errNotificationRetired
	}
	ttl := int(time.Until(time.UnixMilli(g.item.Payload.SourceTimestampMS).Add(maxEventAge)).Seconds())
	if ttl < 1 {
		return nil, errNotificationExpired
	}
	if ttl > 300 {
		ttl = 300
	}
	req.Header.Set("TTL", strconv.Itoa(ttl))
	return g.client.Do(req)
}
