package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
)

// MessageHandler is a function type for handling state updates
type MessageHandler func()

// Client wraps the MQTT client and provides thread-safe state management
type Client struct {
	client          mqtt.Client
	broker          string
	port            int
	state           *state.State
	handler         MessageHandler
	handlerMu       sync.RWMutex
	stateMu         sync.RWMutex
	consoleLines    []string
	consoleMu       sync.RWMutex
	maxConsoleLines int
	lastStateTime   time.Time
	lastStateMu     sync.RWMutex
	cmdBuffer       *CommandBuffer

	// dbus-pump water topics (Cerbo MQTT); empty portal disables
	portalID      string
	tankInstance  int
	pumpInstance  int
	valveInstance int

	// Victron alarm tracking (N/<portal>/.../Alarms/<name> -> last value)
	alarmValues map[string]int
	alarmsMu    sync.Mutex

	// AC PV inverters of any vendor discovered on the GX broker
	// (N/<portal>/pvinverter/<instance>/<path>), keyed by instance.
	pvInverters map[int]*state.Charger

	// camera events (optional Frigate topic; empty disables)
	cameraTopic string
}

// NewClient creates a new MQTT client instance with Python-equivalent defaults
func NewClient(broker string, port int) *Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	// Random suffix keeps the client ID unique so a second instance or a
	// stale broker session cannot kick this client off the broker.
	opts.SetClientID(fmt.Sprintf("inverter-dashboard-%06x", rand.Uint64()&(1<<24-1)))
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5)

	client := &Client{
		client: mqtt.NewClient(opts),
		broker: broker,
		port:   port,
		state: &state.State{
			Booleans:         make(map[string]interface{}),
			Features:         make(map[string]interface{}),
			DailyStats:       state.DailyStats{},
			ESSMode:          state.ESSMode{},
			DashboardVersion: "dev",
			Version:          "0.0.0",
			Console:          make([]string, 0),
		},
		consoleLines:    make([]string, 0),
		maxConsoleLines: 50,
		pvInverters:     make(map[int]*state.Charger),
	}

	// Initialize command buffer with capacity of 1000 commands
	client.cmdBuffer = NewCommandBuffer(1000, client)

	return client
}

func (c *Client) GetIP() string { return c.broker }
func (c *Client) GetPort() int  { return c.port }
func (c *Client) IsConnected() bool {
	return c.client != nil && c.client.IsConnected()
}
func (c *Client) LastStateTime() time.Time {
	c.lastStateMu.RLock()
	defer c.lastStateMu.RUnlock()
	return c.lastStateTime
}
func (c *Client) GetState() *state.State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

// GetCmdBufferStats returns command buffer statistics
func (c *Client) GetCmdBufferStats() map[string]interface{} {
	if c.cmdBuffer == nil {
		return nil
	}
	return c.cmdBuffer.Stats()
}

func (c *Client) GetConsole() []string {
	c.consoleMu.RLock()
	defer c.consoleMu.RUnlock()
	size := len(c.consoleLines)
	if size == 0 {
		return []string{}
	}
	start := 0
	if size > 20 {
		start = size - 20
	}
	result := make([]string, size-start)
	copy(result, c.consoleLines[start:])
	return result
}

func (c *Client) Connect() error {
	if c.client == nil {
		return fmt.Errorf("mqtt client not initialized")
	}
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to mqtt broker: %w", token.Error())
	}
	log.Printf("Connected to MQTT broker")

	// Start command buffer worker
	if c.cmdBuffer != nil {
		c.cmdBuffer.Start()
	}

	return nil
}

// SetWaterConfig selects the dbus-pump water topics to subscribe
// (N/<portal>/tank/<tank>/Level, N/<portal>/pump/<N>/State).
// SetCameraTopic enables camera event subscription on the given MQTT filter.
func (c *Client) SetCameraTopic(topic string) {
	c.cameraTopic = topic
}

func (c *Client) SetWaterConfig(portalID string, tank, pump, valve int) {
	c.portalID = portalID
	c.tankInstance = tank
	c.pumpInstance = pump
	c.valveInstance = valve
}

func (c *Client) SetMessageHandler(handler MessageHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.handler = handler
}

