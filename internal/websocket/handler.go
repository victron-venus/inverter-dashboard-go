package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/victron-venus/inverter-dashboard-go/internal/homeassistant"
	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
)

// Message represents a WebSocket message from client
type Message struct {
	Action   string      `json:"action"`
	Entity   string      `json:"entity,omitempty"`
	Value    interface{} `json:"value,omitempty"`
	Min      float64     `json:"min,omitempty"`
	Max      float64     `json:"max,omitempty"`
	Interval float64     `json:"interval,omitempty"`
}

// State represents the complete state sent to clients
type State struct {
	MQTTState        map[string]interface{} `json:"-"`
	Console          []string               `json:"console"`
	DashboardVersion string                 `json:"dashboard_version"`
	LatestVersion    string                 `json:"latest_version,omitempty"`
	UIConfig         map[string]interface{} `json:"ui_config,omitempty"`
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for simplicity
		},
	}

	// Connected clients
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.RWMutex

	latestVersion string
	latestMu      sync.RWMutex
)

// HandleWebSocket handles WebSocket connections
func HandleWebSocket(c *gin.Context, mqttClient *mqtt.Client, haClient *homeassistant.Client) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Register client
	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	log.Printf("WebSocket client connected (%d total)", len(clients))

	// Send initial state
	if err := sendInitialState(conn, mqttClient, haClient); err != nil {
		log.Printf("Failed to send initial state: %v", err)
		removeClient(conn)
		return
	}

	// Handle messages
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle the action
		if err := handleMessage(msg, mqttClient, haClient); err != nil {
			log.Printf("Failed to handle message: %v", err)
		}

		var broadcastOverlay homeassistant.Overlay

		if haClient != nil {
			broadcastOverlay = haClient.GetOverlay()
		}
		// Broadcast updated state to all clients
		BroadcastState(mqttClient, haClient, broadcastOverlay)
	}

	removeClient(conn)
}

// sendInitialState sends the complete state to a newly connected client
func sendInitialState(conn *websocket.Conn, mqttClient *mqtt.Client, haClient *homeassistant.Client) error {
	state := mqttClient.GetState()
	console := mqttClient.GetConsole()

	// Get overlay and UI config if HA client is configured
	overlay := homeassistant.Overlay{}
	uiConfig := map[string]interface{}{}
	if haClient != nil {
		overlay = haClient.GetOverlay()
		uiConfig = haClient.GetUIConfig()
		// Add boolean buttons for dynamic header toggles
		uiConfig["boolean_buttons"] = haClient.GetBooleanButtons()
	}

	// Merge MQTT state with HA overlay
	mergedState := mergeStates(state, overlay)

	payload := map[string]interface{}{
		"state": mergedState,
		"console": console,
		"dashboard_version": version.GetCurrent(),
		"latest_version": getLatestVersion(),
		"ui_config": uiConfig,
	}

	// Limit console to last 20 lines
	if len(console) > 20 {
		payload["console"] = console[len(console)-20:]
	}

	return conn.WriteJSON(payload)
}

// handleMessage processes incoming WebSocket messages
func handleMessage(msg Message, mqttClient *mqtt.Client, haClient *homeassistant.Client) error {
	switch msg.Action {
	case "toggle":
		return handleToggle(msg.Entity, mqttClient, haClient)
	case "press":
		return handlePress(msg.Entity, mqttClient, haClient)
	case "setpoint":
		return mqttClient.PublishCommand("setpoint", map[string]interface{}{"value": msg.Value})
	case "dry_run":
		return mqttClient.PublishCommand("dry_run", map[string]interface{}{})
	case "limits":
		return mqttClient.PublishCommand("limits", map[string]interface{}{
			"min": msg.Min,
			"max": msg.Max,
		})
	case "ess_mode":
		return mqttClient.PublishCommand("ess_mode", map[string]interface{}{})
	case "loop_interval":
		interval := msg.Interval
		if interval == 0 {
			interval = 0.33
		}
		return mqttClient.PublishCommand("loop_interval", map[string]interface{}{
			"interval": interval,
		})
	default:
		return fmt.Errorf("unknown action: %s", msg.Action)
	}
}

