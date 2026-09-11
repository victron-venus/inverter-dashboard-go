package gateway

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/victron-venus/inverter-dashboard-go/internal/mqtt"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// MapOptions selects Cerbo instances for water/EV (same defaults as MQTT path).
type MapOptions struct {
	TankInstance      int
	PumpInstance      int
	ValveInstance     int
	EVInstance        int
	EVChargerInstance int
}

func (o MapOptions) withDefaults() MapOptions {
	if o.TankInstance == 0 {
		o.TankInstance = 21
	}
	if o.PumpInstance == 0 {
		o.PumpInstance = 1
	}
	if o.ValveInstance == 0 {
		o.ValveInstance = 2
	}
	if o.EVInstance == 0 {
		o.EVInstance = 22
	}
	if o.EVChargerInstance == 0 {
		o.EVChargerInstance = 40
	}
	return o
}

func num(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func pathNum(m leafMap, path string) (float64, bool) {
	v, ok := m[path]
	if !ok || v == nil {
		return 0, false
	}
	return num(v)
}

func pathStr(m leafMap, path string) string {
	v, ok := m[path]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func instanceIDs(m leafMap) []int {
	seen := map[int]struct{}{}
	var out []int
	for k := range m {
		head, _, _ := strings.Cut(k, "/")
		id, err := strconv.Atoi(head)
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func inverterStateName(code int) string {
	if name, ok := map[int]string{
		0: "Off", 1: "Low Power", 2: "Fault", 3: "Bulk", 4: "Absorption",
		5: "Float", 6: "Storage", 7: "Equalize", 8: "Passthru", 9: "Inverting",
		10: "Power assist", 11: "Power supply", 252: "External control",
	}[code]; ok {
		return name
	}
	return fmt.Sprintf("? (%d)", code)
}

func formatTimeToGo(secs float64) string {
	s := uint64(secs)
	if s == 0 || s >= 86400*14 {
		return ""
	}
	h := s / 3600
	m := (s % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
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

// SnapshotToState maps an IGW snapshot onto dashboard state.State
// (parity with inverter-desktop gateway.rs::snapshot_to_state).
func SnapshotToState(snap *Snapshot, opt MapOptions) *state.State {
	opt = opt.withDefaults()
	leaves := snap.decoded()
	st := &state.State{
		Booleans: make(map[string]interface{}),
		Features: make(map[string]interface{}),
	}

	g1, hasG1 := pathNum(leaves.System, "0/Ac/Grid/L1/Power")
	g2, hasG2 := pathNum(leaves.System, "0/Ac/Grid/L2/Power")
	if hasG1 {
		st.G1 = g1
	}
	if hasG2 {
		st.G2 = g2
	}
	if hasG1 || hasG2 {
		st.GT = 0
		if hasG1 {
			st.GT += g1
		}
		if hasG2 {
			st.GT += g2
		}
	}

	t1, hasT1 := pathNum(leaves.System, "0/Ac/Consumption/L1/Power")
	t2, hasT2 := pathNum(leaves.System, "0/Ac/Consumption/L2/Power")
	if hasT1 {
		st.T1 = t1
	}
	if hasT2 {
		st.T2 = t2
	}
	if hasT1 || hasT2 {
		st.TT = 0
		if hasT1 {
			st.TT += t1
		}
		if hasT2 {
			st.TT += t2
		}
	}

	var haveSetpoint bool
	for _, inst := range instanceIDs(leaves.Vebus) {
		if !haveSetpoint {
			if sp, ok := pathNum(leaves.Vebus, fmt.Sprintf("%d/Hub4/L1/AcPowerSetpoint", inst)); ok {
				st.Setpoint = sp
				haveSetpoint = true
			}
		}
		if st.InverterState == "" {
			if code, ok := pathNum(leaves.Vebus, fmt.Sprintf("%d/State", inst)); ok {
				st.InverterState = inverterStateName(int(code))
			}
		}
	}

	var mpptTotal float64
	var hasMPPT bool
	for _, inst := range instanceIDs(leaves.Solarcharger) {
		power, hasP := pathNum(leaves.Solarcharger, fmt.Sprintf("%d/Yield/Power", inst))
		if !hasP {
			power, hasP = pathNum(leaves.Solarcharger, fmt.Sprintf("%d/Dc/0/Power", inst))
		}
		current, hasI := pathNum(leaves.Solarcharger, fmt.Sprintf("%d/Dc/0/Current", inst))
		pvV, _ := pathNum(leaves.Solarcharger, fmt.Sprintf("%d/Pv/V", inst))
		name := pathStr(leaves.Solarcharger, fmt.Sprintf("%d/CustomName", inst))
		if name == "" {
			name = pathStr(leaves.Solarcharger, fmt.Sprintf("%d/ProductName", inst))
		}
		if !hasP && !hasI && name == "" {
			continue
		}
		hasMPPT = true
		if hasP {
			mpptTotal += power
			st.MPPTIndividual = append(st.MPPTIndividual, power)
		}
		st.MPPTChargers = append(st.MPPTChargers, state.Charger{
			Name: name, PVVoltage: pvV, Current: current, Power: power,
		})
	}
	if hasMPPT {
		st.MpptTotal = mpptTotal
	}

	var pvTotal float64
	var hasPV bool
	for _, inst := range instanceIDs(leaves.Pvinverter) {
		power, ok := pathNum(leaves.Pvinverter, fmt.Sprintf("%d/Ac/Power", inst))
		if !ok {
			power, ok = pathNum(leaves.Pvinverter, fmt.Sprintf("%d/Ac/L1/Power", inst))
		}
		if !ok {
			continue
		}
		hasPV = true
		pvTotal += power
		name := pathStr(leaves.Pvinverter, fmt.Sprintf("%d/CustomName", inst))
		if name == "" {
			name = pathStr(leaves.Pvinverter, fmt.Sprintf("%d/ProductName", inst))
		}
		v, _ := pathNum(leaves.Pvinverter, fmt.Sprintf("%d/Ac/L1/Voltage", inst))
		i, _ := pathNum(leaves.Pvinverter, fmt.Sprintf("%d/Ac/L1/Current", inst))
		st.PvInverters = append(st.PvInverters, state.Charger{
			Name: name, PVVoltage: v, Current: i, Power: power,
		})
	}
	if hasPV {
		st.PVTotal = pvTotal
	}
	if hasMPPT || hasPV {
		st.SolarTotal = mpptTotal + pvTotal
	}

	var shunt *state.Battery
	for _, inst := range instanceIDs(leaves.Battery) {
		voltage, hasV := pathNum(leaves.Battery, fmt.Sprintf("%d/Dc/0/Voltage", inst))
		current, hasI := pathNum(leaves.Battery, fmt.Sprintf("%d/Dc/0/Current", inst))
		power, hasP := pathNum(leaves.Battery, fmt.Sprintf("%d/Dc/0/Power", inst))
		soc, _ := pathNum(leaves.Battery, fmt.Sprintf("%d/Soc", inst))
		name := pathStr(leaves.Battery, fmt.Sprintf("%d/CustomName", inst))
		if name == "" {
			name = pathStr(leaves.Battery, fmt.Sprintf("%d/ProductName", inst))
		}
		if !hasV && !hasP && !hasI && name == "" {
			continue
		}
		batState := ""
		if hasI {
			batState = stateFromCurrent(current)
		}
		ttg := ""
		if secs, ok := pathNum(leaves.Battery, fmt.Sprintf("%d/TimeToGo", inst)); ok {
			if batState == "Charging" || batState == "Discharging" {
				ttg = formatTimeToGo(secs)
			}
		}
		b := state.Battery{
			Name: name, Voltage: voltage, Current: current, Power: power,
			SOC: soc, State: batState, TimeToGo: ttg,
		}
		st.Batteries = append(st.Batteries, b)
		if shunt == nil && strings.Contains(strings.ToLower(name), "shunt") {
			cp := b
			shunt = &cp
		}
	}
	switch {
	case shunt != nil:
		st.BatteryVoltage = shunt.Voltage
		st.BV = shunt.Voltage
		st.BatteryCurrent = shunt.Current
		st.BC = shunt.Current
		st.BatteryPower = shunt.Power
		st.BP = shunt.Power
		st.BatterySOC = mqtt.VoltageSOC(shunt.Voltage)
	default:
		if v, ok := pathNum(leaves.System, "0/Dc/Battery/Voltage"); ok {
			st.BatteryVoltage = v
			st.BV = v
			st.BatterySOC = mqtt.VoltageSOC(v)
		}
		if i, ok := pathNum(leaves.System, "0/Dc/Battery/Current"); ok {
			st.BatteryCurrent = i
			st.BC = i
		}
		if p, ok := pathNum(leaves.System, "0/Dc/Battery/Power"); ok {
			st.BatteryPower = p
			st.BP = p
		}
	}

	if level, ok := pathNum(leaves.Tank, fmt.Sprintf("%d/Level", opt.TankInstance)); ok {
		if level <= 1.0 {
			level *= 100.0
		}
		st.WaterLevel = level
	} else {
		for _, inst := range instanceIDs(leaves.Tank) {
			if level, ok := pathNum(leaves.Tank, fmt.Sprintf("%d/Level", inst)); ok {
				if level <= 1.0 {
					level *= 100.0
				}
				st.WaterLevel = level
				break
			}
		}
	}

	if n, ok := pathNum(leaves.Pump, fmt.Sprintf("%d/State", opt.PumpInstance)); ok {
		st.PumpSwitch = n != 0
	}
	if n, ok := pathNum(leaves.Pump, fmt.Sprintf("%d/State", opt.ValveInstance)); ok {
		st.WaterValve = n != 0
	}

	loads := make(map[string]float64)
	loadNames := make(map[string]string)
	for _, inst := range instanceIDs(leaves.ACLoad) {
		power, ok := pathNum(leaves.ACLoad, fmt.Sprintf("%d/Ac/Power", inst))
		if !ok {
			power, ok = pathNum(leaves.ACLoad, fmt.Sprintf("%d/Ac/L1/Power", inst))
		}
		if !ok {
			continue
		}
		name := pathStr(leaves.ACLoad, fmt.Sprintf("%d/CustomName", inst))
		if name == "" {
			name = pathStr(leaves.ACLoad, fmt.Sprintf("%d/ProductName", inst))
		}
		if name == "" {
			name = fmt.Sprintf("AC Load %d", inst)
		}
		key := name
		if strings.HasPrefix(name, "AC Load ") {
			key = strings.ToLower(strings.ReplaceAll(name, " ", "_"))
		}
		if _, exists := loads[key]; exists {
			key = fmt.Sprintf("%s_%d", key, inst)
		}
		loads[key] = power
		loadNames[strconv.Itoa(inst)] = name
	}
	if len(loads) > 0 {
		st.Loads = loads
		st.LoadNames = loadNames
	}

	if p, ok := pathNum(leaves.EV, fmt.Sprintf("%d/Ac/Power", opt.EVInstance)); ok {
		st.EVChargingKW = p / 1000.0
	} else {
		for _, inst := range instanceIDs(leaves.EV) {
			if p, ok := pathNum(leaves.EV, fmt.Sprintf("%d/Ac/Power", inst)); ok {
				st.EVChargingKW = p / 1000.0
				break
			}
		}
	}
	if soc, ok := pathNum(leaves.EV, fmt.Sprintf("%d/Soc", opt.EVInstance)); ok {
		st.CarSOC = soc
	}
	if p, ok := pathNum(leaves.EVCharger, fmt.Sprintf("%d/Ac/Power", opt.EVChargerInstance)); ok {
		st.EVPower = p / 1000.0
	} else {
		for _, inst := range instanceIDs(leaves.EVCharger) {
			if p, ok := pathNum(leaves.EVCharger, fmt.Sprintf("%d/Ac/Power", inst)); ok {
				st.EVPower = p / 1000.0
				break
			}
		}
	}

	return st
}
