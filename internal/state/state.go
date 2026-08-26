package state

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
	ProducedToday       float64   `json:"produced_today,omitempty"`
	ProducedDollars     float64   `json:"produced_dollars,omitempty"`
	BatteryIn           float64   `json:"battery_in,omitempty"`
	BatteryOut          float64   `json:"battery_out,omitempty"`
	BatteryInYesterday  float64   `json:"battery_in_yesterday,omitempty"`
	BatteryOutYesterday float64   `json:"battery_out_yesterday,omitempty"`
	MpptDaily           []float64 `json:"mppt_daily,omitempty"`
	PVTotalDaily        float64   `json:"pv_total_daily,omitempty"`
}

// SolarSource represents individual solar source data
type SolarSource struct {
	Name      string  `json:"name"`
	PVVoltage float64 `json:"pv_voltage,omitempty"`
	Current   float64 `json:"current,omitempty"`
	Power     float64 `json:"power,omitempty"`
}

// Battery represents individual battery data
type Battery struct {
	Name     string  `json:"name"`
	Voltage  float64 `json:"voltage"`
	Current  float64 `json:"current,omitempty"`
	Power    float64 `json:"power,omitempty"`
	SOC      float64 `json:"soc"`
	State    string  `json:"state,omitempty"`
	TimeToGo string  `json:"time_to_go,omitempty"`
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
	Name      string  `json:"name"`
	PVVoltage float64 `json:"pv_voltage,omitempty"`
	Current   float64 `json:"current,omitempty"`
	Power     float64 `json:"power,omitempty"`
}

// State represents complete dashboard state
type State struct {
	// Using interface for booleans to match reference
	Booleans      map[string]interface{} `json:"booleans"`
	Features      map[string]interface{} `json:"features"`
	DailyStats    DailyStats             `json:"daily_stats"`
	ESSMode       ESSMode                `json:"ess_mode"`
	SolarForecast *SolarForecast         `json:"solar_forecast,omitempty"`

	// Core metrics
	SolarTotal        float64        `json:"solar_total,omitempty"`
	MpptTotal         float64        `json:"mppt_total,omitempty"`
	PVTotal           float64        `json:"pv_total,omitempty"`
	GT                float64        `json:"gt,omitempty"`
	G1                float64        `json:"g1,omitempty"`
	G2                float64        `json:"g2,omitempty"`
	TT                float64        `json:"tt,omitempty"`
	T1                float64        `json:"t1,omitempty"`
	T2                float64        `json:"t2,omitempty"`
	BC                float64        `json:"bc,omitempty"`
	BV                float64        `json:"bv,omitempty"`
	BP                float64        `json:"bp,omitempty"`
	Setpoint          float64        `json:"setpoint,omitempty"`
	BatteryVoltage    float64        `json:"battery_voltage,omitempty"`
	BatteryCurrent    float64        `json:"battery_current,omitempty"`
	BatteryPower      float64        `json:"battery_power,omitempty"`
	BatterySOC        float64        `json:"battery_soc,omitempty"`
	InverterState     string         `json:"inverter_state,omitempty"`
	Uptime            float64        `json:"uptime,omitempty"`
	HAConnected       bool           `json:"ha_connected,omitempty"`
	HADirectConnected bool           `json:"ha_direct_connected,omitempty"`
	Version           string         `json:"version,omitempty"`
	DashboardVersion  string         `json:"dashboard_version,omitempty"`
	Console           []string       `json:"console,omitempty"`
	Notifications     []Notification `json:"notifications,omitempty"`

	// Latest camera event (Frigate), nil until one arrives.
	CameraEvent *CameraEvent `json:"camera_event,omitempty"`

	// Arrays for detailed data
	Batteries      []Battery     `json:"batteries,omitempty"`
	SolarSources   []SolarSource `json:"solar_sources,omitempty"`
	MPPTChargers   []Charger     `json:"mppt_chargers,omitempty"`
	MPPTIndividual []float64     `json:"mppt_individual,omitempty"`
	// AC PV inverters of any vendor: [{name?, pv_voltage, current, power}]
	PvInverters []Charger `json:"pv_inverters,omitempty"`

	// Loads
	Loads map[string]float64 `json:"loads,omitempty"`

	// EV data - always shown

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
