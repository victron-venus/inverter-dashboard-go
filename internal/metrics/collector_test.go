package metrics

import (
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestUpdateFromState(t *testing.T) {
	c := DefaultCollector
	st := &state.State{
		SolarTotal: 1234.0,
		BatterySOC: 56.7,
		GT:         -500.0,
	}
	c.UpdateFromState(st)
	// No panic check
}

func TestUpdateWebsocketClients(t *testing.T) {
	c := DefaultCollector
	c.UpdateWebsocketClients()
}

func TestUpdateMqttCmdBuffer(t *testing.T) {
	c := DefaultCollector
	stats := map[string]interface{}{
		"count":     5,
		"capacity":  10,
		"utilization": 50.0,
	}
	c.UpdateMqttCmdBuffer(stats)
}
