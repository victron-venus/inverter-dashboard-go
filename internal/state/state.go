package state

import "encoding/json"

// DailyStats represents daily statistics with money calculations
type DailyStats struct {
	SolarKWh   float64 `json:"solar_kwh"`
	SolarMoney float64 `json:"solar_money"`
	GridKWh    float64 `json:"grid_kwh"`
	GridMoney  float64 `json:"grid_money"`
	BattInKWh  float64 `json:"batt_in_kwh"`
	BattOutKWh float64 `json:"batt_out_kwh"`
	BattNetKWh float64 `json:"batt_net_kwh"`
	// Extra fields from reference
	ProducedYesterday   float64   `json:"produced_yesterday"`
	PVInverterDaily     []float64 `json:"pv_inverter_daily"`
	PVInverterYesterday []float64 `json:"pv_inverter_yesterday"`
	MpptYesterday       []float64 `json:"mppt_yesterday"`
	ProducedToday       float64   `json:"produced_today"`
	ProducedDollars     float64   `json:"produced_dollars"`
	BatteryIn           float64   `json:"battery_in"`
	BatteryOut          float64   `json:"battery_out"`
	BatteryInYesterday  float64   `json:"battery_in_yesterday"`
	BatteryOutYesterday float64   `json:"battery_out_yesterday"`
	MpptDaily           []float64 `json:"mppt_daily"`
	PVTotalDaily        float64   `json:"pv_total_daily"`
}

// SolarSource represents individual solar source data
type SolarSource struct {
	Name      string  `json:"name"`
	PVVoltage float64 `json:"pv_voltage,omitempty"`
	Current   float64 `json:"current"`
	Power     float64 `json:"power"`
}

// Battery represents individual battery data
type Battery struct {
	Instance           string          `json:"instance,omitempty"`
	Serial             string          `json:"serial,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	MinCellVoltage     *float64        `json:"min_cell_voltage,omitempty"`
	MaxCellVoltage     *float64        `json:"max_cell_voltage,omitempty"`
	MinVoltageCellID   string          `json:"min_voltage_cell_id,omitempty"`
	MaxVoltageCellID   string          `json:"max_voltage_cell_id,omitempty"`
	TelemetryAvailable map[string]bool `json:"telemetry_available,omitempty"`
	Name               string          `json:"name"`
	Voltage            float64         `json:"voltage"`
	Current            float64         `json:"current"`
	Power              float64         `json:"power"`
	SOC                float64         `json:"soc"`
	State              string          `json:"state,omitempty"`
	TimeToGo           string          `json:"time_to_go,omitempty"`
}

// ESSMode represents ESS mode with parsed fields
type ESSMode struct {
	BatteryLifeState int    `json:"battery_life_state"`
	Hub4Mode         int    `json:"hub4_mode"`
	IsExternal       bool   `json:"is_external"`
	ModeName         string `json:"mode_name"`
}

// SolarForecast is computed upstream by inverter-control and passed through
// to clients inside the state payload.
type SolarForecast struct {
	Date        string  `json:"date,omitempty"`
	GeneratedAt string  `json:"generated_at,omitempty"`
	TodayKWh    float64 `json:"today_kwh,omitempty"`
	TomorrowKWh float64 `json:"tomorrow_kwh,omitempty"`
}

// Notification is a dashboard banner notification: either pushed by
// inverter-control on inverter/notifications or synthesized from Victron
// alarm transitions (N/<portal>/<service>/Alarms/<Name>, value 0/1/2).
type Notification struct {
	ID     string `json:"id"`
	Level  string `json:"level"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Source string `json:"source"`
	Ts     string `json:"ts,omitempty"`
}

// CameraEvent is the latest camera event from the optional Frigate topic
// (desktop CameraEvent contract: {agent_name, video_url, timestamp}).
type CameraEvent struct {
	Camera string `json:"camera"`
	URL    string `json:"url"`
	Ts     string `json:"ts,omitempty"`
}

// Charger represents MPPT charger data
type Charger struct {
	Instance           string          `json:"instance,omitempty"`
	Serial             string          `json:"serial,omitempty"`
	TelemetryAvailable map[string]bool `json:"telemetry_available,omitempty"`
	Name               string          `json:"name"`
	PVVoltage          float64         `json:"pv_voltage,omitempty"`
	Current            float64         `json:"current"`
	Power              float64         `json:"power"`
}