// triggerHandler calls the state update callback asynchronously (matches Python's asyncio.run_coroutine_threadsafe)
func (c *Client) triggerHandler() {
	c.handlerMu.RLock()
	handler := c.handler
	c.handlerMu.RUnlock()

	if handler != nil {
		// Execute in goroutine to match Python's async callback pattern
		go handler()
	}
}

func (c *Client) Subscribe() error {
	if token := c.client.Subscribe("inverter/state", 0, c.onStateMessage); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to inverter/state: %w", token.Error())
	}
	if token := c.client.Subscribe("inverter/console", 0, c.onConsoleMessage); token.Wait() && token.Error() != nil {
		log.Printf("Warning: failed to subscribe to inverter/console: %v", token.Error())
	}
	if token := c.client.Subscribe("inverter/notifications", 0, c.onNotificationMessage); token.Wait() && token.Error() != nil {
		log.Printf("Warning: failed to subscribe to inverter/notifications: %v", token.Error())
	}

	// AC PV inverters of any vendor (Tasmota, ESPHome, ...) published on the
	// GX broker — tiles stay alive even when inverter-control is down.
	if token := c.client.Subscribe("N/+/pvinverter/+/#", 0, c.onPvInverterMessage); token.Wait() && token.Error() != nil {
		log.Printf("Warning: failed to subscribe to N/+/pvinverter topics: %v", token.Error())
	}

	if c.portalID != "" {
		if token := c.client.Subscribe(fmt.Sprintf("N/%s/+/Alarms/#", c.portalID), 0, c.onAlarmMessage); token.Wait() && token.Error() != nil {
			log.Printf("Warning: failed to subscribe to Victron alarm topics: %v", token.Error())
		}
		if token := c.client.Subscribe(fmt.Sprintf("N/%s/tank/+/Level", c.portalID), 0, c.onWaterMessage); token.Wait() && token.Error() != nil {
			log.Printf("Warning: failed to subscribe to water level topic: %v", token.Error())
		}
		if token := c.client.Subscribe(fmt.Sprintf("N/%s/pump/+/State", c.portalID), 0, c.onWaterMessage); token.Wait() && token.Error() != nil {
			log.Printf("Warning: failed to subscribe to pump state topic: %v", token.Error())
		}
		log.Printf("Subscribed to Cerbo water topics (portal %s)", c.portalID)
	}
	if c.cameraTopic != "" {
		if token := c.client.Subscribe(c.cameraTopic, 0, c.onCameraMessage); token.Wait() && token.Error() != nil {
			log.Printf("Warning: failed to subscribe to %s: %v", c.cameraTopic, token.Error())
		} else {
			log.Printf("Subscribed to camera events on %s", c.cameraTopic)
		}
	}
	log.Printf("Subscribed to MQTT topics")
	return nil
}

// onWaterMessage decodes dbus-pump water topics into the shared state.
// Topic shapes: N/<portal>/tank/<instance>/Level and
// N/<portal>/pump/<instance>/State, payload {"value": <num>}.
func (c *Client) onWaterMessage(client mqtt.Client, msg mqtt.Message) {
	if c.portalID == "" {
		return
	}
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) < 5 || parts[0] != "N" || parts[1] != c.portalID {
		return
	}
	var payload struct {
		Value interface{} `json:"value"`
	}
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		return
	}
	num, ok := toFloat(payload.Value)
	if !ok {
		return
	}

	c.stateMu.Lock()
	st := c.state
	switch {
	case parts[2] == "tank" && parts[3] == strconv.Itoa(c.tankInstance) && parts[4] == "Level":
		st.WaterLevel = num
	// Venus bridges pump.startstop services as N/<portal>/pump/<instance>/State
	case len(parts) >= 5 && parts[2] == "pump" && parts[4] == "State" && parts[3] == strconv.Itoa(c.valveInstance):
		st.WaterValve = num != 0
	case len(parts) >= 5 && parts[2] == "pump" && parts[4] == "State" && parts[3] == strconv.Itoa(c.pumpInstance):
		st.PumpSwitch = num != 0
	default:
		c.stateMu.Unlock()
		return
	}
	c.stateMu.Unlock()
	c.triggerHandler()
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// onPvInverterMessage decodes GX PV-inverter topics into the shared state.
// Topic shape: N/<portal>/pvinverter/<instance>/<path...>, payload
// {"value": <num|str>}. Works with any vendor's dbus publisher.
func (c *Client) onPvInverterMessage(client mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) < 5 || parts[0] != "N" || parts[2] != "pvinverter" {
		return
	}
	inst, err := strconv.Atoi(parts[3])
	if err != nil {
		return
	}
	path := strings.Join(parts[4:], "/")

	var payload struct {
		Value interface{} `json:"value"`
	}
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		return
	}

	c.stateMu.Lock()
	entry, ok := c.pvInverters[inst]
	if !ok {
		entry = &state.Charger{}
		c.pvInverters[inst] = entry
	}
	changed := false
	switch path {
	case "Ac/Power", "Ac/L1/Power":
		if num, okNum := toFloat(payload.Value); okNum && entry.Power != num {
			entry.Power = num
			changed = true
		}
	case "Ac/L1/Voltage":
		if num, okNum := toFloat(payload.Value); okNum && entry.PVVoltage != num {
			entry.PVVoltage = num
			changed = true
		}
	case "Ac/L1/Current":
		if num, okNum := toFloat(payload.Value); okNum && entry.Current != num {
			entry.Current = num
			changed = true
		}
	case "ProductName":
		if name, okStr := payload.Value.(string); okStr && name != "" && entry.Name != name {
			entry.Name = name
			changed = true
		}
	}
	if changed {
		instances := make([]int, 0, len(c.pvInverters))
		for i := range c.pvInverters {
			instances = append(instances, i)
		}
		sort.Ints(instances)
		list := make([]state.Charger, 0, len(instances))
		for _, i := range instances {
			list = append(list, *c.pvInverters[i])
		}
		st := c.state
		st.PvInverters = list
		c.stateMu.Unlock()
		c.triggerHandler()
		return
	}
	c.stateMu.Unlock()
}

