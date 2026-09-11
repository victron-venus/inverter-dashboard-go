package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// Config holds HTTPS IGW client settings (Cloudflare Access + bearer).
type Config struct {
	URL                string
	AccessClientID     string
	AccessClientSecret string
	APIToken           string
	PollInterval       time.Duration
	MapOptions         MapOptions
}

// Client polls GET /v1/snapshot and can POST /v1/commands/{name}.
type Client struct {
	cfg        Config
	http       *http.Client
	apply      func(*state.State)
	onStatus   func(connected bool)
	connected  atomic.Bool
	stopCh     chan struct{}
	stopOnce   sync.Once
	mapOptions MapOptions
}

// NewClient builds an IGW HTTP client. apply receives mapped dashboard state.
func NewClient(cfg Config, apply func(*state.State), onStatus func(connected bool)) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if base == "" {
		return nil, fmt.Errorf("gateway URL is required")
	}
	if strings.TrimSpace(cfg.AccessClientID) == "" || strings.TrimSpace(cfg.AccessClientSecret) == "" {
		return nil, fmt.Errorf("cloudflare Access client id and secret are required")
	}
	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	cfg.URL = base
	cfg.PollInterval = interval
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 25 * time.Second,
		},
		apply:      apply,
		onStatus:   onStatus,
		stopCh:     make(chan struct{}),
		mapOptions: cfg.MapOptions,
	}, nil
}

// IsConnected reports whether the last poll succeeded.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// Start begins the background poll loop. Non-blocking.
func (c *Client) Start() {
	go c.pollLoop()
}

// Stop ends the poll loop.
func (c *Client) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

func (c *Client) pollLoop() {
	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	c.pollOnce()

	for {
		select {
		case <-c.stopCh:
			c.connected.Store(false)
			log.Printf("[gateway] poller stopped")
			return
		case <-ticker.C:
			c.pollOnce()
		}
	}
}

func (c *Client) pollOnce() {
	snap, err := c.FetchSnapshot(context.Background())
	if err != nil {
		was := c.connected.Swap(false)
		log.Printf("[gateway] poll failed: %v", err)
		if was {
			log.Printf("[gateway] marked disconnected")
		}
		if c.onStatus != nil {
			c.onStatus(false)
		}
		return
	}
	mapped := SnapshotToState(snap, c.mapOptions)
	if !c.connected.Swap(true) {
		log.Printf("[gateway] connected to %s", c.cfg.URL)
	}
	if c.onStatus != nil {
		c.onStatus(true)
	}
	if c.apply != nil {
		c.apply(mapped)
	}
}

func (c *Client) authHeaders(req *http.Request) {
	req.Header.Set("CF-Access-Client-Id", c.cfg.AccessClientID)
	req.Header.Set("CF-Access-Client-Secret", c.cfg.AccessClientSecret)
	if tok := strings.TrimSpace(c.cfg.APIToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "inverter-dashboard-go/gateway")
}

// FetchSnapshot performs GET /v1/snapshot.
func (c *Client) FetchSnapshot(ctx context.Context) (*Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL+"/v1/snapshot", nil)
	if err != nil {
		return nil, err
	}
	c.authHeaders(req)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot HTTP %d: %s", res.StatusCode, truncate(string(body), 200))
	}
	var snap Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	return &snap, nil
}

// PostCommand performs POST /v1/commands/{name} for IGW-whitelisted commands.
func (c *Client) PostCommand(ctx context.Context, name string, body any) error {
	if !IsWhitelistedCommand(name) {
		return fmt.Errorf("%w: %s", ErrCommandNotOnGateway, name)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if len(payload) == 0 || string(payload) == "null" {
		payload = []byte("{}")
	}
	url := fmt.Sprintf("%s/v1/commands/%s", c.cfg.URL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.authHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("command %s HTTP %d: %s", name, res.StatusCode, truncate(string(respBody), 200))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
