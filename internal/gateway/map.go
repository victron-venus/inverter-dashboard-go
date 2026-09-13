package gateway

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

// platformSlot holds one Venus GUIv2 Notifications/<slot> field set.
type platformSlot struct {
	inst         int
	slot         int
	description  string
	deviceName   string
	service      string
	dateTime     int64
	notifType    int64
	hasType      bool
	acknowledged bool
	active       *bool
}

func parsePlatformSlots(m leafMap) (slots []platformSlot, seen bool) {
	byKey := map[string]*platformSlot{}
	for k, raw := range m {
		// k: "<inst>/Notifications/<slot>/<Field>"
		parts := strings.Split(k, "/")
		if len(parts) < 4 || parts[1] != "Notifications" {
			continue
		}
		seen = true
		inst, err1 := strconv.Atoi(parts[0])
		slot, err2 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil {
			continue
		}
		field := parts[3]
		key := fmt.Sprintf("%d-%d", inst, slot)
		ps, ok := byKey[key]
		if !ok {
			ps = &platformSlot{inst: inst, slot: slot}
			byKey[key] = ps
		}
		switch field {
		case "Description":
			if str, ok := raw.(string); ok {
				ps.description = strings.TrimSpace(str)
			}
		case "DeviceName":
			if str, ok := raw.(string); ok {
				ps.deviceName = strings.TrimSpace(str)
			}
		case "Service":
			if str, ok := raw.(string); ok {
				ps.service = strings.TrimSpace(str)
			}
		case "DateTime":
			if n, ok := num(raw); ok {
				ps.dateTime = int64(n)
			}
		case "Type":
			if n, ok := num(raw); ok {
				ps.notifType = int64(n)
				ps.hasType = true
			}
		case "Acknowledged":
			if n, ok := num(raw); ok {
				ps.acknowledged = n != 0
			} else if b, ok := raw.(bool); ok {
				ps.acknowledged = b
			}
		case "Active":
			if n, ok := num(raw); ok {
				v := n != 0
				ps.active = &v
			} else if b, ok := raw.(bool); ok {
				ps.active = &b
			}
		}
	}
	for _, ps := range byKey {
		slots = append(slots, *ps)
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].inst != slots[j].inst {
			return slots[i].inst < slots[j].inst
		}
		return slots[i].slot < slots[j].slot
	})
	return slots, seen
}

func (ps platformSlot) toNotification() (state.Notification, bool) {
	if ps.acknowledged {
		return state.Notification{}, false
	}
	desc := strings.TrimSpace(ps.description)
	if desc == "" {
		return state.Notification{}, false
	}
	// Prefer Active=true when known; if Active absent still show (desktop parity).
	if ps.active != nil && !*ps.active {
		return state.Notification{}, false
	}
	level := "alarm"
	if ps.hasType {
		switch ps.notifType {
		case 0:
			level = "warning"
		case 2:
			level = "info"
		default:
			level = "alarm"
		}
	}
	body := ps.deviceName
	if body == "" {
		body = ps.service
	}
	ts := ""
	if ps.dateTime > 0 {
		ts = time.Unix(ps.dateTime, 0).UTC().Format(time.RFC3339)
	}
	return state.Notification{
		ID:     fmt.Sprintf("victron-platform-%d-%d", ps.inst, ps.slot),
		Level:  level,
		Title:  desc,
		Body:   body,
		Source: "victron",
		Ts:     ts,
	}, true
}

func prettyAlarmName(name string) string {
	var b strings.Builder
	current := ""
	flush := func() {
		if current == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.ToUpper(current[:1]) + current[1:])
		current = ""
	}
	for i, ch := range name {
		switch {
		case ch >= 'A' && ch <= 'Z' && current != "" && current[len(current)-1] >= 'a' && current[len(current)-1] <= 'z':
			flush()
			current = string(ch)
		case ch == '_' || ch == '-' || ch == ' ':
			flush()
		default:
			current += string(ch)
			_ = i
		}
	}
	flush()
	return b.String()
}

func prettyServiceName(service string) string {
	name, inst, found := strings.Cut(service, "_")
	if found {
		if _, err := strconv.Atoi(inst); err == nil {
			if name == "" {
				return service
			}
			return strings.ToUpper(name[:1]) + name[1:] + " " + inst
		}
	}
	if service == "" {
		return service
	}
	return strings.ToUpper(service[:1]) + service[1:]
}

func mapAlarmLeaves(service string, m leafMap, out *[]state.Notification) {
	for k, raw := range m {
		// "<inst>/Alarms/<Name>"
		parts := strings.Split(k, "/")
		if len(parts) < 3 || parts[1] != "Alarms" {
			continue
		}
		n, ok := num(raw)
		if !ok {
			continue
		}
		v := int(n)
		if v != 1 && v != 2 {
			continue
		}
		level := "warning"
		stateTxt := "Warning"
		if v == 2 {
			level = "alarm"
			stateTxt = "Alarm"
		}
		inst := parts[0]
		name := parts[2]
		svc := service
		if inst != "" {
			svc = service + "_" + inst
		}
		*out = append(*out, state.Notification{
			ID:     "victron-" + service + "/" + k,
			Level:  level,
			Title:  prettyServiceName(svc),
			Body:   fmt.Sprintf("%s: %s", prettyAlarmName(name), stateTxt),
			Source: "victron",
		})
	}
}

func mapNotifications(leaves snapshotLeaves) []state.Notification {
	// Always non-nil so ApplyState can clear banners when alarms resolve.
	out := make([]state.Notification, 0)
	slots, platformSeen := parsePlatformSlots(leaves.Platform)
	if platformSeen {
		for _, ps := range slots {
			if n, ok := ps.toNotification(); ok {
				out = append(out, n)
			}
		}
		return out
	}
	// Fallback: raw Victron Alarms/* (suppressed once platform Notifications appear).
	mapAlarmLeaves("battery", leaves.Battery, &out)
	mapAlarmLeaves("vebus", leaves.Vebus, &out)
	return out
}

// SnapshotToState maps an IGW snapshot onto dashboard state.State
// (parity with inverter-desktop gateway.rs::snapshot_to_state).
func SnapshotToState(snap *Snapshot, opt MapOptions) *state.State {
	opt = opt.withDefaults()
	leaves := snap.decoded()
	st := mqtt.CerboSnapshotToState(map[string]map[string]interface{}{
		"system": leaves.System, "grid": leaves.Grid,
		"battery": leaves.Battery, "solarcharger": leaves.Solarcharger,
		"pvinverter": leaves.Pvinverter, "vebus": leaves.Vebus,
		"acload": leaves.ACLoad, "tank": leaves.Tank, "pump": leaves.Pump,
		"ev": leaves.EV, "evcharger": leaves.EVCharger,
		"settings": leaves.Settings,
	}, mqtt.CerboOptions{
		TankInstance: opt.TankInstance, PumpInstance: opt.PumpInstance,
		ValveInstance: opt.ValveInstance, EVInstance: opt.EVInstance,
		EVChargerInstance: opt.EVChargerInstance,
	})

	st.Notifications = mapNotifications(leaves)

	return st
}
