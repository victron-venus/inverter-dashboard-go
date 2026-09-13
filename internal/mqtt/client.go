package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// MessageHandler is a function type for handling state updates
type MessageHandler func()

// Client wraps the MQTT client and provides thread-safe state management
type Client struct {
	client           mqtt.Client
	broker           string
	port             int
	state            *state.State
	handler          MessageHandler
	handlerMu        sync.RWMutex
	broadcastPending int32
	stateMu          sync.RWMutex
	consoleLines     []string
	consoleMu        sync.RWMutex
	maxConsoleLines  int
	lastStateTime    time.Time
	lastStateMu      sync.RWMutex
	cmdBuffer        *CommandBuffer

	// Cerbo identity; empty portal enables native MQTT discovery.
	portalID      string
	tankInstance  int
	pumpInstance  int
	valveInstance int

	// Selected vehicle and wallbox instances on the same Cerbo portal.
	evInstance        int
	evchargerInstance int

	// Victron alarm tracking (N/<portal>/.../Alarms/<name> -> last value)
	alarmValues map[string]int
	alarmsMu    sync.Mutex

	// Venus-platform notification slots (GUIv2). Once any platform message
	// arrives, raw Alarms/* banners are suppressed (desktop parity).
	platformMu         sync.Mutex
	platformSlots      map[string]*platformSlotState
	platformNotifsSeen bool

	// Cerbo state is cached by service, instance and path. Null leaves remain
	// present to distinguish unavailable measurements from undiscovered data.
	cerboLeaves           map[string]map[string]interface{}
	cerboOwned            map[string]bool
	cameraTopic           string
	keepaliveStop         chan struct{}
	keepaliveMu           sync.Mutex
	keepaliveNeedsRefresh bool
	subscribeMu           sync.Mutex
	subscriptionsEnabled  atomic.Bool

	// IGW-only mode: no Cerbo MQTT dial; ApplyState + gateway publisher.
	gatewayMode      bool
	gatewayConnected bool
	gatewayMu        sync.RWMutex
	gatewayPublish   func(action string, payload interface{}) error
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
	opts.SetConnectRetryInterval(5 * time.Second)

	client := &Client{
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
			CarSOC:           0,
		},
		consoleLines:    make([]string, 0),
		maxConsoleLines: 50,
	}

	// Clean sessions lose subscriptions on reconnect. Subscribe runs outside
	// Paho's message router; waiting for SUBACK inside a router callback can deadlock.
	opts.SetOnConnectHandler(func(_ mqtt.Client) {
		if client.subscriptionsEnabled.Load() {
			if err := client.Subscribe(); err != nil {
				log.Printf("MQTT resubscribe: %v", err)
			}
		}
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
		client.stopKeepalive()
		client.invalidateCerbo()
	})
	client.client = mqtt.NewClient(opts)
	client.initCerboMaps()

	// Initialize command buffer with capacity of 1000 commands
	client.cmdBuffer = NewCommandBuffer(1000, client)

	return client
}

func (c *Client) GetIP() string { return c.broker }
func (c *Client) GetPort() int  { return c.port }
func (c *Client) IsConnected() bool {
	c.gatewayMu.RLock()
	gm := c.gatewayMode
	gc := c.gatewayConnected
	c.gatewayMu.RUnlock()
	if gm {
		return gc
	}
	// Paho IsConnected also means "will reconnect". Health and immediate
	// controls require a currently open broker connection.
	return c.client != nil && c.client.IsConnectionOpen()
}

// EnableGatewayMode marks this client as IGW-fed (skip MQTT dial semantics).
func (c *Client) EnableGatewayMode() {
	c.gatewayMu.Lock()
	c.gatewayMode = true
	c.gatewayMu.Unlock()
}

// DisableGatewayMode returns the client to Cerbo MQTT health semantics
// (used when dual-path recovery probes succeed and IGW is stopped).
func (c *Client) DisableGatewayMode() {
	c.gatewayMu.Lock()
	c.gatewayMode = false
	c.gatewayConnected = false
	c.gatewayMu.Unlock()
}

// SetGatewayConnected updates health for IGW mode.
func (c *Client) SetGatewayConnected(v bool) {
	c.gatewayMu.Lock()
	changed := c.gatewayConnected != v
	c.gatewayConnected = v
	c.gatewayMu.Unlock()
	// Failed gateway polls have no ApplyState call. Push their transport
	// transition to existing WebSockets while retaining the last snapshot.
	if changed {
		c.triggerHandler()
	}
}

