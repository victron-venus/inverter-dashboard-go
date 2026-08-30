package mockmqtt

import (
	"sync"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// Client is a fake MQTT client for testing.
type Client struct {
	mu        sync.RWMutex
	state     *state.State
	console   []string
	published []publishedCmd
}

type publishedCmd struct {
	Action  string
	Payload map[string]interface{}
}

// NewClient returns a ready-to-use fake.
func NewClient() *Client {
	return &Client{
		state:     &state.State{},
		published: nil,
	}
}

func (c *Client) GetState() *state.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

func (c *Client) GetConsole() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.console
}

func (c *Client) SetState(s *state.State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = s
}

func (c *Client) SetConsole(lines []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.console = lines
}

func (c *Client) Published() []publishedCmd {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]publishedCmd, len(c.published))
	copy(out, c.published)
	return out
}

// PublishCommand records the call and returns nil.
func (c *Client) PublishCommand(action string, payload interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var p map[string]interface{}
	if payload != nil {
		p, _ = payload.(map[string]interface{})
	}
	c.published = append(c.published, publishedCmd{Action: action, Payload: p})
	return nil
}
