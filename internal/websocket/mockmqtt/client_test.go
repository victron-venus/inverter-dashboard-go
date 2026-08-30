package mockmqtt

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c.GetState() == nil {
		t.Fatal("GetState returned nil")
	}
	if len(c.GetConsole()) != 0 {
		t.Fatal("GetConsole returned non-empty")
	}
}

func TestPublishCommand(t *testing.T) {
	c := NewClient()
	err := c.PublishCommand("toggle", map[string]interface{}{"entity": "light.foo"})
	if err != nil {
		t.Fatal(err)
	}
	pub := c.Published()
	if len(pub) != 1 {
		t.Fatalf("len=%d, want 1", len(pub))
	}
	if pub[0].Action != "toggle" {
		t.Errorf("action=%q, want %q", pub[0].Action, "toggle")
	}
	if pub[0].Payload["entity"] != "light.foo" {
		t.Errorf("entity=%v, want light.foo", pub[0].Payload["entity"])
	}
}

func TestPublishCommand_Multiple(t *testing.T) {
	c := NewClient()
	if err := c.PublishCommand("toggle", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.PublishCommand("setpoint", map[string]interface{}{"value": 42.0}); err != nil {
		t.Fatal(err)
	}
	pub := c.Published()
	if len(pub) != 2 {
		t.Fatalf("len=%d, want 2", len(pub))
	}
	if pub[0].Action != "toggle" {
		t.Errorf("first action=%q", pub[0].Action)
	}
	if pub[1].Action != "setpoint" {
		t.Errorf("second action=%q", pub[1].Action)
	}
}