func (c *Client) PublishCommand(action string, payload interface{}) error {
	topic := fmt.Sprintf("inverter/cmd/%s", action)
	var message string
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		message = string(data)
	}
	if token := c.client.Publish(topic, 0, false, message); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, token.Error())
	}
	log.Printf("Published command to %s", topic)
	return nil
}

// PublishCommandAsync publishes a command asynchronously via the command buffer.
// Returns immediately; the command will be sent when the broker is available.
func (c *Client) PublishCommandAsync(action string, payload interface{}) error {
	if c.cmdBuffer == nil {
		return fmt.Errorf("command buffer not initialized")
	}
	return c.cmdBuffer.Enqueue(action, payload)
}

func (c *Client) Disconnect() {
	// Stop command buffer worker
	if c.cmdBuffer != nil {
		c.cmdBuffer.Stop()
	}

	if c.client != nil && c.client.IsConnected() {
		c.client.Disconnect(250)
		log.Printf("Disconnected from MQTT broker")
	}
}

func (c *Client) onStateMessage(client mqtt.Client, msg mqtt.Message) {
	var data map[string]interface{}
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to unmarshal state message: %v", err)
		return
	}

	c.lastStateMu.Lock()
	c.lastStateTime = time.Now()
	c.lastStateMu.Unlock()

	c.stateMu.Lock()
	st := c.state

	// Set version info
	st.DashboardVersion = version.GetCurrent()
	if ver, ok := data["version"].(string); ok {
		st.Version = ver
	}

	// Unmarshal directly into State struct - JSON tags match field names
	// Re-marshal data to JSON then unmarshal into struct to handle type conversions
	dataJSON, _ := json.Marshal(data)
	if err := json.Unmarshal(dataJSON, st); err != nil {
		log.Printf("Failed to unmarshal state into struct: %v", err)
	}
	c.stateMu.Unlock()

	// Log values
	log.Printf("State update - solar: %.2fW, grid: %.2fW, battery: %.2f%%, cons: %.2fW",
		st.SolarTotal, st.GT, st.BatterySOC, st.TT)

	// Trigger handler asynchronously (matches Python's asyncio pattern)
	c.triggerHandler()
}

func (c *Client) onConsoleMessage(client mqtt.Client, msg mqtt.Message) {
	line := string(msg.Payload())
	c.consoleMu.Lock()
	c.consoleLines = append(c.consoleLines, line)
	if len(c.consoleLines) > c.maxConsoleLines {
		c.consoleLines = c.consoleLines[len(c.consoleLines)-c.maxConsoleLines:]
	}
	c.consoleMu.Unlock()
	log.Printf("Received console line: %s", line)
}
