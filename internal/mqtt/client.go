package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MessageHandler is a function type for handling state updates
type MessageHandler func()

// Client wraps the MQTT client and provides thread-safe state management
type Client struct {
	client mqtt.Client
	broker string
	port int
	state *state.State
	handler MessageHandler
	handlerMu sync.RWMutex
	stateMu sync.RWMutex
	consoleLines []string
	consoleMu sync.RWMutex
	maxConsoleLines int
}

// NewClient creates a new MQTT client instance with Python-equivalent defaults
func NewClient(broker string, port int) *Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, port))
	opts.SetClientID("inverter-dashboard")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5)

	return &Client{
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
	}
}

func (c *Client) GetIP() string { return c.broker }
func (c *Client) GetPort() int { return c.port }
func (c *Client) GetState() *state.State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
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
	return nil
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
	log.Printf("Subscribed to MQTT topics")
	return nil
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

func (c *Client) Disconnect() {
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

	c.stateMu.Lock()
	st := c.state

	// Set version info
	st.DashboardVersion = version.GetCurrent()
	if version, ok := data["version"].(string); ok {
		st.Version = version
	}

	// Parse all fields from raw map
	for k, v := range data {
		switch k {
		// Core metrics
		case "solar_total":
			if val, ok := v.(float64); ok {
				st.SolarTotal = val
			}
		case "gt":
			if val, ok := v.(float64); ok {
				st.GT = val
			}
		case "battery_soc":
			if val, ok := v.(float64); ok {
				st.BatterySOC = val
			}
		case "tt":
			if val, ok := v.(float64); ok {
				st.TT = val
			}
		case "mppt_total":
			if val, ok := v.(float64); ok {
				st.MpptTotal = val
			}
		case "pv_total":
			if val, ok := v.(float64); ok {
				st.PVTotal = val
			}
		case "g1":
			if val, ok := v.(float64); ok {
				st.G1 = val
			}
		case "g2":
			if val, ok := v.(float64); ok {
				st.G2 = val
			}
		case "t1":
			if val, ok := v.(float64); ok {
				st.T1 = val
			}
		case "t2":
			if val, ok := v.(float64); ok {
				st.T2 = val
			}
		case "bc":
			if val, ok := v.(float64); ok {
				st.BC = val
			}
		case "bv":
			if val, ok := v.(float64); ok {
				st.BV = val
			}
		case "bp":
			if val, ok := v.(float64); ok {
				st.BP = val
			}
		case "setpoint":
			if val, ok := v.(float64); ok {
				st.Setpoint = val
			}
		case "battery_voltage":
			if val, ok := v.(float64); ok {
				st.BatteryVoltage = val
			}
		case "battery_current":
			if val, ok := v.(float64); ok {
				st.BatteryCurrent = val
			}
		case "battery_power":
			if val, ok := v.(float64); ok {
				st.BatteryPower = val
			}
		case "inverter_state":
			if val, ok := v.(string); ok {
				st.InverterState = val
			}
		case "uptime":
			if val, ok := v.(float64); ok {
				st.Uptime = val
			}
		case "ha_connected":
			if val, ok := v.(bool); ok {
				st.HAConnected = val
			}
		case "ha_direct_connected":
			if val, ok := v.(bool); ok {
				st.HADirectConnected = val
			}
		case "ess_mode":
			if essMap, ok := v.(map[string]interface{}); ok {
				c.parseESSMode(st, essMap)
			}
		case "booleans":
			if boolMap, ok := v.(map[string]interface{}); ok {
				st.Booleans = boolMap
			}
		case "features":
			if featureMap, ok := v.(map[string]interface{}); ok {
				st.Features = featureMap
			}
		case "daily_stats":
			if dsMap, ok := v.(map[string]interface{}); ok {
				c.parseDailyStats(st, dsMap)
			}
		case "batteries":
			if batteryArray, ok := v.([]interface{}); ok {
				st.Batteries = c.parseBatteryArray(batteryArray)
			}
		case "solar_sources":
			if solarArray, ok := v.([]interface{}); ok {
				st.SolarSources = c.parseSolarArray(solarArray)
			}
		case "mppt_chargers":
			if chargerArray, ok := v.([]interface{}); ok {
				st.MPPTChargers = c.parseChargerArray(chargerArray)
			}
		case "mppt_individual":
			if floatArray, ok := v.([]interface{}); ok {
				st.MPPTIndividual = c.parseFloatArray(floatArray)
			}
		case "tasmota_individual":
			if floatArray, ok := v.([]interface{}); ok {
				st.TasmotaIndividual = c.parseFloatArray(floatArray)
			}
		case "loads":
			if loadsMap, ok := v.(map[string]interface{}); ok {
				st.Loads = c.parseLoads(loadsMap)
			}
		case "water_valve":
			if val, ok := v.(bool); ok {
				st.WaterValve = val
			}
		case "pump_switch":
			if val, ok := v.(bool); ok {
				st.PumpSwitch = val
			}
		case "dishwasher_running":
			if val, ok := v.(bool); ok {
				st.DishwasherRunning = val
			}
		case "dishwasher_duration":
			if val, ok := v.(float64); ok {
				st.DishwasherDuration = val
			}
		case "dishwasher_time":
			if val, ok := v.(float64); ok {
				st.DishwasherTime = val
			}
		case "dishwasher_active":
			if val, ok := v.(bool); ok {
				st.DishwasherActive = val
			}
		case "washer_time":
			if val, ok := v.(float64); ok {
				st.WasherTime = val
			}
		case "washer_power":
			if val, ok := v.(float64); ok {
				st.WasherPower = val
			}
		case "dryer_time":
			if val, ok := v.(float64); ok {
				st.DryerTime = val
			}
		case "dryer_power":
			if val, ok := v.(float64); ok {
				st.DryerPower = val
			}
		case "only_charging":
			if val, ok := v.(bool); ok {
				st.OnlyCharging = val
			}
		case "no_feed":
			if val, ok := v.(bool); ok {
				st.NoFeed = val
			}
		case "house_support":
			if val, ok := v.(bool); ok {
				st.HouseSupport = val
			}
		case "charge_battery":
			if val, ok := v.(bool); ok {
				st.ChargeBattery = val
			}
		case "do_not_supply_ev":
			if val, ok := v.(bool); ok {
				st.DoNotSupplyEV = val
			}
		case "limit_to_ev":
			if val, ok := v.(bool); ok {
				st.LimitToEV = val
			}
		case "minimize_charging":
			if val, ok := v.(bool); ok {
				st.MinimizeCharging = val
			}
		case "dry_run":
			if val, ok := v.(bool); ok {
				st.DryRun = val
			}
		default:
			// Store unknown fields in Features
			st.Features[k] = v
		}
	}
	c.stateMu.Unlock()

	// Log values
	log.Printf("State update - solar: %.2fW, grid: %.2fW, battery: %.2f%%, cons: %.2fW",
		st.SolarTotal, st.GT, st.BatterySOC, st.TT)

	// Trigger handler asynchronously (matches Python's asyncio pattern)
	c.triggerHandler()
}

