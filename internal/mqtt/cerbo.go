package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
	"github.com/victron-venus/inverter-dashboard-go/internal/version"
)

const keepaliveInterval = 45 * time.Second

var inverterStates = map[int]string{0: "Off", 1: "Low Power", 2: "Fault", 3: "Bulk", 4: "Absorption", 5: "Float", 6: "Storage", 7: "Equalize", 8: "Passthru", 9: "Inverting", 10: "Power assist", 11: "Power supply", 252: "External control"}

// CerboOptions selects explicitly configured device instances; zero is a valid instance.
type CerboOptions struct{ TankInstance, PumpInstance, ValveInstance, EVInstance, EVChargerInstance int }

var directFields = []string{"g1", "g2", "g3", "gt", "t1", "t2", "t3", "tt", "bv", "bc", "bp", "battery_soc", "battery_voltage", "battery_current", "battery_power", "setpoint", "inverter_state", "solar_total", "mppt_total", "pv_total", "pv_inverter_total", "batteries", "mppt_chargers", "mppt_individual", "pv_inverters", "loads", "load_names", "ev_power", "ev_charging_kw", "car_soc", "water_level", "water_valve", "pump_switch", "water_valve_mode", "pump_mode"}

func emptyAvailability() map[string]bool {
	m := map[string]bool{}
	for _, k := range directFields {
		m[k] = false
	}
	return m
}

// VoltageSOC remains for source compatibility. Telemetry never estimates SOC
// from voltage: the relationship depends on chemistry, temperature and load.
func VoltageSOC(v float64) float64 {
	return math.Round(math.Max(0, math.Min(100, (v-40)/(54.4-40)*100)))
}
func stateFromCurrent(v float64) string {
	if v > 0.5 {
		return "Charging"
	}
	if v < -0.5 {
		return "Discharging"
	}
	return "Idle"
}
func parseCerboPayload(payload []byte) (interface{}, bool) {
	var body map[string]interface{}
	if json.Unmarshal(payload, &body) != nil {
		return nil, false
	}
	value, ok := body["value"]
	return value, ok
}
func number(v interface{}) (float64, bool) {
	var n float64
	switch v := v.(type) {
	case float64:
		n = v
	case float32:
		n = float64(v)
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0)
}
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, ea := strconv.Atoi(keys[i])
		b, eb := strconv.Atoi(keys[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})
	return keys
}

type leaves map[string]interface{}

