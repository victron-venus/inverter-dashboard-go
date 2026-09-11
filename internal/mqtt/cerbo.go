package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
)

// Live tiles owned by Cerbo MQTT (mirrored out of slim inverter/state).
var cerboOwnedKeys = map[string]struct{}{
	"g1": {}, "g2": {}, "gt": {}, "t1": {}, "t2": {}, "tt": {},
	"bv": {}, "bc": {}, "bp": {},
	"battery_soc": {}, "battery_power": {}, "battery_voltage": {}, "battery_current": {},
	"batteries":   {},
	"solar_total": {}, "pv_total": {}, "mppt_total": {},
	"mppt_data": {}, "mppt_individual": {}, "mppt_chargers": {},
	"pv_inverter_total": {}, "pv_inverter_individual": {}, "pv_inverter_powers": {}, "pv_inverters": {},
	"loads": {}, "load_names": {},
	"setpoint": {}, "inverter_state": {},
	"ev_power": {}, "car_soc": {}, "ev_charging_kw": {},
	"water_level": {}, "water_valve": {}, "pump_switch": {},
}

var inverterStates = map[int]string{
	0: "Off", 1: "Low Power", 2: "Fault", 3: "Bulk", 4: "Absorption",
	5: "Float", 6: "Storage", 7: "Equalize", 8: "Passthru", 9: "Inverting",
	10: "Power assist", 11: "Power supply", 252: "External control",
}

const (
	vSocMin           = 40.0
	vSocMax           = 54.4
	keepaliveInterval = 45 * time.Second
)

// VoltageSOC maps pack voltage to 0–100% SoC (absorption at 54.4 V).
func VoltageSOC(voltage float64) float64 {
	pct := ((voltage - vSocMin) / (vSocMax - vSocMin)) * 100.0
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return float64(int(pct + 0.5)) // round half up like Python round()
}

func stateFromCurrent(amps float64) string {
	if amps > 0.5 {
		return "Charging"
	}
	if amps < -0.5 {
		return "Discharging"
	}
	return "Idle"
}