// handleToggle handles toggle actions
// handleToggle handles toggle actions
func handleToggle(entityID string, mqttClient *mqtt.Client, haClient *homeassistant.Client) error {
	if entityID == "" {
		return fmt.Errorf("entity ID required for toggle")
	}

	// Use HA direct mode if enabled
	if haClient != nil && haClient.IsDirectMode() && haClient.IsToggleAllowed(entityID) {
		if err := haClient.ToggleEntity(entityID); err != nil {
			return fmt.Errorf("failed to toggle entity: %w", err)
		}

		// Refresh HA state after toggle
		overlay, err := haClient.FetchStatesOnce()
		if err != nil {
			log.Printf("[WEBSOCKET] Failed to fetch updated state after toggle: %v", err)
			return fmt.Errorf("failed to fetch state: %w", err)
		}
		if overlay.HADirectConnected {
			haClient.ReplaceOverlay(overlay)
			// Broadcast updated state to all clients after toggle
			BroadcastState(mqttClient, haClient, overlay)
		}
		return nil
	}

	// Broadcast updated state to all clients after toggle
	var broadcastOverlay homeassistant.Overlay
	if haClient != nil {
		broadcastOverlay = haClient.GetOverlay()
	}
	BroadcastState(mqttClient, haClient, broadcastOverlay)

	// Fall back to MQTT
	return mqttClient.PublishCommand("toggle", map[string]interface{}{
		"entity": entityID,
	})
}

// handlePress handles button press actions
// handlePress handles button press actions
func handlePress(entityID string, mqttClient *mqtt.Client, haClient *homeassistant.Client) error {
	if entityID == "" {
		return fmt.Errorf("entity ID required for press")
	}

	// Try HA button press first
	if haClient != nil && haClient.IsDirectMode() && haClient.IsToggleAllowed(entityID) {
		if strings.HasPrefix(entityID, "button.") {
			err := haClient.PressButton(entityID)
			if err == nil {
				return nil
			}
		}
	}

	// Fall back to MQTT
	return mqttClient.PublishCommand("press", map[string]interface{}{
		"entity": entityID,
	})
}

// removeClient removes a client from the active list
func removeClient(conn *websocket.Conn) {
	clientsMu.Lock()
	delete(clients, conn)
	clientsMu.Unlock()

	conn.Close()
	log.Printf("WebSocket client disconnected (%d remaining)", len(clients))
}

// SetLatestVersion sets the latest version (called from version checker)
func SetLatestVersion(version string) {
	latestMu.Lock()
	latestVersion = version
	latestMu.Unlock()
	log.Printf("Set latest version to: %s", version)
}

// getLatestVersion returns the cached latest version
func getLatestVersion() string {
	latestMu.RLock()
	defer latestMu.RUnlock()
	return latestVersion
}

// getKeys returns slice of map keys for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// BroadcastState sends the current state to all connected clients
func BroadcastState(mqttClient *mqtt.Client, haClient *homeassistant.Client, overlay homeassistant.Overlay) error {
	state := mqttClient.GetState()
	console := mqttClient.GetConsole()

	// Debug: log all important fields
	log.Printf("[BROADCAST DEBUG] Base MQTT state: solar=%.2fW, grid=%.2fW, battery_SOC=%.2f%%, cons=%.2fW", state.SolarTotal, state.GT, state.BatterySOC, state.TT)

	uiConfig := map[string]interface{}{}
	haDirectConnected := false

	if haClient != nil {
		uiConfig = haClient.GetUIConfig()
		uiConfig["boolean_buttons"] = haClient.GetBooleanButtons()
		haDirectConnected = overlay.HADirectConnected
	}


	// Debug: show overlay values
	if haDirectConnected {
		log.Printf("[BROADCAST DEBUG] HA Direct Connected: true")
		log.Printf("[BROADCAST DEBUG] Overlay AdditionalFields: %+v", overlay.AdditionalFields)
	} else {
		log.Printf("[BROADCAST DEBUG] HA Direct Connected: false - HA values not available")
	}

	// Merge MQTT state with HA overlay
	mergedState := mergeStates(state, overlay)
	log.Printf("[BROADCAST DEBUG] Merged state keys: %+v", getKeys(mergedState))
	log.Printf("[BROADCAST DEBUG] Merged state water_level: %v", mergedState["water_level"])
	log.Printf("[BROADCAST DEBUG] Merged state car_soc: %v", mergedState["car_soc"])
	log.Printf("[BROADCAST DEBUG] Merged state ev_charging_kw: %v", mergedState["ev_charging_kw"])

	// Prepare payload
	payload := map[string]interface{}{
		"state": mergedState,
		"console": console,
		"dashboard_version": version.GetCurrent(),
		"latest_version": getLatestVersion(),
		"ui_config": uiConfig,
		"ha_direct_connected": haDirectConnected,
	}

	// Limit console to last 20 lines
	if len(console) > 20 {
		payload["console"] = console[len(console)-20:]
	}

	// Add overlay booleans if HA is configured
	if overlay.Booleans != nil {
		payload["booleans"] = overlay.Booleans
	}

	// Add other overlay fields
	log.Printf("[BROADCAST DEBUG] Adding AdditionalFields to payload:")
	for k, v := range overlay.AdditionalFields {
		log.Printf("[BROADCAST DEBUG]   - %s: %v (type: %T)", k, v, v)
		payload[k] = v
	}

	// Log full payload keys
	log.Printf("[BROADCAST DEBUG] Full payload keys: %+v", getKeys(payload))

	// Broadcast to all clients with per-client exception handling (matches Python)
	// Use write lock to prevent concurrent websocket writes
	clientsMu.Lock()
	defer clientsMu.Unlock()

	message, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Debug: log message content
	var payloadDebug map[string]interface{}
	if err := json.Unmarshal(message, &payloadDebug); err == nil {
		log.Printf("[BROADCAST DEBUG] Payload being sent: keys=%+v", getKeys(payloadDebug))
		if waterLevel, ok := payloadDebug["water_level"]; ok {
			log.Printf("[BROADCAST DEBUG] Payload contains water_level: %v", waterLevel)
		} else {
			log.Printf("[BROADCAST DEBUG] Payload does NOT contain water_level")
		}
		if carSOC, ok := payloadDebug["car_soc"]; ok {
			log.Printf("[BROADCAST DEBUG] Payload contains car_soc: %v", carSOC)
		} else {
			log.Printf("[BROADCAST DEBUG] Payload does NOT contain car_soc")
		}
	}

	log.Printf("Broadcasting to %d clients", len(clients))

	// Send to each client individually, catching errors per client (matches Python try/except)
	for conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("Failed to send to client: %v", err)
			delete(clients, conn)
			conn.Close()
		}
	}

	return nil
}

