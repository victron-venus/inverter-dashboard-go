package mockha

import "github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"

// Client is a fake HA client for testing.
type Client struct {
	directMode        bool
	overlay           homeassistant.Overlay
	uiConfig          map[string]interface{}
	booleanButtons    []homeassistant.Button
	managedKeys       []string
	toggleAllowed     map[string]bool
	pressButtonCalled string
}

func NewClient() *Client {
	return &Client{
		toggleAllowed: make(map[string]bool),
		uiConfig:      make(map[string]interface{}),
	}
}

func (c *Client) IsDirectMode() bool { return c.directMode }
func (c *Client) IsToggleAllowed(id string) bool {
	if c.toggleAllowed == nil {
		return false
	}
	return c.toggleAllowed[id]
}
func (c *Client) GetOverlay() homeassistant.Overlay         { return c.overlay }
func (c *Client) GetUIConfig() map[string]interface{}       { return c.uiConfig }
func (c *Client) GetBooleanButtons() []homeassistant.Button { return c.booleanButtons }
func (c *Client) GetManagedKeys() []string                  { return c.managedKeys }
func (c *Client) PressButtonCalled() string                 { return c.pressButtonCalled }

// Setters for configuring the fake.
func (c *Client) SetDirectMode(v bool)                 { c.directMode = v }
func (c *Client) SetOverlay(o homeassistant.Overlay)   { c.overlay = o }
func (c *Client) SetUIConfig(m map[string]interface{}) { c.uiConfig = m }
func (c *Client) SetManagedKeys(keys []string)         { c.managedKeys = keys }
func (c *Client) SetToggleAllowed(id string, allowed bool) {
	c.toggleAllowed[id] = allowed
}

// PressButton records the call.
func (c *Client) PressButton(entityID string) error {
	c.pressButtonCalled = entityID
	return nil
}

// ToggleEntity is a no-op for tests.
func (c *Client) ToggleEntity(_ string) error { return nil }

// FetchStatesOnce returns the current overlay.
func (c *Client) FetchStatesOnce() (homeassistant.Overlay, error) {
	return c.overlay, nil
}

// ReplaceOverlay updates the stored overlay.
func (c *Client) ReplaceOverlay(o homeassistant.Overlay) {
	c.overlay = o
}