func (d leaves) num(paths ...string) (float64, bool) {
	for _, p := range paths {
		if n, ok := number(d[p]); ok {
			return n, true
		}
	}
	return 0, false
}
func (d leaves) str(paths ...string) string {
	for _, p := range paths {
		if s, ok := d[p].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
func (d leaves) sum(paths ...string) (float64, bool) {
	total := 0.0
	valid := false
	for _, p := range paths {
		if n, ok := d.num(p); ok {
			total += n
			valid = true
		}
	}
	return total, valid
}
func (d leaves) acPower() (float64, bool) {
	if n, ok := d.num("Ac/Power"); ok {
		return n, true
	}
	return d.sum("Ac/L1/Power", "Ac/L2/Power", "Ac/L3/Power")
}
func (d leaves) dcPower() (float64, bool) {
	if n, ok := d.num("Dc/0/Power"); ok {
		return n, true
	}
	v, hv := d.num("Dc/0/Voltage")
	i, hi := d.num("Dc/0/Current")
	return v * i, hv && hi
}
func (d leaves) enabled() bool { n, ok := d.num("Connected"); return !ok || n != 0 }
func devices(all map[string]map[string]interface{}, kind string) map[string]leaves {
	out := map[string]leaves{}
	for path, value := range all[kind] {
		id, p, ok := strings.Cut(path, "/")
		if !ok {
			continue
		}
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}
		if out[id] == nil {
			out[id] = leaves{}
		}
		out[id][p] = value
	}
	for id, d := range out {
		if !d.enabled() {
			delete(out, id)
		}
	}
	return out
}
func deviceName(d leaves, fallback string) string {
	if s := d.str("CustomName", "ProductName"); s != "" {
		return s
	}
	return fallback
}
func firstNum(ds map[string]leaves, paths ...string) (float64, bool) {
	for _, id := range sortedStringKeys(ds) {
		if n, ok := ds[id].num(paths...); ok {
			return n, true
		}
	}
	return 0, false
}

// cerboOverlay is the canonical reducer for LAN messages and gateway snapshots.
// Metadata alone never claims an unrelated scalar measurement.
func cerboOverlay(all map[string]map[string]interface{}, o CerboOptions) map[string]interface{} {
	out := map[string]interface{}{}
	put := func(key string, n float64, ok bool) {
		if ok {
			out[key] = n
		}
	}
	sys, grid, vebus := devices(all, "system"), devices(all, "grid"), devices(all, "vebus")
	for phase := 1; phase <= 3; phase++ {
		p := fmt.Sprintf("L%d", phase)
		n, ok := firstNum(sys, "Ac/Grid/"+p+"/Power")
		if !ok {
			n, ok = firstNum(grid, "Ac/"+p+"/Power")
		}
		if !ok { // Active input is grid only when systemcalc identifies it as mains/shore.
			input, known := firstNum(sys, "Ac/ActiveIn/Source")
			if known && (input == 1 || input == 3) {
				for _, id := range sortedStringKeys(vebus) {
					d := vebus[id]
					if connected, known := d.num("Ac/ActiveIn/Connected"); known && connected == 0 {
						continue
					}
					if n, ok = d.num("Ac/ActiveIn/"+p+"/P", "Ac/ActiveIn/"+p+"/Power"); ok {
						break
					}
				}
			}
		}
		put(fmt.Sprintf("g%d", phase), n, ok)
		n, ok = firstNum(sys, "Ac/Consumption/"+p+"/Power")
		if !ok { // Some systemcalc versions publish input/output consumption separately.
			a, ha := firstNum(sys, "Ac/ConsumptionOnInput/"+p+"/Power")
			b, hb := firstNum(sys, "Ac/ConsumptionOnOutput/"+p+"/Power")
			n, ok = a+b, ha || hb
		}
		put(fmt.Sprintf("t%d", phase), n, ok)
	}
	for _, group := range []struct{ prefix, total string }{{"g", "gt"}, {"t", "tt"}} {
		total, have := 0.0, false
		for phase := 1; phase <= 3; phase++ {
			if n, ok := out[fmt.Sprintf("%s%d", group.prefix, phase)].(float64); ok {
				total += n
				have = true
			}
		}
		if !have && group.total == "gt" {
			total, have = firstNum(grid, "Ac/Power")
		}
		put(group.total, total, have)
	}
	n, ok := firstNum(vebus, "Hub4/L1/AcPowerSetpoint")
	put("setpoint", n, ok)
	if code, ok := firstNum(vebus, "State"); ok {
		name, known := inverterStates[int(code)]
		if !known {
			name = fmt.Sprintf("? (%d)", int(code))
		}
		out["inverter_state"] = name
	}

	bats := devices(all, "battery")
	batteryList := []state.Battery{}
	for _, id := range sortedStringKeys(bats) {
		d := bats[id]
		b := state.Battery{Instance: id, Name: deviceName(d, "Battery "+id), Serial: d.str("Serial"), TelemetryAvailable: map[string]bool{}}
		b.Voltage, b.TelemetryAvailable["voltage"] = d.num("Dc/0/Voltage")
		b.Current, b.TelemetryAvailable["current"] = d.num("Dc/0/Current")
		b.Power, b.TelemetryAvailable["power"] = d.dcPower()
		b.SOC, b.TelemetryAvailable["soc"] = d.num("Soc")
		if b.TelemetryAvailable["current"] {
			b.State = stateFromCurrent(b.Current)
		}
		if seconds, ok := d.num("TimeToGo"); ok && seconds > 0 && seconds < 14*86400 && b.State != "Idle" {
			b.TimeToGo = fmt.Sprintf("%dh %02dm", int(seconds)/3600, int(seconds)%3600/60)
		}
		for _, v := range []struct {
			path string
			dest **float64
		}{{"Dc/0/Temperature", &b.Temperature}, {"System/MinCellVoltage", &b.MinCellVoltage}, {"System/MaxCellVoltage", &b.MaxCellVoltage}} {
			if n, ok := d.num(v.path); ok {
				*v.dest = &n
			}
		}
		b.MinVoltageCellID = d.str("System/MinVoltageCellId")
		b.MaxVoltageCellID = d.str("System/MaxVoltageCellId")
		batteryList = append(batteryList, b)
	}
	if len(bats) > 0 {
		out["batteries"] = batteryList
	}
	// The systemcalc-selected battery is authoritative. Device fallback is safe
	// only for an explicit monitor instance or one unambiguous battery service.
	selected := ""
	if n, ok := firstNum(sys, "Dc/Battery/Instance"); ok {
		selected = strconv.Itoa(int(n))
	}
	if selected == "" && len(bats) == 1 {
		for id := range bats {
			selected = id
		}
	}
	battery := bats[selected]
	for _, f := range []struct{ key, alias, system, path string }{{"battery_voltage", "bv", "Voltage", "Dc/0/Voltage"}, {"battery_current", "bc", "Current", "Dc/0/Current"}, {"battery_power", "bp", "Power", "Dc/0/Power"}, {"battery_soc", "", "Soc", "Soc"}} {
		n, ok := firstNum(sys, "Dc/Battery/"+f.system)
		if !ok {
			if f.key == "battery_power" {
				n, ok = battery.dcPower()
			} else {
				n, ok = battery.num(f.path)
			}
		}
		put(f.key, n, ok)
		if f.alias != "" {
			put(f.alias, n, ok)
		}
	}

	mppts, pvs := devices(all, "solarcharger"), devices(all, "pvinverter")
	mpptTotal, pvTotal := 0.0, 0.0
	hasMPPT, hasPV := false, false
	for _, group := range []struct {
		ds   map[string]leaves
		kind string
	}{{mppts, "mppt"}, {pvs, "pv"}} {
		list := []state.Charger{}
		powers := []float64{}
		for _, id := range sortedStringKeys(group.ds) {
			d := group.ds[id]
			ch := state.Charger{Instance: id, Name: deviceName(d, map[string]string{"mppt": "Solar charger ", "pv": "PV inverter "}[group.kind]+id), Serial: d.str("Serial"), TelemetryAvailable: map[string]bool{}}
			if group.kind == "mppt" {
				ch.Power, ok = d.num("Yield/Power")
				if !ok {
					ch.Power, ok = d.dcPower()
				}
				ch.PVVoltage, ch.TelemetryAvailable["pv_voltage"] = d.num("Pv/V")
				ch.Current, ch.TelemetryAvailable["current"] = d.num("Dc/0/Current")
			} else {
				ch.Power, ok = d.acPower()
				ch.PVVoltage, ch.TelemetryAvailable["pv_voltage"] = d.num("Ac/L1/Voltage")
				ch.Current, ch.TelemetryAvailable["current"] = d.num("Ac/L1/Current")
			}
			ch.TelemetryAvailable["power"] = ok
			list = append(list, ch)
			if ok {
				powers = append(powers, ch.Power)
				if group.kind == "mppt" {
					mpptTotal += ch.Power
					hasMPPT = true
				} else {
					pvTotal += ch.Power
					hasPV = true
				}
			}
		}
		if len(group.ds) > 0 {
			if group.kind == "mppt" {
				out["mppt_chargers"] = list
				out["mppt_individual"] = powers
			} else {
				out["pv_inverters"] = list
			}
		}
	}
	// System totals cover the complete installation, even while only a subset
	// of individual devices has published its first samples.
	if n, ok := firstNum(sys, "Dc/Pv/Power"); ok {
		mpptTotal, hasMPPT = n, true
	}
	systemPV, hasSystemPV := 0.0, false
	for _, location := range []string{"PvOnGrid", "PvOnOutput", "PvOnGenset"} {
		for phase := 1; phase <= 3; phase++ {
			if n, ok := firstNum(sys, fmt.Sprintf("Ac/%s/L%d/Power", location, phase)); ok {
				systemPV += n
				hasSystemPV = true
			}
		}
	}
	if hasSystemPV {
		pvTotal, hasPV = systemPV, true
	}
	put("mppt_total", mpptTotal, hasMPPT)
	put("pv_total", mpptTotal, hasMPPT)
	put("pv_inverter_total", pvTotal, hasPV)
	if hasMPPT || hasPV {
		out["solar_total"] = mpptTotal + pvTotal
	}

	loads, loadNames := map[string]float64{}, map[string]string{}
	ac := devices(all, "acload")
	for _, id := range sortedStringKeys(ac) {
		d := ac[id]
		if p, ok := d.acPower(); ok {
			name := deviceName(d, "AC Load "+id)
			key := name
			if strings.HasPrefix(name, "AC Load ") {
				key = strings.ToLower(strings.ReplaceAll(name, " ", "_"))
			}
			if _, exists := loads[key]; exists {
				key += "_" + id
			}
			loads[key] = p
			loadNames[id] = name
		}
	}
	if len(ac) > 0 {
		out["loads"] = loads
		out["load_names"] = loadNames
	}
	for _, f := range []struct {
		kind      string
		inst      int
		path, key string
		scale     float64
	}{{"tank", o.TankInstance, "Level", "water_level", 1}, {"ev", o.EVInstance, "Soc", "car_soc", 1}, {"ev", o.EVInstance, "Ac/Power", "ev_power", 1}, {"evcharger", o.EVChargerInstance, "Ac/Power", "ev_charging_kw", .001}, {"pump", o.PumpInstance, "State", "pump_switch", 1}, {"pump", o.ValveInstance, "State", "water_valve", 1}, {"pump", o.PumpInstance, "Mode", "pump_mode", 1}, {"pump", o.ValveInstance, "Mode", "water_valve_mode", 1}} {
		d := devices(all, f.kind)[strconv.Itoa(f.inst)]
		if n, ok := d.num(f.path); ok {
			switch f.key {
			case "pump_switch", "water_valve":
				out[f.key] = n != 0
			case "pump_mode", "water_valve_mode":
				out[f.key] = int(n)
			default:
				out[f.key] = n * f.scale
			}
		}
	}
	return out
}

func (c *Client) options() CerboOptions {
	return CerboOptions{c.tankInstance, c.pumpInstance, c.valveInstance, c.evInstance, c.evchargerInstance}
}
func (c *Client) initCerboMaps() {
	if c.cerboLeaves == nil {
		c.cerboLeaves = map[string]map[string]interface{}{}
	}
	if c.cerboOwned == nil {
		c.cerboOwned = map[string]bool{}
	}
	if c.state.TelemetryAvailable == nil {
		c.state.TelemetryAvailable = emptyAvailability()
	}
}
func clearStateField(st *state.State, key string) {
	v := reflect.ValueOf(st).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if strings.Split(t.Field(i).Tag.Get("json"), ",")[0] == key {
			field := v.Field(i)
			switch field.Kind() {
			case reflect.Map:
				field.Set(reflect.MakeMap(field.Type()))
			case reflect.Slice:
				field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			default:
				field.SetZero()
			}
			return
		}
	}
}
func (c *Client) applyCerboOverlays() {
	c.initCerboMaps()
	out := cerboOverlay(c.cerboLeaves, c.options())
	for key := range c.cerboOwned {
		if _, ok := out[key]; !ok {
			clearStateField(c.state, key)
			c.state.TelemetryAvailable[key] = false
		}
	}
	for key := range out {
		// JSON unmarshalling merges existing maps; direct collections are complete
		// snapshots, so clear them first to remove renamed or deleted devices.
		clearStateField(c.state, key)
		c.cerboOwned[key] = true
		c.state.TelemetryAvailable[key] = true
	}
	raw, _ := json.Marshal(out)
	_ = json.Unmarshal(raw, c.state)
}
func (c *Client) mergeDaemonState(data map[string]interface{}) {
	c.initCerboMaps()
	clean := map[string]interface{}{}
	for key, value := range data {
		if !c.cerboOwned[key] && key != "telemetry_available" {
			clean[key] = value
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return
	}
	if err = json.Unmarshal(raw, c.state); err != nil {
		log.Printf("Invalid inverter/state: %v", err)
		return
	}
	for _, key := range directFields {
		if v, ok := clean[key]; ok && v != nil {
			c.state.TelemetryAvailable[key] = true
		}
	}
	c.state.DashboardVersion = version.GetCurrent()
	c.applyCerboOverlays()
}

// CerboSnapshotToState maps a complete gateway snapshot with the exact LAN
// reducer. The service maps use keys such as "0/Ac/Grid/L1/Power".
func CerboSnapshotToState(all map[string]map[string]interface{}, o CerboOptions) *state.State {
	c := &Client{state: &state.State{}, cerboLeaves: all, tankInstance: o.TankInstance, pumpInstance: o.PumpInstance, valveInstance: o.ValveInstance, evInstance: o.EVInstance, evchargerInstance: o.EVChargerInstance}
	c.applyCerboOverlays()
	return c.state
}

func supportedKind(kind string) bool {
	switch kind {
	case "system", "grid", "battery", "solarcharger", "pvinverter", "vebus", "acload", "tank", "pump", "ev", "evcharger":
		return true
	}
	return false
}

// handleCerboDevice requires stateMu and does not perform network I/O.
func (c *Client) handleCerboDevice(topic string, payload []byte) bool {
	parts := strings.Split(topic, "/")
	if len(parts) < 4 || parts[0] != "N" || !supportedKind(parts[2]) {
		return false
	}
	if c.portalID != "" && parts[1] != c.portalID {
		return false
	}
	if _, err := strconv.Atoi(parts[3]); err != nil {
		return false
	}
	c.initCerboMaps()
	kind, instance := parts[2], parts[3]
	if len(payload) == 0 { // dbus-flashmq emits empty payloads when a service disappears.
		for path := range c.cerboLeaves[kind] {
			if strings.HasPrefix(path, instance+"/") {
				delete(c.cerboLeaves[kind], path)
			}
		}
		c.applyCerboOverlays()
		return true
	}
	if len(parts) < 5 {
		return false
	}
	value, ok := parseCerboPayload(payload)
	if !ok {
		return false
	}
	path := strings.Join(parts[4:], "/")
	// Numeric paths accept JSON numbers only. Metadata accepts strings; a null
	// explicitly invalidates either. Booleans/numeric strings are not readings.
	if value != nil {
		switch value.(type) {
		case string:
			if !strings.HasSuffix(path, "Name") && path != "Serial" && !strings.HasSuffix(path, "CellId") && path != "Dc/Battery/BatteryService" {
				return false
			}
		default:
			if _, ok := number(value); !ok {
				return false
			}
		}
	}
	if c.cerboLeaves[kind] == nil {
		c.cerboLeaves[kind] = map[string]interface{}{}
	}
	if value == nil {
		// An initial null is still a direct report of unavailability. Discover
		// its dependent output fields with a temporary numeric sample, then
		// discard that sample before reducing; stale daemon data cannot win.
		c.cerboLeaves[kind][instance+"/"+path] = 0.0
		for key := range cerboOverlay(c.cerboLeaves, c.options()) {
			c.cerboOwned[key] = true
		}
	}
	c.cerboLeaves[kind][instance+"/"+path] = value
	c.applyCerboOverlays()
	return true
}
func (c *Client) handleACLoad(topic string, payload []byte) bool {
	return c.handleCerboDevice(topic, payload)
}

func validPortal(portal string) bool {
	if portal == "" {
		return false
	}
	for _, ch := range portal {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
			return false
		}
	}
	return true
}
func (c *Client) acceptPortal(portal string) bool {
	if !validPortal(portal) {
		return false
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.portalID != "" {
		return c.portalID == portal
	}
	c.portalID = portal
	return true
}
func (c *Client) onCerboLiveMessage(_ mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) < 3 || parts[0] != "N" {
		return
	}
	value, valid := parseCerboPayload(msg.Payload())
	if !valid && len(msg.Payload()) != 0 {
		return
	}
	previous := c.PortalID()
	if previous == "" {
		if !valid || value == nil {
			return
		}
		if _, ok := value.(bool); ok {
			return
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			return
		}
	}
	if !c.acceptPortal(parts[1]) {
		return
	}
	if previous == "" {
		log.Printf("Discovered Cerbo portal %s from native MQTT", parts[1])
		go c.refreshPortal(parts[1])
	}
	if len(parts) == 3 {
		return
	} // heartbeat/keepalive discovery has no measurement leaf.
	if parts[2] == "platform" && strings.Contains(msg.Topic(), "/Notifications/") {
		c.onPlatformNotificationMessage(nil, msg)
		return
	}
	if strings.Contains(msg.Topic(), "/Alarms/") {
		c.onAlarmMessage(nil, msg)
		return
	}
	c.stateMu.Lock()
	changed := c.handleCerboDevice(msg.Topic(), msg.Payload())
	c.stateMu.Unlock()
	if changed {
		c.lastStateMu.Lock()
		c.lastStateTime = time.Now()
		c.lastStateMu.Unlock()
		c.triggerHandler()
	}
}
func (c *Client) onPortalMessage(_ mqtt.Client, msg mqtt.Message) {
	portal := strings.TrimSpace(string(msg.Payload()))
	var quoted string
	if json.Unmarshal(msg.Payload(), &quoted) == nil {
		portal = quoted
	}
	previous := c.PortalID()
	if previous != "" || !c.acceptPortal(portal) {
		return
	}
	go c.refreshPortal(portal)
}
func (c *Client) refreshPortal(portal string) {
	if c.client == nil || !c.client.IsConnectionOpen() {
		return
	}
	c.subscribeMu.Lock()
	defer c.subscribeMu.Unlock()
	c.keepaliveMu.Lock()
	c.keepaliveNeedsRefresh = true
	c.keepaliveMu.Unlock()
	if err := c.subscribeNativeTopics(portal); err != nil {
		log.Printf("Cerbo subscribe: %v", err)
		return
	}
	c.publishKeepalive(false)
}
func nativeFilters(portal string) []string {
	out := []string{}
	for _, kind := range []string{"system", "grid", "battery", "solarcharger", "pvinverter", "vebus", "acload", "tank", "pump", "ev", "evcharger", "platform"} {
		out = append(out, fmt.Sprintf("N/%s/%s/+/#", portal, kind))
	}
	out = append(out, fmt.Sprintf("N/%s/+/Alarms/#", portal), fmt.Sprintf("N/%s/heartbeat", portal), fmt.Sprintf("N/%s/keepalive", portal))
	return out
}
func (c *Client) subscribeNativeTopics(portal string) error {
	for _, filter := range nativeFilters(portal) {
		if token := c.client.Subscribe(filter, 0, c.onCerboLiveMessage); token.Wait() && token.Error() != nil {
			return fmt.Errorf("subscribe %s: %w", filter, token.Error())
		}
	}
	return nil
}
func (c *Client) publishKeepalive(suppress bool) {
	portal := c.PortalID()
	if portal == "" || c.client == nil || !c.client.IsConnectionOpen() {
		return
	}
	c.keepaliveMu.Lock()
	full := !suppress || c.keepaliveNeedsRefresh
	c.keepaliveMu.Unlock()
	body := ""
	if !full {
		body = `{"keepalive-options":["suppress-republish"]}`
	}
	if token := c.client.Publish("R/"+portal+"/keepalive", 0, false, body); token.Wait() && token.Error() != nil {
		log.Printf("Cerbo keepalive: %v", token.Error())
		c.keepaliveMu.Lock()
		if full {
			c.keepaliveNeedsRefresh = true
		}
		c.keepaliveMu.Unlock()
		return
	}
	if full {
		c.keepaliveMu.Lock()
		c.keepaliveNeedsRefresh = false
		c.keepaliveMu.Unlock()
	}
}
func (c *Client) startKeepalive() {
	c.stopKeepalive()
	stop := make(chan struct{})
	c.keepaliveMu.Lock()
	c.keepaliveStop = stop
	c.keepaliveNeedsRefresh = true
	c.keepaliveMu.Unlock()
	c.publishKeepalive(false)
	go func() {
		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				c.publishKeepalive(true)
			}
		}
	}()
}
func (c *Client) stopKeepalive() {
	c.keepaliveMu.Lock()
	defer c.keepaliveMu.Unlock()
	if c.keepaliveStop != nil {
		close(c.keepaliveStop)
		c.keepaliveStop = nil
	}
}
func (c *Client) PortalID() string { c.stateMu.RLock(); defer c.stateMu.RUnlock(); return c.portalID }

// invalidateCerbo makes a connection loss visible immediately, including legacy
// physical readings that arrived before direct telemetry. Controller state stays.
func (c *Client) invalidateCerbo() {
	c.stateMu.Lock()
	c.initCerboMaps()
	c.cerboLeaves = nil
	for _, key := range directFields {
		clearStateField(c.state, key)
		c.state.TelemetryAvailable[key] = false
	}
	c.stateMu.Unlock()
	c.lastStateMu.Lock()
	c.lastStateTime = time.Time{}
	c.lastStateMu.Unlock()
	c.triggerHandler()
}
