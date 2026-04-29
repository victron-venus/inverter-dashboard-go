package homeassistant

import (
	"context"
	"encoding/json"
	"strconv"
	"bytes"
	"fmt"
	"log"
	"net/http"
"sort"
	"strings"
	"net/url"
	"sync"

	"time"
	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

// EntityState represents a single entity's state from HA

// convertConfigSwitchEntities converts []config.EntityConfig to []Button
func convertConfigSwitchEntities(entities []config.EntityConfig) []Button {
	result := make([]Button, 0, len(entities))
	for _, entity := range entities {
		btn := Button{
			ID:       strings.ReplaceAll(entity.Key, "_", "-"),
			Label:    entity.Label,
			Entity:   entity.Entity,
			StateKey: entity.Key,
			Order:    entity.Order,
		}
		if btn.Label == "" {
			btn.Label = generateDefaultLabel(entity.Key)
		}
		result = append(result, btn)
	}
	// Sort by order to maintain consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Order < result[j].Order
	})
	return result
}

// convertConfigBooleanEntities converts []config.BooleanEntityConfig to (map[string]string, []Button)
func convertConfigBooleanEntities(entities []config.BooleanEntityConfig) (map[string]string, []Button) {
	entityMap := make(map[string]string)
	var buttons []Button

	for _, entity := range entities {
		entityMap[entity.Key] = entity.Entity

		btn := Button{
			ID:       strings.ReplaceAll(entity.Key, "_", "-"),
			Label:    generateDefaultLabel(entity.Key),
			Entity:   entity.Entity,
			StateKey: entity.Key,
			Order:    entity.Order,
		}
		buttons = append(buttons, btn)
	}

	// Sort buttons by order
	sort.Slice(buttons, func(i, j int) bool {
		return buttons[i].Order < buttons[j].Order
	})

	return entityMap, buttons
}
type EntityState struct {
	EntityID string `json:"entity_id"`
	State string `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// Overlay represents the HA state merged with MQTT state
type Overlay struct {
	Booleans map[string]bool `json:"booleans"`
	HADirectConnected bool `json:"ha_direct_connected"`
	AdditionalFields map[string]interface{} `json:"-"`
}

// Button defines a UI button for home controls
type Button struct {
	ID string `json:"id"`
	Label string `json:"label"`
	Entity string `json:"entity"`
	StateKey string `json:"state_key"`
Order    int    `json:"order"`
}

// Client handles Home Assistant REST API interactions
type Client struct {
	httpURL string
	token string
	directMode bool
	pollInterval time.Duration
	booleanEntities map[string]string
	booleanButtons []Button
	switchEntities []Button
	waterValve string
	waterPump string
	waterLevel string
	carSOC string
	evChargingKW string
	evPower string
	applianceEntities map[string]string
	vueSensors map[string]string

	// Runtime state
	overlay Overlay
	overlayMu sync.RWMutex
	configured bool

	httpClient *http.Client
}

// NewClient creates a new Home Assistant client from configuration
func NewClient(cfg *config.HomeAssistantConfig) *Client {
	if cfg == nil {
		return &Client{
			configured: false,
			overlayMu:  sync.RWMutex{},
			overlay: Overlay{
				Booleans:         make(map[string]bool),
				HADirectConnected: true,
			},
		}
	}

	// Convert switch entities from config
	switchEntities := convertConfigSwitchEntities(cfg.SwitchEntities)

	// Convert boolean entities from config
	booleanEntityMap, booleanButtons := convertConfigBooleanEntities(cfg.BooleanEntities)

	client := &Client{
		httpURL:         cfg.URL,
		token:           cfg.Token,
		directMode:      cfg.DirectControls,
		pollInterval:    time.Duration(cfg.PollInterval * float64(time.Second)),
		booleanEntities: booleanEntityMap,
		booleanButtons:  booleanButtons,
		applianceEntities: cfg.ApplianceEntities,
		vueSensors:       cfg.VueSensors,
		switchEntities:   switchEntities,
		configured:       false,
		overlayMu:        sync.RWMutex{},
		overlay: Overlay{
			Booleans:         make(map[string]bool),
			HADirectConnected: true,
		},
	}
	client.waterLevel = cfg.WaterLevelEntity
	client.waterPump = cfg.PumpSwitchEntity
	client.carSOC = cfg.CarSOCEntity
	client.evChargingKW = cfg.EVChargingKWEntity
	client.evPower = cfg.EVPowerEntity

	// Initialize HTTP client with timeout
	client.httpClient = &http.Client{
		Timeout: 20 * time.Second,
	}

	// Validate and set configured flag
	client.configured = client.validateConfig()

	return client
}

func (c *Client) validateConfig() bool {
	if c.httpURL == "" || c.token == "" {
		return false
	}

	if c.token == "REPLACE_WITH_LONG_LIVED_ACCESS_TOKEN" {
		return false
	}

	_, err := url.Parse(c.httpURL)
	return err == nil
}

func (c *Client) IsDirectMode() bool {
	log.Printf("[HA CLIENT DEBUG] IsDirectMode() called: configured=%v, directMode=%v, returning=%v (configured decides)",
		c.configured, c.directMode, c.configured)
	// If HA is configured, always use direct mode, ignoring the config flag
	return c.configured
}

func (c *Client) GetUIConfig() map[string]interface{} {
	if !c.configured || len(c.switchEntities) == 0 {
		return map[string]interface{}{}
	}

	buttons := make([]Button, 0, len(c.switchEntities))
	for _, btn := range c.switchEntities {
		buttons = append(buttons, btn)
	}

	// Sort buttons by order to prevent shuffle
	sort.Slice(buttons, func(i, j int) bool {
		return buttons[i].Order < buttons[j].Order
	})

	return map[string]interface{}{
		"home_buttons": buttons,
	}
}

func (c *Client) IsToggleAllowed(entityID string) bool {
	if !c.configured || entityID == "" {
		return false
	}

	for _, eid := range c.booleanEntities {
		if eid == entityID {
			return true
		}
	}

	for _, button := range c.switchEntities {
		if button.Entity == entityID {
			return true
		}
	}

	return entityID == c.waterValve || entityID == c.waterPump
}

// parseSwitchEntities handles Python's three entity formats:
// - String: "entity_id"
// - Tuple/List: ["entity_id", "Label"]
// - Dict: {"entity": "entity_id", "label": "Label"}
// parseSwitchEntities handles Python's three entity formats:
// - String: "entity_id"
// - Tuple/List: ["entity_id", "Label"]
// - Dict: {"entity": "entity_id", "label": "label"}
// Plus direct YAML format: []string{"entity_id", "Label"}
// parseSwitchEntities handles Python's three entity formats:
// - String: "entity_id"
// - Tuple/List: ["entity_id", "Label"]
// - Dict: {"entity": "entity_id", "label": "label"}
// Plus direct YAML format: []string{"entity_id", "Label"}
func parseSwitchEntities(entities map[string]interface{}) map[string]Button {
	result := make(map[string]Button)

	for key, value := range entities {
		btn := Button{
			ID: strings.ReplaceAll(key, "_", "-"),
			StateKey: key,
		}

		// Handle different value types based on HA_SWITCH_ENTITIES format
		switch v := value.(type) {
		case string:
			// String format: "entity_id"
			btn.Entity = v
			btn.Label = generateDefaultLabel(key)

		case []interface{}:
			// Tuple/List format: ["entity_id", "Label"]
			if len(v) > 0 {
				if entityID, ok := v[0].(string); ok {
					btn.Entity = entityID
				}
				if len(v) > 1 {
					if label, ok := v[1].(string); ok && label != "" {
						btn.Label = label
					} else {
						btn.Label = generateDefaultLabel(key)
					}
				}
			}

		case []string:
			// YAML string array ["entity_id", "Label"]
			if len(v) > 0 {
				btn.Entity = v[0]
				if len(v) > 1 && v[1] != "" {
					btn.Label = v[1]
				} else {
					btn.Label = generateDefaultLabel(key)
				}
			}

		case map[string]interface{}:
			// Dict format: {"entity": "entity_id", "label": "label"}
			if entityID, ok := v["entity"].(string); ok {
				btn.Entity = entityID
			}
			if label, ok := v["label"].(string); ok && label != "" {
				btn.Label = label
			} else if label, ok := v["short"].(string); ok && label != "" {
				btn.Label = label
			} else if label, ok := v["name"].(string); ok && label != "" {
				btn.Label = label
			} else {
				btn.Label = generateDefaultLabel(key)
			}
if order, ok := v["order"].(int); ok {
		btn.Order = order
	} else if order, ok := v["order"].(float64); ok {
		btn.Order = int(order)
	}

		default:
			continue // Skip invalid type
		}

		result[key] = btn
	}
	return result
}

// generateDefaultLabel creates a default label from state key (matches Python's _default_switch_label)
func generateDefaultLabel(stateKey string) string {
	s := stateKey
	if strings.HasPrefix(s, "home_") {
		s = s[5:]
	}
	return strings.ReplaceAll(strings.ToUpper(s), "_", " ")
}

func (c *Client) GetOverlay() Overlay {
	c.overlayMu.RLock()
	defer c.overlayMu.RUnlock()
	return c.overlay
}

func (c *Client) ReplaceOverlay(data Overlay) {
	c.overlayMu.Lock()
	defer c.overlayMu.Unlock()
	c.overlay = data
	log.Printf("[HA CLIENT DEBUG] Overlay replaced: %+v", data.AdditionalFields)
}

func (c *Client) FetchStatesOnce() (Overlay, error) {
	log.Printf("[HA CLIENT DEBUG] FetchStatesOnce() entry: IsDirectMode()=%v", c.IsDirectMode())
	if !c.IsDirectMode() {
		log.Printf("[HA CLIENT DEBUG] FetchStatesOnce: IsDirectMode=false, returning empty")
		return Overlay{Booleans: map[string]bool{}, HADirectConnected: false}, nil
	}

	result := Overlay{
		Booleans:          make(map[string]bool),
		HADirectConnected: true, // Always true when HA configured
		AdditionalFields:  make(map[string]interface{}),
	}

	// Set up request context and headers
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Helper to fetch a single entity state - now uses the new method below
	fetchEntityState := func(entityID string) (string, error) {
		return c.getEntityState(ctx, entityID)
	}

	// Fetch boolean entities
	for key, entityID := range c.booleanEntities {
		state, err := fetchEntityState(entityID)
		if err != nil {
			continue
		}
		result.Booleans[key] = isOn(state)
	}

	// Fetch switch entities
	for _, btn := range c.switchEntities {
		state, err := fetchEntityState(btn.Entity)
		if err != nil {
			continue
		}
		result.AdditionalFields[btn.StateKey] = isOn(state)
	}

	// Fetch water valve and pump
	if c.waterValve != "" {
		state, err := fetchEntityState(c.waterValve)
		if err == nil {
			result.AdditionalFields["water_valve"] = isOn(state)
		}
	}
	if c.waterPump != "" {
		state, err := fetchEntityState(c.waterPump)
		if err == nil {
			result.AdditionalFields["pump_switch"] = isOn(state)
		}
	}

	// Fetch water level
	log.Printf("[HA DEBUG] Fetching water_level from entity: %s", c.waterLevel)
	if c.waterLevel != "" {
		state, err := fetchEntityState(c.waterLevel)
		if err == nil {
			if val, err := strconv.ParseFloat(state, 64); err == nil {
				result.AdditionalFields["water_level"] = val
				log.Printf("[HA DEBUG] water_level fetched successfully: %v cm", val)
			}
		} else {
			log.Printf("[HA DEBUG] Failed to fetch water_level: %v", err)
		}
	} else {
		log.Printf("[HA DEBUG] waterLevel entity not configured")
	}

	// Fetch car SOC
	log.Printf("[HA DEBUG] Fetching car_soc from entity: %s", c.carSOC)
	if c.carSOC != "" {
		state, err := fetchEntityState(c.carSOC)
		if err == nil {
			if val, err := strconv.ParseFloat(state, 64); err == nil {
				result.AdditionalFields["car_soc"] = val
				log.Printf("[HA DEBUG] car_soc fetched successfully: %v%%", val)
			}
		} else {
			log.Printf("[HA DEBUG] Failed to fetch car_soc: %v", err)
		}
	} else {
		log.Printf("[HA DEBUG] carSOC entity not configured")
	}

	// Fetch EV charging power
	log.Printf("[HA DEBUG] Fetching ev_charging_kw from entity: %s", c.evChargingKW)
	if c.evChargingKW != "" {
		state, err := fetchEntityState(c.evChargingKW)
		if err == nil {
			if val, err := strconv.ParseFloat(state, 64); err == nil {
				result.AdditionalFields["ev_charging_kw"] = val
				log.Printf("[HA DEBUG] ev_charging_kw fetched successfully: %v kW", val)
			}
		} else {
			log.Printf("[HA DEBUG] Failed to fetch ev_charging_kw: %v", err)
		}
	} else {
		log.Printf("[HA DEBUG] evChargingKW entity not configured")
	}

	// Fetch EV power
	log.Printf("[HA DEBUG] Fetching ev_power from entity: %s", c.evPower)
	if c.evPower != "" {
		state, err := fetchEntityState(c.evPower)
		if err == nil {
			if val, err := strconv.ParseFloat(state, 64); err == nil {
				result.AdditionalFields["ev_power"] = val
				log.Printf("[HA DEBUG] ev_power fetched successfully: %v", val)
			}
		} else {
			log.Printf("[HA DEBUG] Failed to fetch ev_power: %v", err)
		}
	} else {
		log.Printf("[HA DEBUG] evPower entity not configured")
	}

	log.Printf("[HA DEBUG] Final AdditionalFields: %+v", result.AdditionalFields)
	// Fetch appliance entities
	for key, entityID := range c.applianceEntities {
		state, err := fetchEntityState(entityID)
		if err != nil {
			continue
		}
		result.AdditionalFields[key] = parseApplianceField(key, entityID, state)
	}
	// Fetch Vue sensors (wattage) - store as nested loads object
	loads := make(map[string]float64)
	for key, entityID := range c.vueSensors {
		state, err := fetchEntityState(entityID)
		if err != nil {
			continue
		}
		if val, err := strconv.ParseFloat(state, 64); err == nil {
			loads[key] = val
			log.Printf("[HA DEBUG] Vue sensor %s: %.2f W", key, val)
		}
	}
	if len(loads) > 0 {
		result.AdditionalFields["loads"] = loads
	}

	log.Printf("[HA CLIENT DEBUG] FetchStatesOnce completed, setting HADirectConnected=true")
	result.HADirectConnected = true
	log.Printf("[HA CLIENT DEBUG] Final AdditionalFields: %+v", result.AdditionalFields)
	return result, nil
}

// isOn converts a Home Assistant state string to a boolean
func isOn(state string) bool {
	if state == "" {
		return false
	}
	switch strings.ToLower(state) {
	case "on", "true", "yes", "1":
		return true
	default:
		return false
	}
}

// parseApplianceField maps HA state string to dashboard type (bool vs seconds)
func parseApplianceField(stateKey, entityID, raw string) interface{} {
	domain := strings.Split(entityID, ".")[0]

	// Boolean domains
	if domain == "binary_sensor" || domain == "switch" || domain == "light" || domain == "input_boolean" {
		return isOn(raw)
	}

	// Sensor domains
	if domain == "sensor" && strings.HasSuffix(stateKey, "_power") {
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return isOn(raw)
		}
		return val > 1.0
	}

	// Time/duration sensors (HH:MM:SS or numeric seconds)
	if domain == "sensor" && (strings.HasSuffix(stateKey, "_time") || strings.HasSuffix(stateKey, "_duration")) {
		return parseStateToSeconds(raw)
	}

	// All other cases
	return parseStateToSeconds(raw)
}

// parseStateToSeconds converts HA sensor state to seconds (supports HH:MM:SS or numeric)
func parseStateToSeconds(raw string) int {
	if raw == "" || raw == "unavailable" || raw == "unknown" || raw == "None" {
		return 0
	}

	// Try parsing as float first (seconds)
	val, err := strconv.ParseFloat(raw, 64)
	if err == nil {
		return int(val)
	}

	// Try parsing as HH:MM:SS
	parts := strings.Split(raw, ":")
	if len(parts) == 3 {
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		s, _ := strconv.Atoi(parts[2])
		return h*3600 + m*60 + s
	}
	if len(parts) == 2 {
		m, _ := strconv.Atoi(parts[0])
		s, _ := strconv.Atoi(parts[1])
		return m*60 + s
	}

	return 0
}

func (c *Client) mergeSwitchState(data Overlay) Overlay {
	merged := data
	merged.HADirectConnected = c.overlay.HADirectConnected

	if c.overlay.HADirectConnected {
		if merged.Booleans == nil {
			merged.Booleans = make(map[string]bool)
		}
		for k, v := range c.overlay.Booleans {
			merged.Booleans[k] = v
		}
	}

	updatedSwitches := make(map[string]bool)
	for _, btn := range c.switchEntities {
		if val, ok := c.overlay.AdditionalFields[btn.StateKey]; ok {
			updatedSwitches[btn.StateKey] = val.(bool)
		}
	}

	for k, v := range updatedSwitches {
		merged.AdditionalFields[k] = v
	}

	return merged
}

func getStateForDomain(domain string) bool {
	return false
}

// getEntityState fetches the current state of a single entity
func (c *Client) getEntityState(ctx context.Context, entityID string) (string, error) {
	if c.httpClient == nil {
		return "", fmt.Errorf("http client not initialized")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/states/%s", c.httpURL, url.QueryEscape(entityID)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var entity EntityState
	if err := json.NewDecoder(resp.Body).Decode(&entity); err != nil {
		return "", err
	}

	return entity.State, nil
}

func (c *Client) ToggleEntity(entityID string) error {
	if !c.configured {
		return fmt.Errorf("HA not configured")
	}

	domain := strings.Split(entityID, ".")[0]
	if domain != "input_boolean" && domain != "switch" && domain != "light" {
		return fmt.Errorf("unsupported domain for toggle: %s", domain)
	}

	// Get current state
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	currentState, err := c.getEntityState(ctx, entityID)
	if err != nil {
		return fmt.Errorf("failed to fetch current state: %w", err)
	}

	// Determine if entity is currently on
	isOn := isOn(currentState)

	// Call turn_on if off, turn_off if on
	if isOn {
		return c.TurnEntity(entityID, false)
	}
	return c.TurnEntity(entityID, true)
}

func (c *Client) TurnEntity(entityID string, turnOn bool) error {
	if !c.configured {
		return fmt.Errorf("HA not configured")
	}

	domain := strings.Split(entityID, ".")[0]
	if domain != "input_boolean" && domain != "switch" && domain != "light" {
		return fmt.Errorf("unsupported domain for turn: %s", domain)
	}

	service := "turn_on"
	if !turnOn {
		service = "turn_off"
	}

	return c.callService(domain, service, entityID)
}

func (c *Client) PressButton(entityID string) error {
	if !c.configured {
		return fmt.Errorf("HA not configured")
	}

	if !strings.HasPrefix(entityID, "button.") {
		return fmt.Errorf("not a button entity: %s", entityID)
	}

	return c.callService("button", "press", entityID)
}

// callService makes a POST request to Home Assistant service endpoint
func (c *Client) callService(domain, service, entityID string) error {
	if c.httpClient == nil {
		return fmt.Errorf("http client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	body := map[string]interface{}{
		"entity_id": entityID,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/services/%s/%s", c.httpURL, domain, service),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("service call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service call returned HTTP %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetPollInterval() time.Duration {
	return c.pollInterval
}

// Parse boolean entities similarly to switch entities for header toggles
func parseBooleanEntities(entities map[string]interface{}) (map[string]string, map[string]Button) {
	entityMap := make(map[string]string)
	buttonMap := make(map[string]Button)

	for key, value := range entities {
		btn := Button{
			ID:       strings.ReplaceAll(key, "_", "-"),
			StateKey: key,
		}

		switch v := value.(type) {
		case string:
			btn.Entity = v
			btn.Label = generateDefaultLabel(key)
			entityMap[key] = v
		case []interface{}:
			if len(v) > 0 {
				if entityID, ok := v[0].(string); ok {
					btn.Entity = entityID
					entityMap[key] = entityID
				}
				if len(v) > 1 {
					if label, ok := v[1].(string); ok && label != "" {
						btn.Label = label
					} else {
						btn.Label = generateDefaultLabel(key)
					}
				}
			}
		case map[string]interface{}:
			if entityID, ok := v["entity"].(string); ok {
				btn.Entity = entityID
				entityMap[key] = entityID
			}
			if label, ok := v["label"].(string); ok && label != "" {
				btn.Label = label
			} else if label, ok := v["short"].(string); ok && label != "" {
				btn.Label = label
			} else if label, ok := v["name"].(string); ok && label != "" {
				btn.Label = label
			} else {
				btn.Label = generateDefaultLabel(key)
			}
		default:
			continue
		}

		buttonMap[key] = btn
	}

	return entityMap, buttonMap
}

// GetBooleanButtons returns parsed boolean entity buttons for header toggles
func (c *Client) GetBooleanButtons() []Button {
	if !c.configured || len(c.booleanButtons) == 0 {
		return []Button{}
	}

	buttons := make([]Button, 0, len(c.booleanButtons))
	for _, btn := range c.booleanButtons {
		buttons = append(buttons, btn)
	}
// Sort buttons by order to prevent shuffle
	sort.Slice(buttons, func(i, j int) bool {
			return buttons[i].Order < buttons[j].Order
	})

	return buttons
}