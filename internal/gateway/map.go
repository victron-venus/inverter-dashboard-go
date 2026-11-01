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
	TankInstance             int
	PumpInstance             int
	ValveInstance            int
	EVInstance               int
	EVChargerInstance        int
	EVInstancesConfigured    bool
	WaterInstancesConfigured bool
}

func (o MapOptions) withDefaults() MapOptions {
	if !o.WaterInstancesConfigured {
		if o.TankInstance == 0 {
			o.TankInstance = 21
		}
		if o.PumpInstance == 0 {
			o.PumpInstance = 1
		}
		if o.ValveInstance == 0 {
			o.ValveInstance = 2
		}
	}
	if !o.EVInstancesConfigured {
		if o.EVInstance == 0 {
			o.EVInstance = -1
		}
		if o.EVChargerInstance == 0 {
			o.EVChargerInstance = -1
		}
	}
	return o
}

// platformSlot holds one Venus GUIv2 Notifications/<slot> field set.
type platformSlot struct {
	inst          uint32
	slot          uint32
	description   string
	deviceName    string
	service       string
	dateTime      string
	notifType     int64
	hasType       bool
	pushTypeKnown bool
	acknowledged  bool
	active        *bool
}

func parsePlatformSlots(m leafMap) (slots []platformSlot, seen bool) {
	byKey := map[string]*platformSlot{}
	for k, raw := range m {
		// k: "<inst>/Notifications/<slot>/<Field>"
		inst, slot, field, valid := state.ParsePlatformNotificationKey(k)
		if !valid || !state.ValidPlatformNotificationValue(field, raw) {
			continue
		}
		seen = true
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
			ps.dateTime = state.NotificationTime(raw)
		case "Type":
			if level, ok := state.NativeNotificationLevel(raw); ok {
				_, text := raw.(string)
				ps.pushTypeKnown = !text
				ps.notifType = int64(level)
				ps.hasType = true
			}
		case "Acknowledged":
			if b, ok := state.NativeNotificationBool(raw); ok {
				ps.acknowledged = b
			}
		case "Active":
			if b, ok := state.NativeNotificationBool(raw); ok {
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
	// GUIv2 keeps inactive but unacknowledged warnings. Active describes the
	// physical condition; only acknowledgement removes its pending banner.
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
	return state.Notification{
		ID:             fmt.Sprintf("victron-platform-%d-%d", ps.inst, ps.slot),
		Level:          level,
		Title:          desc,
		Body:           body,
		Source:         "victron",
		Ts:             ps.dateTime,
		PushIncomplete: !ps.pushTypeKnown,
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

// nativeAlarmPath preserves nested DBus paths such as Alarms/L1/Overload.
// Mapping and exact-number decoding must accept the same alarm leaves.
func nativeAlarmPath(key string) (instance, name string, valid bool) {
	parts := strings.Split(key, "/")
	if len(parts) < 3 || parts[1] != "Alarms" {
		return "", "", false
	}
	for _, part := range parts {
		if part == "" {
			return "", "", false
		}
	}
	return parts[0], parts[len(parts)-1], true
}

func mapAlarmLeaves(service string, m leafMap, out *[]state.Notification) {
	for k, raw := range m {
		inst, name, valid := nativeAlarmPath(k)
		if !valid {
			continue
		}
		v, ok := state.NativeNotificationLevel(raw)
		if !ok || (v != 1 && v != 2) {
			continue
		}
		level := "warning"
		stateTxt := "Warning"
		if v == 2 {
			level = "alarm"
			stateTxt = "Alarm"
		}
		svc := service + "_" + inst
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

	st.PushSamples = mqtt.NativePushSamples(map[string]map[string]interface{}{
		"system": leaves.System, "battery": leaves.Battery, "pump": leaves.Pump, "evcharger": leaves.EVCharger,
	}, mqtt.CerboOptions{PumpInstance: opt.PumpInstance, ValveInstance: opt.ValveInstance, EVChargerInstance: opt.EVChargerInstance}, "")
	st.GatewayCapabilities = snap.Capabilities
	st.Notifications = mapNotifications(leaves)
	if snap.InverterPresent || snap.Inverter != nil {
		mqtt.ApplyControllerSnapshot(st, snap.Inverter)
		at := float64(time.Now().UnixMilli()) / 1000
		if snap.Capabilities["setpoint_override"] {
			st.SetpointOverride = state.ParseOverrideStatus(snap.Inverter["setpoint_override"])
			if st.SetpointOverride != nil {
				st.SetpointOverrideObservedAt = &at
			}
		}
		if snap.Capabilities["electricity_tariff"] {
			st.ElectricityTariffObservedAt = &at
		}
		if st.GridBackup != nil {
			st.GridBackupObservedAt = st.GridBackup.MeasurementTime
		}
		if _, ok := snap.Inverter["ess_mode"].(map[string]interface{}); ok {
			at := float64(time.Now().UnixMilli()) / 1000
			st.ESSModeObservedAt = &at
		}
	}

	return st
}
