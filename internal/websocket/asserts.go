package websocket

import (
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
)

// Compile-time assertions that the real clients satisfy our interfaces.
// Method drift in either client breaks the build here.
var (
	_ MQTTCommander = (*mqtt.Client)(nil)
	_ HAClient      = (*homeassistant.Client)(nil)
)