// State represents complete dashboard state
type State struct {
	UIConfig            map[string]interface{} `json:"ui_config,omitempty"`
	DVCCLimits          map[string]interface{} `json:"dvcc_limits"`
	Limits              map[string]interface{} `json:"limits,omitempty"`
	Perf                map[string]interface{} `json:"perf,omitempty"`
	LoopInterval        float64                `json:"loop_interval,omitempty"`
	GridControlValid    *bool                  `json:"grid_control_valid,omitempty"`
	GridControlReason   *string                `json:"grid_control_reason"`
	GridLossState       string                 `json:"grid_loss_state,omitempty"`
	GridLossHoldSeconds *float64               `json:"grid_loss_hold_seconds"`
	GridLossElapsed     *float64               `json:"grid_loss_elapsed"`
	GridLossRemaining   *float64               `json:"grid_loss_remaining"`
	GridLossZeroApplied *bool                  `json:"grid_loss_zero_applied,omitempty"`
	// Explicit false marks an invalidated direct measurement; absent allows legacy data.
	TelemetryAvailable map[string]bool `json:"telemetry_available,omitempty"`
	G3                 float64         `json:"g3"`
	T3                 float64         `json:"t3"`
	WaterValveMode     int             `json:"water_valve_mode"`
	PumpMode           int             `json:"pump_mode"`
	// Using interface for booleans to match reference
	Booleans      map[string]interface{} `json:"booleans"`
	Features      map[string]interface{} `json:"features"`
	DailyStats    DailyStats             `json:"daily_stats"`
	ESSMode       ESSMode                `json:"ess_mode"`
	SolarForecast *SolarForecast         `json:"solar_forecast,omitempty"`

	// Core metrics
	SolarTotal        float64        `json:"solar_total"`
	MpptTotal         float64        `json:"mppt_total"`
	PVInverterTotal   float64        `json:"pv_inverter_total"`
	PVTotal           float64        `json:"pv_total"`
	GT                float64        `json:"gt"`
	G1                float64        `json:"g1"`
	G2                float64        `json:"g2"`
	TT                float64        `json:"tt"`
	T1                float64        `json:"t1"`
	T2                float64        `json:"t2"`
	BC                float64        `json:"bc"`
	BV                float64        `json:"bv"`
	BP                float64        `json:"bp"`
	Setpoint          float64        `json:"setpoint"`
	BatteryVoltage    float64        `json:"battery_voltage"`
	BatteryCurrent    float64        `json:"battery_current"`
	BatteryPower      float64        `json:"battery_power"`
	BatterySOC        float64        `json:"battery_soc"`
	InverterState     string         `json:"inverter_state"`
	Uptime            float64        `json:"uptime,omitempty"`
	HAConnected       bool           `json:"ha_connected,omitempty"`
	HADirectConnected bool           `json:"ha_direct_connected,omitempty"`
	Version           string         `json:"version,omitempty"`
	DashboardVersion  string         `json:"dashboard_version,omitempty"`
	Console           []string       `json:"console,omitempty"`
	Notifications     []Notification `json:"notifications"`

	// Latest camera event (Frigate), nil until one arrives.
	CameraEvent *CameraEvent `json:"camera_event,omitempty"`

	// Arrays for detailed data
	Batteries      []Battery     `json:"batteries"`
	SolarSources   []SolarSource `json:"solar_sources,omitempty"`
	MPPTChargers   []Charger     `json:"mppt_chargers"`
	MPPTIndividual []float64     `json:"mppt_individual"`
	// AC PV inverters of any vendor: [{name?, pv_voltage, current, power}]
	PvInverters []Charger `json:"pv_inverters"`

	// Loads
	Loads     map[string]float64 `json:"loads"`
	LoadNames map[string]string  `json:"load_names"`

	// EV power is watts; ev_charging_kw is explicitly kilowatts.
	// Sourced from Cerbo MQTT (N/<portal>/ev/<i>/... and
	// N/<portal>/evcharger/<i>/...), never from Home Assistant.
	CarSOC       float64 `json:"car_soc"`
	EVChargingKW float64 `json:"ev_charging_kw"`
	EVPower      float64 `json:"ev_power"`

	// Water data - dbus-pump via Cerbo MQTT (level %, valve/pump running).
	// No omitempty: a closed valve / empty tank are valid states that must
	// reach the UI instead of being dropped as "missing".
	WaterLevel float64 `json:"water_level"`
	WaterValve bool    `json:"water_valve"`
	PumpSwitch bool    `json:"pump_switch"`

	// Appliance data - shown when running
	DishwasherRunning  bool    `json:"dishwasher_running,omitempty"`
	DishwasherDuration float64 `json:"dishwasher_duration,omitempty"`
	DishwasherTime     float64 `json:"dishwasher_time,omitempty"`
	DishwasherActive   bool    `json:"dishwasher_active,omitempty"`
	WasherTime         float64 `json:"washer_time,omitempty"`
	WasherPower        float64 `json:"washer_power,omitempty"`
	DryerTime          float64 `json:"dryer_time,omitempty"`
	DryerPower         float64 `json:"dryer_power,omitempty"`

	// Charger booleans per reference
	OnlyCharging        bool `json:"only_charging,omitempty"`
	NoFeed              bool `json:"no_feed,omitempty"`
	HouseSupport        bool `json:"house_support,omitempty"`
	ChargeBattery       bool `json:"charge_battery,omitempty"`
	DoNotSupplyCharger  bool `json:"do_not_supply_charger,omitempty"`
	SetLimitToEVCharger bool `json:"set_limit_to_ev_charger,omitempty"`
	MinimizeCharging    bool `json:"minimize_charging,omitempty"`
	DryRun              bool `json:"dry_run,omitempty"`
}

// Clone returns a deep copy of State so callers can safely read maps/slices
// without holding the MQTT client's stateMu (json.Marshal on a live State
// races with MQTT writers → "concurrent map read and map write").
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		cp := *s
		return &cp
	}
	var out State
	if err := json.Unmarshal(data, &out); err != nil {
		cp := *s
		return &cp
	}
	return &out
}