func parseCerboPayload(payload []byte) (interface{}, bool) {
	var data struct {
		Value interface{} `json:"value"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, false
	}
	return data.Value, true
}

type cerboBattery struct {
	Instance string
	Name     string
	Serial   string
	SOC      float64
	Voltage  float64
	Current  float64
	Power    float64
	State    string
	HasSOC   bool
	HasV     bool
	HasI     bool
	HasP     bool
}

type cerboCharger struct {
	Name      string
	Serial    string
	Power     float64
	PVVoltage float64
	Current   float64
	HasPower  bool
}

type cerboSystem struct {
	G1, G2, T1, T2             float64
	HasG1, HasG2, HasT1, HasT2 bool
}

type cerboVebus struct {
	L1Power, L2Power, ACPower, Setpoint  float64
	InverterState                        string
	HasL1, HasL2, HasAC, HasSP, HasState bool
}

type cerboACLoad struct {
	Power       float64
	CustomName  string
	ProductName string
	HasPower    bool
}

// initCerboMaps ensures Cerbo device maps are allocated.
func (c *Client) initCerboMaps() {
	if c.batteries == nil {
		c.batteries = make(map[string]*cerboBattery)
	}
	if c.chargers == nil {
		c.chargers = make(map[string]*cerboCharger)
	}
	if c.system == nil {
		c.system = make(map[string]*cerboSystem)
	}
	if c.vebus == nil {
		c.vebus = make(map[string]*cerboVebus)
	}
	if c.acloads == nil {
		c.acloads = make(map[string]*cerboACLoad)
	}
	if c.pvInverters == nil {
		c.pvInverters = make(map[int]*state.Charger)
	}
}

func (c *Client) cerboHasOverlay(key string) bool {
	hasACLoads := len(c.acloads) > 0
	hasSystem := len(c.system) > 0
	hasVebus := len(c.vebus) > 0
	hasBatteries := len(c.batteries) > 0
	hasChargers := len(c.chargers) > 0
	hasPV := len(c.pvInverters) > 0
	st := c.state
	hasEV := st != nil && (st.EVPower != 0 || st.CarSOC != 0 || st.EVChargingKW != 0)
	hasWater := st != nil && (st.WaterLevel != 0 || st.WaterValve || st.PumpSwitch)
	owned := map[string]bool{
		"loads": hasACLoads, "load_names": hasACLoads,
		"g1": hasSystem || hasVebus, "g2": hasSystem || hasVebus, "gt": hasSystem || hasVebus,
		"t1": hasSystem || hasVebus, "t2": hasSystem || hasVebus, "tt": hasSystem || hasVebus,
		"battery_soc": hasBatteries, "battery_power": hasBatteries,
		"battery_voltage": hasBatteries, "battery_current": hasBatteries,
		"bv": hasBatteries, "bc": hasBatteries, "bp": hasBatteries, "batteries": hasBatteries,
		"solar_total": hasChargers || hasPV, "mppt_total": hasChargers || hasPV,
		"mppt_chargers": hasChargers || hasPV, "mppt_individual": hasChargers || hasPV,
		"mppt_data": hasChargers || hasPV, "pv_total": hasChargers || hasPV,
		"pv_inverters": hasPV, "pv_inverter_total": hasPV,
		"pv_inverter_individual": hasPV, "pv_inverter_powers": hasPV,
		"setpoint": hasVebus, "inverter_state": hasVebus,
		"ev_power": hasEV, "car_soc": hasEV, "ev_charging_kw": hasEV,
		"water_level": hasWater, "water_valve": hasWater, "pump_switch": hasWater,
	}
	return owned[key]
}

// mergeDaemonState non-destructively merges slim inverter/state into current state.
// Cerbo-owned live tiles are never taken from the daemon once overlays exist.
func (c *Client) mergeDaemonState(data map[string]interface{}) {
	c.initCerboMaps()
	for key := range data {
		if _, owned := cerboOwnedKeys[key]; owned && c.cerboHasOverlay(key) {
			delete(data, key)
		}
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal daemon state: %v", err)
		return
	}
	if err := json.Unmarshal(dataJSON, c.state); err != nil {
		log.Printf("Failed to unmarshal daemon state: %v", err)
	}
	c.state.DashboardVersion = version.GetCurrent()
	if ver, ok := data["version"].(string); ok {
		c.state.Version = ver
	}
	c.applyCerboOverlays()
}

func (c *Client) findShunt() *cerboBattery {
	for _, b := range c.batteries {
		if strings.Contains(strings.ToLower(b.Name), "shunt") {
			return b
		}
	}
	return nil
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, aerr := strconv.Atoi(keys[i])
		bi, berr := strconv.Atoi(keys[j])
		if aerr == nil && berr == nil {
			return ai < bi
		}
		return keys[i] < keys[j]
	})
	return keys
}

func (c *Client) applyCerboOverlays() {
	st := c.state
	if st == nil {
		return
	}
	c.syncACLoadToState()

	if len(c.batteries) > 0 {
		batteries := make([]state.Battery, 0, len(c.batteries))
		for _, inst := range sortedStringKeys(c.batteries) {
			b := c.batteries[inst]
			name := b.Name
			if name == "" {
				name = "Battery " + inst
			}
			batteries = append(batteries, state.Battery{
				Name: name, Voltage: b.Voltage, Current: b.Current,
				Power: b.Power, SOC: b.SOC, State: b.State,
			})
		}
		if shunt := c.findShunt(); shunt != nil {
			if shunt.HasV {
				st.BatterySOC = VoltageSOC(shunt.Voltage)
				st.BatteryVoltage = shunt.Voltage
				st.BV = shunt.Voltage
			}
			if shunt.HasI {
				st.BatteryCurrent = shunt.Current
				st.BC = shunt.Current
			}
			if shunt.HasP {
				st.BatteryPower = shunt.Power
				st.BP = shunt.Power
			}
		}
		st.Batteries = batteries
	}

	mpptTotal := 0.0
	if len(c.chargers) > 0 {
		chargers := make([]state.Charger, 0, len(c.chargers))
		individuals := make([]float64, 0, len(c.chargers))
		for _, inst := range sortedStringKeys(c.chargers) {
			ch := c.chargers[inst]
			chargers = append(chargers, state.Charger{
				Name: ch.Name, PVVoltage: ch.PVVoltage, Current: ch.Current, Power: ch.Power,
			})
			individuals = append(individuals, ch.Power)
			mpptTotal += ch.Power
		}
		st.MPPTChargers = chargers
		st.MpptTotal = mpptTotal
		st.MPPTIndividual = individuals
	} else {
		mpptTotal = st.MpptTotal
	}

	pvTotal := 0.0
	if len(c.pvInverters) > 0 {
		instances := make([]int, 0, len(c.pvInverters))
		for i := range c.pvInverters {
			instances = append(instances, i)
		}
		sort.Ints(instances)
		list := make([]state.Charger, 0, len(instances))
		for _, i := range instances {
			list = append(list, *c.pvInverters[i])
			pvTotal += c.pvInverters[i].Power
		}
		st.PvInverters = list
	} else {
		for _, p := range st.PvInverters {
			pvTotal += p.Power
		}
	}

	if len(c.chargers) > 0 || len(c.pvInverters) > 0 {
		st.SolarTotal = mpptTotal + pvTotal
		st.PVTotal = pvTotal
	}

	if len(c.system) > 0 {
		var s *cerboSystem
		for _, inst := range sortedStringKeys(c.system) {
			s = c.system[inst]
			break
		}
		if s.HasG1 {
			st.G1 = s.G1
		}
		if s.HasG2 {
			st.G2 = s.G2
		}
		if s.HasT1 {
			st.T1 = s.T1
		}
		if s.HasT2 {
			st.T2 = s.T2
		}
		if s.HasG1 || s.HasG2 {
			st.GT = 0
			if s.HasG1 {
				st.GT += s.G1
			}
			if s.HasG2 {
				st.GT += s.G2
			}
		}
		if s.HasT1 || s.HasT2 {
			st.TT = 0
			if s.HasT1 {
				st.TT += s.T1
			}
			if s.HasT2 {
				st.TT += s.T2
			}
		}
	}

	if len(c.vebus) > 0 {
		var v *cerboVebus
		for _, inst := range sortedStringKeys(c.vebus) {
			v = c.vebus[inst]
			break
		}
		// Prefer systemcalc; only fill from vebus when system hasn't set g1/g2/gt.
		if len(c.system) == 0 {
			if v.HasL1 {
				st.G1 = v.L1Power
			}
			if v.HasL2 {
				st.G2 = v.L2Power
			}
			if v.HasL1 && v.HasL2 {
				st.GT = v.L1Power + v.L2Power
			} else if v.HasAC {
				st.GT = v.ACPower
			}
		}
		if v.HasSP {
			st.Setpoint = v.Setpoint
		}
		if v.HasState {
			st.InverterState = v.InverterState
		}
	}
}

func (c *Client) syncACLoadToState() {
	if len(c.acloads) == 0 {
		return
	}
	loadsNamed := make(map[string]float64)
	names := make(map[string]string)
	for _, inst := range sortedStringKeys(c.acloads) {
		entry := c.acloads[inst]
		if !entry.HasPower {
			continue
		}
		label := entry.CustomName
		if label == "" {
			label = entry.ProductName
		}
		if label == "" {
			label = "AC Load " + inst
		}
		names[inst] = label
		key := label
		if strings.HasPrefix(label, "AC Load ") {
			key = strings.ToLower(strings.ReplaceAll(label, " ", "_"))
		}
		if _, exists := loadsNamed[key]; exists && key != inst {
			key = fmt.Sprintf("%s_%s", key, inst)
		}
		loadsNamed[key] = entry.Power
	}
	c.state.Loads = loadsNamed
	c.state.LoadNames = names
}

func (c *Client) handleCerboDevice(topic string, payload []byte) bool {
	parts := strings.Split(topic, "/")
	if len(parts) < 5 || parts[0] != "N" {
		return false
	}
	kind, instance := parts[2], parts[3]
	if kind != "system" && kind != "battery" && kind != "solarcharger" && kind != "vebus" {
		return false
	}
	path := strings.Join(parts[4:], "/")
	val, ok := parseCerboPayload(payload)
	if !ok {
		return false
	}
	c.initCerboMaps()
	changed := false

	switch kind {
	case "system":
		entry, ok := c.system[instance]
		if !ok {
			entry = &cerboSystem{}
			c.system[instance] = entry
		}
		num, okNum := toFloat(val)
		if !okNum {
			return false
		}
		switch path {
		case "Ac/Grid/L1/Power":
			entry.G1, entry.HasG1, changed = num, true, true
		case "Ac/Grid/L2/Power":
			entry.G2, entry.HasG2, changed = num, true, true
		case "Ac/Consumption/L1/Power":
			entry.T1, entry.HasT1, changed = num, true, true
		case "Ac/Consumption/L2/Power":
			entry.T2, entry.HasT2, changed = num, true, true
		}
	case "battery":
		entry, ok := c.batteries[instance]
		if !ok {
			entry = &cerboBattery{Instance: instance}
			c.batteries[instance] = entry
		}
		switch path {
		case "Soc":
			if num, okNum := toFloat(val); okNum {
				entry.SOC, entry.HasSOC, changed = num, true, true
			}
		case "Dc/0/Voltage":
			if num, okNum := toFloat(val); okNum {
				entry.Voltage, entry.HasV, changed = num, true, true
			}
		case "Dc/0/Current":
			if num, okNum := toFloat(val); okNum {
				entry.Current, entry.HasI = num, true
				entry.State = stateFromCurrent(num)
				changed = true
			}
		case "Dc/0/Power":
			if num, okNum := toFloat(val); okNum {
				entry.Power, entry.HasP, changed = num, true, true
			}
		case "ProductName":
			if name, okStr := val.(string); okStr && strings.TrimSpace(name) != "" {
				if entry.Name == "" {
					entry.Name = strings.TrimSpace(name)
					changed = true
				}
			}
		case "CustomName":
			if name, okStr := val.(string); okStr && strings.TrimSpace(name) != "" {
				entry.Name = strings.TrimSpace(name)
				changed = true
			}
		case "Serial":
			if s, okStr := val.(string); okStr && strings.TrimSpace(s) != "" {
				entry.Serial = strings.TrimSpace(s)
				changed = true
			}
		}
	case "solarcharger":
		entry, ok := c.chargers[instance]
		if !ok {
			entry = &cerboCharger{}
			c.chargers[instance] = entry
		}
		switch path {
		case "Yield/Power":
			if num, okNum := toFloat(val); okNum {
				entry.Power, entry.HasPower, changed = num, true, true
			}
		case "Pv/V":
			if num, okNum := toFloat(val); okNum {
				entry.PVVoltage, changed = num, true
			}
		case "Dc/0/Current":
			if num, okNum := toFloat(val); okNum {
				entry.Current, changed = num, true
			}
		case "ProductName":
			if name, okStr := val.(string); okStr && strings.TrimSpace(name) != "" {
				entry.Name = strings.TrimSpace(name)
				changed = true
			}
		case "Serial":
			if s, okStr := val.(string); okStr && strings.TrimSpace(s) != "" {
				entry.Serial = strings.TrimSpace(s)
				changed = true
			}
		}
	case "vebus":
		entry, ok := c.vebus[instance]
		if !ok {
			entry = &cerboVebus{}
			c.vebus[instance] = entry
		}
		switch path {
		case "Ac/ActiveIn/L1/Power", "Ac/L1/Power":
			if num, okNum := toFloat(val); okNum {
				entry.L1Power, entry.HasL1, changed = num, true, true
			}
		case "Ac/ActiveIn/L2/Power", "Ac/L2/Power":
			if num, okNum := toFloat(val); okNum {
				entry.L2Power, entry.HasL2, changed = num, true, true
			}
		case "Ac/Out/P", "Ac/Power":
			if num, okNum := toFloat(val); okNum {
				entry.ACPower, entry.HasAC, changed = num, true, true
			}
		case "Hub4/L1/AcPowerSetpoint":
			if num, okNum := toFloat(val); okNum {
				entry.Setpoint, entry.HasSP, changed = num, true, true
			}
		case "State":
			if num, okNum := toFloat(val); okNum {
				code := int(num)
				name, okName := inverterStates[code]
				if !okName {
					name = fmt.Sprintf("? (%d)", code)
				}
				entry.InverterState, entry.HasState, changed = name, true, true
			}
		}
	}
	if changed {
		c.applyCerboOverlays()
	}
	return changed
}

func (c *Client) handleACLoad(topic string, payload []byte) bool {
	parts := strings.Split(topic, "/")
	if len(parts) < 5 || parts[0] != "N" || parts[2] != "acload" {
		return false
	}
	instance := parts[3]
	path := strings.Join(parts[4:], "/")
	val, ok := parseCerboPayload(payload)
	if !ok {
		return false
	}
	c.initCerboMaps()
	entry, ok := c.acloads[instance]
	if !ok {
		entry = &cerboACLoad{}
		c.acloads[instance] = entry
	}
	changed := false
	switch path {
	case "Ac/Power", "Ac/L1/Power":
		if num, okNum := toFloat(val); okNum {
			entry.Power, entry.HasPower, changed = num, true, true
		}
	case "CustomName":
		if name, okStr := val.(string); okStr && strings.TrimSpace(name) != "" {
			entry.CustomName = strings.TrimSpace(name)
			changed = true
		}
	case "ProductName":
		if name, okStr := val.(string); okStr && strings.TrimSpace(name) != "" {
			entry.ProductName = strings.TrimSpace(name)
			if entry.CustomName == "" {
				changed = true
			}
		}
	}
	if changed {
		c.syncACLoadToState()
	}
	return changed
}

func (c *Client) onCerboLiveMessage(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()
	changed := false
	c.stateMu.Lock()
	switch {
	case strings.Contains(topic, "/acload/"):
		changed = c.handleACLoad(topic, payload)
	case strings.Contains(topic, "/system/") || strings.Contains(topic, "/battery/") ||
		strings.Contains(topic, "/solarcharger/") || strings.Contains(topic, "/vebus/"):
		changed = c.handleCerboDevice(topic, payload)
	}
	c.stateMu.Unlock()
	if changed {
		c.triggerHandler()
	}
}

func (c *Client) onPortalMessage(_ mqtt.Client, msg mqtt.Message) {
	portal := strings.TrimSpace(string(msg.Payload()))
	portal = strings.Trim(portal, "\"")
	if portal == "" {
		return
	}
	c.stateMu.Lock()
	prev := c.portalID
	if portal == prev {
		c.stateMu.Unlock()
		return
	}
	c.portalID = portal
	c.stateMu.Unlock()
	log.Printf("Discovered Cerbo portal ID via inverter/portal: %s", portal)
	c.subscribePortalTopics(portal)
}

func (c *Client) subscribePortalTopics(portal string) {
	if portal == "" || c.client == nil || !c.client.IsConnected() {
		return
	}
	subs := []struct {
		topic   string
		handler mqtt.MessageHandler
	}{
		{fmt.Sprintf("N/%s/+/Alarms/#", portal), c.onAlarmMessage},
		{fmt.Sprintf("N/%s/+/+/Alarms/#", portal), c.onAlarmMessage},
		// Venus GUIv2 notification slots (preferred over raw Alarms/* once seen).
		{fmt.Sprintf("N/%s/platform/+/Notifications/#", portal), c.onPlatformNotificationMessage},
		{fmt.Sprintf("N/%s/tank/+/Level", portal), c.onWaterMessage},
		{fmt.Sprintf("N/%s/pump/+/State", portal), c.onWaterMessage},
		{fmt.Sprintf("N/%s/ev/%d/Soc", portal, c.evInstance), c.onEVMessage},
		{fmt.Sprintf("N/%s/ev/%d/Ac/Power", portal, c.evInstance), c.onEVMessage},
		{fmt.Sprintf("N/%s/evcharger/%d/Ac/Power", portal, c.evchargerInstance), c.onEVMessage},
	}
	for _, s := range subs {
		if token := c.client.Subscribe(s.topic, 0, s.handler); token.Wait() && token.Error() != nil {
			log.Printf("Warning: failed to subscribe to %s: %v", s.topic, token.Error())
		}
	}
	log.Printf("Subscribed to Cerbo water/EV/alarm/platform topics for portal %s", portal)
}

func (c *Client) publishKeepalive() {
	c.stateMu.RLock()
	portal := c.portalID
	c.stateMu.RUnlock()
	if portal == "" || c.client == nil || !c.client.IsConnected() {
		return
	}
	topic := fmt.Sprintf("R/%s/keepalive", portal)
	if token := c.client.Publish(topic, 0, false, ""); token.Wait() && token.Error() != nil {
		log.Printf("Cerbo keepalive publish failed: %v", token.Error())
	}
}

func (c *Client) startKeepalive() {
	c.stopKeepalive()
	c.keepaliveStop = make(chan struct{})
	// Immediate keepalive so Venus starts streaming N/ topics without waiting 45s.
	c.publishKeepalive()
	go func() {
		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.keepaliveStop:
				return
			case <-ticker.C:
				c.publishKeepalive()
			}
		}
	}()
}

func (c *Client) stopKeepalive() {
	if c.keepaliveStop != nil {
		close(c.keepaliveStop)
		c.keepaliveStop = nil
	}
}

// PortalID returns the configured or discovered Cerbo portal id.
func (c *Client) PortalID() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.portalID
}