func (c *Client) parseESSMode(st *state.State, essMap map[string]interface{}) {
	ess := &st.ESSMode
	if val, ok := essMap["battery_life_state"].(float64); ok {
		ess.BatteryLifeState = int(val)
	}
	if val, ok := essMap["hub4_mode"].(float64); ok {
		ess.Hub4Mode = int(val)
	}
	if val, ok := essMap["is_external"].(bool); ok {
		ess.IsExternal = val
	}
	if val, ok := essMap["mode_name"].(string); ok {
		ess.ModeName = val
	}
}

func (c *Client) parseDailyStats(st *state.State, dsMap map[string]interface{}) {
	ds := &st.DailyStats
	if val, ok := dsMap["solar_kwh"].(float64); ok {
		ds.SolarKWh = val
		ds.ProducedToday = val
	}
	if val, ok := dsMap["solar_money"].(float64); ok {
		ds.SolarMoney = val
		ds.ProducedDollars = val
	}
	if val, ok := dsMap["grid_kwh"].(float64); ok {
		ds.GridKWh = val
	}
	if val, ok := dsMap["grid_money"].(float64); ok {
		ds.GridMoney = val
	}
	if val, ok := dsMap["batt_in_kwh"].(float64); ok {
		ds.BattInKWh = val
		ds.BatteryIn = val
	}
	if val, ok := dsMap["batt_out_kwh"].(float64); ok {
		ds.BattOutKWh = val
		ds.BatteryOut = val
	}
	if val, ok := dsMap["batt_net_kwh"].(float64); ok {
		ds.BattNetKWh = val
	}
	// Parse reference naming
	if val, ok := dsMap["produced_today"].(float64); ok {
		ds.ProducedToday = val
	}
	if val, ok := dsMap["produced_dollars"].(float64); ok {
		ds.ProducedDollars = val
	}
	if val, ok := dsMap["battery_in"].(float64); ok {
		ds.BatteryIn = val
	}
	if val, ok := dsMap["battery_out"].(float64); ok {
		ds.BatteryOut = val
	}
	if val, ok := dsMap["battery_in_yesterday"].(float64); ok {
		ds.BatteryInYesterday = val
	}
	if val, ok := dsMap["battery_out_yesterday"].(float64); ok {
		ds.BatteryOutYesterday = val
	}
	if val, ok := dsMap["tasmota_daily"].([]interface{}); ok {
		ds.TasmotaDaily = parseFloatSlice(val)
	}
	if val, ok := dsMap["mppt_daily"].([]interface{}); ok {
		ds.MpptDaily = parseFloatSlice(val)
	}
	if val, ok := dsMap["pv_total_daily"].(float64); ok {
		ds.PVTotalDaily = val
	}
}