// SetGatewayPublisher routes PublishCommand to IGW when set.
// Pass nil to clear (MQTT W/ / inverter/cmd path resumes).
func (c *Client) SetGatewayPublisher(fn func(action string, payload interface{}) error) {
	c.gatewayMu.Lock()
	c.gatewayPublish = fn
	c.gatewayMu.Unlock()
}

// ApplyState replaces Cerbo telemetry state from an external source (IGW)
// and triggers the WebSocket broadcast handler. Preserves console, version,
// notifications, camera, forecast and controller metadata when a telemetry snapshot omits them.
func (c *Client) ApplyState(st *state.State) {
	if st == nil {
		return
	}
	c.stateMu.Lock()
	prev := c.state
	if prev != nil {
		if st.Version == "" {
			st.Version = prev.Version
		}
		if st.DashboardVersion == "" {
			st.DashboardVersion = prev.DashboardVersion
		}
		if len(st.Console) == 0 {
			st.Console = prev.Console
		}
		// nil = omitted (preserve); non-nil (incl. empty) = replace so cleared
		// alarms drop from the banner strip on the next IGW poll.
		if st.Notifications == nil {
			st.Notifications = prev.Notifications
		}
		if st.CameraEvent == nil {
			st.CameraEvent = prev.CameraEvent
		}
		if st.SolarForecast == nil {
			st.SolarForecast = prev.SolarForecast
		}
		if st.Booleans == nil {
			st.Booleans = prev.Booleans
			st.OnlyCharging, st.NoFeed, st.HouseSupport = prev.OnlyCharging, prev.NoFeed, prev.HouseSupport
			st.ChargeBattery, st.DoNotSupplyCharger = prev.ChargeBattery, prev.DoNotSupplyCharger
			st.SetLimitToEVCharger, st.MinimizeCharging, st.DryRun = prev.SetLimitToEVCharger, prev.MinimizeCharging, prev.DryRun
		}
		if reflect.ValueOf(st.DailyStats).IsZero() {
			st.DailyStats = prev.DailyStats
		}
		if reflect.ValueOf(st.ESSMode).IsZero() {
			st.ESSMode = prev.ESSMode
		}
		if st.UIConfig == nil {
			st.UIConfig = prev.UIConfig
		}
		if st.DVCCLimits == nil {
			st.DVCCLimits = prev.DVCCLimits
		}
		if st.Limits == nil {
			st.Limits = prev.Limits
		}
		if st.Perf == nil {
			st.Perf = prev.Perf
		}
		if st.LoopInterval == 0 {
			st.LoopInterval = prev.LoopInterval
		}
		if st.GridControlValid == nil {
			st.GridControlValid, st.GridControlReason = prev.GridControlValid, prev.GridControlReason
			st.GridLossState, st.GridLossHoldSeconds = prev.GridLossState, prev.GridLossHoldSeconds
			st.GridLossElapsed, st.GridLossRemaining = prev.GridLossElapsed, prev.GridLossRemaining
			st.GridLossZeroApplied = prev.GridLossZeroApplied
		}
		if st.Features == nil {
			st.Features = prev.Features
		}
	}
	c.state = st
	c.stateMu.Unlock()

	c.lastStateMu.Lock()
	c.lastStateTime = time.Now()
	c.lastStateMu.Unlock()

	c.triggerHandler()
}
func (c *Client) LastStateTime() time.Time {
	c.lastStateMu.RLock()
	defer c.lastStateMu.RUnlock()
	return c.lastStateTime
}
func (c *Client) GetState() *state.State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state.Clone()
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
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.portalID = strings.TrimSpace(portalID)
	c.tankInstance = tank
	c.pumpInstance = pump
	c.valveInstance = valve
}

func (c *Client) SetEVConfig(ev, evcharger int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.evInstance = ev
	c.evchargerInstance = evcharger
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

	if handler == nil {
		return
	}
	// Coalesce bursts, but run again when data changed during the callback.
	// Otherwise the last reading (especially a disconnect) can be lost forever.
	for {
		switch atomic.LoadInt32(&c.broadcastPending) {
		case 0:
			if !atomic.CompareAndSwapInt32(&c.broadcastPending, 0, 1) {
				continue
			}
			go func() {
				for {
					handler()
					if atomic.CompareAndSwapInt32(&c.broadcastPending, 1, 0) {
						return
					}
					atomic.StoreInt32(&c.broadcastPending, 1)
				}
			}()
			return
		case 1:
			if atomic.CompareAndSwapInt32(&c.broadcastPending, 1, 2) {
				return
			}
		case 2:
			return
		}
	}
}

