package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/websocket"
)

// Collector holds all Prometheus metrics for the dashboard
type Collector struct {
	// Solar power in watts
	SolarWatts prometheus.Gauge
	// Battery state of charge percentage
	BatterySOC prometheus.Gauge
	// Grid power in watts (positive = import, negative = export)
	GridWatts prometheus.Gauge
	// Active WebSocket clients
	WebsocketClients prometheus.Gauge
	// MQTT command buffer stats
	MqttCmdBufferSize       prometheus.Gauge
	MqttCmdBufferCapacity   prometheus.Gauge
	MqttCmdBufferUtilization prometheus.Gauge
}

// NewCollector creates and registers all metrics
func NewCollector() *Collector {
	c := &Collector{
		SolarWatts: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "victron_solar_watts",
			Help: "Current solar power production in watts",
		}),
		BatterySOC: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "victron_battery_soc",
			Help: "Current battery state of charge percentage",
		}),
		GridWatts: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "victron_grid_watts",
			Help: "Current grid power in watts (positive = import, negative = export)",
		}),
		WebsocketClients: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_active_clients",
			Help: "Number of active WebSocket connections",
		}),
		MqttCmdBufferSize: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "mqtt_command_buffer_size",
			Help: "Current number of commands in the MQTT command buffer",
		}),
		MqttCmdBufferCapacity: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "mqtt_command_buffer_capacity",
			Help: "Maximum capacity of the MQTT command buffer",
		}),
		MqttCmdBufferUtilization: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "mqtt_command_buffer_utilization",
			Help: "MQTT command buffer utilization percentage (0-100)",
		}),
	}
	return c
}

// UpdateFromState updates all metrics from the current state
func (c *Collector) UpdateFromState(st *state.State) {
	if st == nil {
		return
	}
	// Solar total power (solar production)
	c.SolarWatts.Set(st.SolarTotal)
	// Battery SOC
	c.BatterySOC.Set(st.BatterySOC)
	// Grid power (GT)
	c.GridWatts.Set(st.GT)
}

// UpdateWebsocketClients updates the active WebSocket client count
func (c *Collector) UpdateWebsocketClients() {
	c.WebsocketClients.Set(float64(websocket.GetConnectedCount()))
}

// UpdateMqttCmdBuffer updates MQTT command buffer metrics
func (c *Collector) UpdateMqttCmdBuffer(stats map[string]interface{}) {
	if stats == nil {
		return
	}
	if size, ok := stats["count"].(int); ok {
		c.MqttCmdBufferSize.Set(float64(size))
	}
	if cap, ok := stats["capacity"].(int); ok {
		c.MqttCmdBufferCapacity.Set(float64(cap))
	}
	if util, ok := stats["utilization"].(float64); ok {
		c.MqttCmdBufferUtilization.Set(util)
	}
}

// DefaultCollector is the global metrics collector
var DefaultCollector = NewCollector()