// mergeStates merges MQTT state with HA overlay
func mergeStates(mqttState *state.State, overlay homeassistant.Overlay) map[string]interface{} {
	// Start with MQTT state as map
	merged := stateToMap(mqttState)

	// Set HA connection status
	merged["ha_direct_connected"] = overlay.HADirectConnected

	// If HA is connected, override with HA values
	if overlay.HADirectConnected {
		// Override booleans (create deep copy to prevent concurrent map access)
		if len(overlay.Booleans) > 0 {
			booleansCopy := make(map[string]bool, len(overlay.Booleans))
			for k, v := range overlay.Booleans {
				booleansCopy[k] = v
			}
			merged["booleans"] = booleansCopy
		}

		// Add other fields from overlay (create deep copy to prevent concurrent map access)
		if len(overlay.AdditionalFields) > 0 {
			additionalCopy := make(map[string]interface{}, len(overlay.AdditionalFields))
			for k, v := range overlay.AdditionalFields {
				// Only override if value is not nil
				if v != nil {
					additionalCopy[k] = v
				}
			}
			for k, v := range additionalCopy {
				merged[k] = v
			}
		}
	} else {
		// Reset HA-specific fields when not connected
		if booleans, ok := merged["booleans"].(map[string]interface{}); ok {
			for key := range booleans {
				booleans[key] = false
			}
		}
	}

	return merged
}

// stateToMap converts State struct to map[string]interface{}
func stateToMap(s *state.State) map[string]interface{} {
	return map[string]interface{}{
		"booleans": s.Booleans,
		"features": safeFeatures(s.Features),
		"daily_stats": s.DailyStats,
		"ess_mode": s.ESSMode,
		"solar_total": s.SolarTotal,
		"mppt_total": s.MpptTotal,
		"pv_total": s.PVTotal,
		"solar_sources": s.SolarSources,
		"mppt_chargers": s.MPPTChargers,
		"mppt_individual": s.MPPTIndividual,
		"tasmota_individual": s.TasmotaIndividual,
		"batteries": s.Batteries,
		"gt": s.GT,
		"g1": s.G1,
		"g2": s.G2,
		"tt": s.TT,
		"t1": s.T1,
		"t2": s.T2,
		"bc": s.BC,
		"bv": s.BV,
		"bp": s.BP,
		"setpoint": s.Setpoint,
		"battery_voltage": s.BatteryVoltage,
		"battery_current": s.BatteryCurrent,
		"battery_power": s.BatteryPower,
		"battery_soc": s.BatterySOC,
		"inverter_state": s.InverterState,
		"uptime": s.Uptime,
		"ha_connected": s.HAConnected,
		"ha_direct_connected": s.HADirectConnected,
		"version": s.Version,
		"dashboard_version": s.DashboardVersion,
		"console": s.Console,
	}
}

// safeFeatures ensures we never return nil for features
func safeFeatures(features map[string]interface{}) map[string]interface{} {
	if features == nil {
		return make(map[string]interface{})
	}
	return features
}

// GetConnectedCount returns the number of connected WebSocket clients
func GetConnectedCount() int {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	return len(clients)
}

// CloseAll closes all WebSocket connections (for shutdown)
func CloseAll() {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for conn := range clients {
		conn.Close()
		delete(clients, conn)
	}
}