func (c *Client) parseBatteryArray(arr []interface{}) []state.Battery {
	result := make([]state.Battery, 0, len(arr))
	for _, item := range arr {
		if batMap, ok := item.(map[string]interface{}); ok {
			bat := state.Battery{}
			if val, ok := batMap["name"].(string); ok {
				bat.Name = val
			}
			if val, ok := batMap["voltage"].(float64); ok {
				bat.Voltage = val
			}
			if val, ok := batMap["current"].(float64); ok {
				bat.Current = val
			}
			if val, ok := batMap["power"].(float64); ok {
				bat.Power = val
			}
			if val, ok := batMap["soc"].(float64); ok {
				bat.SOC = val
			}
			if val, ok := batMap["state"].(string); ok {
				bat.State = val
			}
			if val, ok := batMap["time_to_go"].(string); ok {
				bat.TimeToGo = val
			}
			result = append(result, bat)
		}
	}
	return result
}

func (c *Client) parseSolarArray(arr []interface{}) []state.SolarSource {
	result := make([]state.SolarSource, 0, len(arr))
	for _, item := range arr {
		if srcMap, ok := item.(map[string]interface{}); ok {
			src := state.SolarSource{}
			if val, ok := srcMap["name"].(string); ok {
				src.Name = val
			}
			if val, ok := srcMap["pv_voltage"].(float64); ok {
				src.PVVoltage = val
			}
			if val, ok := srcMap["current"].(float64); ok {
				src.Current = val
			}
			if val, ok := srcMap["power"].(float64); ok {
				src.Power = val
			}
			result = append(result, src)
		}
	}
	return result
}

func (c *Client) parseChargerArray(arr []interface{}) []state.Charger {
	result := make([]state.Charger, 0, len(arr))
	for _, item := range arr {
		if chgMap, ok := item.(map[string]interface{}); ok {
			chg := state.Charger{}
			if val, ok := chgMap["name"].(string); ok {
				chg.Name = val
			}
			if val, ok := chgMap["pv_voltage"].(float64); ok {
				chg.PVVoltage = val
			}
			if val, ok := chgMap["current"].(float64); ok {
				chg.Current = val
			}
			if val, ok := chgMap["power"].(float64); ok {
				chg.Power = val
			}
			result = append(result, chg)
		}
	}
	return result
}

func parseFloatSlice(arr []interface{}) []float64 {
	result := make([]float64, 0, len(arr))
	for _, item := range arr {
		if val, ok := item.(float64); ok {
			result = append(result, val)
		}
	}
	return result
}

func (c *Client) parseFloatArray(arr []interface{}) []float64 {
	return parseFloatSlice(arr)
}

func (c *Client) parseLoads(loadMap map[string]interface{}) map[string]float64 {
	result := make(map[string]float64)
	for k, v := range loadMap {
		if val, ok := v.(float64); ok {
			result[k] = val
		}
	}
	return result
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
