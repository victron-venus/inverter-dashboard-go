package mockha

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
)

func TestClient_SetOverlay(t *testing.T) {
	c := NewClient()
	c.SetOverlay(homeassistant.Overlay{HADirectConnected: true})
	if !c.GetOverlay().HADirectConnected {
		t.Error("overlay not set")
	}
}

func TestClient_PressButton(t *testing.T) {
	c := NewClient()
	if err := c.PressButton("button.foo"); err != nil {
		t.Fatal(err)
	}
	if c.PressButtonCalled() != "button.foo" {
		t.Errorf("PressButtonCalled=%q, want button.foo", c.PressButtonCalled())
	}
}

func TestClient_ManagedKeys(t *testing.T) {
	c := NewClient()
	c.SetManagedKeys([]string{"k1", "k2"})
	keys := c.GetManagedKeys()
	if len(keys) != 2 {
		t.Fatalf("len=%d, want 2", len(keys))
	}
}