func (c *Client) Subscribe() error {
	c.subscriptionsEnabled.Store(true)
	c.subscribeMu.Lock()
	defer c.subscribeMu.Unlock()
	if c.client == nil || !c.client.IsConnectionOpen() {
		return fmt.Errorf("mqtt not connected")
	}
	if portal := c.PortalID(); portal != "" && !validPortal(portal) {
		return fmt.Errorf("invalid Cerbo portal id")
	}
	// A clean MQTT session needs a fresh device inventory. Old retained daemon
	// values cannot resurrect readings that the previous Cerbo session owned.
	c.stateMu.Lock()
	c.cerboLeaves = nil
	c.applyCerboOverlays()
	c.stateMu.Unlock()
	for _, sub := range []struct {
		topic   string
		handler mqtt.MessageHandler
	}{
		{"inverter/state", c.onStateMessage}, {"inverter/console", c.onConsoleMessage},
		{"inverter/notifications", c.onNotificationMessage}, {"inverter/portal", c.onPortalMessage},
	} {
		if token := c.client.Subscribe(sub.topic, 0, sub.handler); token.Wait() && token.Error() != nil {
			log.Printf("Optional controller subscription %s failed: %v", sub.topic, token.Error())
		}
	}
	portal := c.PortalID()
	if portal == "" {
		portal = "+"
	}
	if err := c.subscribeNativeTopics(portal); err != nil {
		return err
	}
	if c.cameraTopic != "" {
		if token := c.client.Subscribe(c.cameraTopic, 0, c.onCameraMessage); token.Wait() && token.Error() != nil {
			return token.Error()
		}
	}
	c.startKeepalive()
	return nil
}

// Compatibility entry points share the same reducer as all native telemetry.
func (c *Client) onWaterMessage(_ mqtt.Client, msg mqtt.Message) { c.onCerboLiveMessage(nil, msg) }
func (c *Client) onEVMessage(_ mqtt.Client, msg mqtt.Message)    { c.onCerboLiveMessage(nil, msg) }
func (c *Client) onPvInverterMessage(_ mqtt.Client, msg mqtt.Message) {
	if strings.Contains(msg.Topic(), "/pvinverter/") {
		c.onCerboLiveMessage(nil, msg)
	}
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

func (c *Client) PublishCommand(action string, payload interface{}) error {
	c.gatewayMu.RLock()
	fn := c.gatewayPublish
	c.gatewayMu.RUnlock()
	if fn != nil {
		return fn(action, payload)
	}

	// Banner ack/silence against Cerbo MQTT when not on IGW.
	if action == "acknowledge_all_notifications" || action == "silence_alarm" ||
		action == "dismiss_banner" || action == "acknowledge_victron_banner" {
		return c.publishCerboAlarmCommand(action)
	}

	topic := fmt.Sprintf("inverter/cmd/%s", action)
	var message []byte
	var err error
	if payload != nil {
		message, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal command payload: %w", err)
		}
	} else {
		message = []byte("{}")
	}
	if c.client == nil || !c.client.IsConnectionOpen() {
		return fmt.Errorf("mqtt not connected")
	}
	if token := c.client.Publish(topic, 0, false, message); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish command: %w", token.Error())
	}
	log.Printf("Published command to %s", topic)
	return nil
}

// publishCerboAlarmCommand writes Venus-platform AcknowledgeAll or vebus SilenceAlarm.
func (c *Client) publishCerboAlarmCommand(action string) error {
	portal := c.PortalID()
	if portal == "" {
		return fmt.Errorf("cerbo portal id required for %s", action)
	}
	if c.client == nil || !c.client.IsConnectionOpen() {
		return fmt.Errorf("mqtt not connected")
	}
	var topic, body string
	switch action {
	case "silence_alarm":
		topic = fmt.Sprintf("W/%s/vebus/0/Alarm", portal)
		body = `{"SilenceAlarm":"1"}`
	default:
		topic = fmt.Sprintf("W/%s/platform/0/Notifications/AcknowledgeAll", portal)
		body = `{"value":1}`
	}
	if token := c.client.Publish(topic, 0, false, body); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish %s: %w", action, token.Error())
	}
	log.Printf("Published Cerbo command %s to %s", action, topic)
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
	c.stopKeepalive()

	// Stop command buffer worker
	if c.cmdBuffer != nil {
		c.cmdBuffer.Stop()
	}

	// Disconnect must also cancel Paho's pending reconnect loop.
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
	c.mergeDaemonState(data)
	st := c.state.Clone()
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
