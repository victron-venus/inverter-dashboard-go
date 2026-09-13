package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// maxNotifications caps the ring buffer of notifications kept for clients.
const maxNotifications = 100

// onNotificationMessage handles inverter/notifications payloads
// ({"id","level","title","body","source","ts"}, the desktop reference's
// MqttNotification shape) and appends them to the shared state so clients
// receive them on the next broadcast.
func (c *Client) onNotificationMessage(_ mqtt.Client, msg mqtt.Message) {
	var raw struct {
		ID     string `json:"id"`
		Level  string `json:"level"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Source string `json:"source"`
		Ts     string `json:"ts"`
	}
	if err := json.Unmarshal(msg.Payload(), &raw); err != nil {
		log.Printf("Bad notification payload: %v", err)
		return
	}
	if raw.Source == "" {
		raw.Source = "inverter-control"
	}
	if raw.Level == "" {
		raw.Level = "info"
	}

	c.stateMu.Lock()
	st := c.state
	st.Notifications = append(st.Notifications, state.Notification{
		ID:     raw.ID,
		Level:  raw.Level,
		Title:  raw.Title,
		Body:   raw.Body,
		Source: raw.Source,
		Ts:     raw.Ts,
	})
	if len(st.Notifications) > maxNotifications {
		st.Notifications = st.Notifications[len(st.Notifications)-maxNotifications:]
	}
	c.stateMu.Unlock()
	c.triggerHandler()
}

// onAlarmMessage tracks a Victron alarm topic
// (N/<portal>/<service>/Alarms/<Name>, value 0=ok / 1=warning / 2=alarm)
// and emits/clears banner notifications on transitions.
func (c *Client) onAlarmMessage(client mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()

	c.platformMu.Lock()
	platformSeen := c.platformNotifsSeen
	c.platformMu.Unlock()
	if platformSeen {
		// Desktop: once platform Notifications appear, suppress raw Alarms banners.
		return
	}

	var payload struct {
		Value interface{} `json:"value"`
	}
	if len(msg.Payload()) > 0 {
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			return
		}
	}
	value := 0 // Null or removed alarm paths clear their previous banner.
	if payload.Value != nil {
		num, ok := toFloat(payload.Value)
		if !ok {
			return
		}
		value = int(num)
	}

	if _, changed := c.setAlarmValue(topic, value); !changed {
		return
	}

	id := "victron-" + topic
	parts := strings.Split(topic, "/")
	service := "device"
	name := topic
	if len(parts) > 4 {
		service = parts[2]
		name = parts[len(parts)-1]
		if len(parts) > 5 && parts[4] == "Alarms" {
			service += "_" + parts[3]
		}
	}

	c.stateMu.Lock()
	st := c.state
	if value == 1 || value == 2 {
		level := "warning"
		stateTxt := "Warning"
		if value == 2 {
			level = "alarm"
			stateTxt = "Alarm"
		}
		st.Notifications = append(st.Notifications, state.Notification{
			ID:     id,
			Level:  level,
			Title:  prettyServiceName(service),
			Body:   fmt.Sprintf("%s: %s", prettyAlarmName(name), stateTxt),
			Source: "victron",
		})
		if len(st.Notifications) > maxNotifications {
			st.Notifications = st.Notifications[len(st.Notifications)-maxNotifications:]
		}
	} else {
		kept := st.Notifications[:0]
		for _, n := range st.Notifications {
			if n.ID != id {
				kept = append(kept, n)
			}
		}
		st.Notifications = kept
	}
	c.stateMu.Unlock()
	c.triggerHandler()
}

// setAlarmValue records the latest alarm value for a topic and reports
// whether it differs from the previously seen one (transition detection).
func (c *Client) setAlarmValue(topic string, value int) (prev int, changed bool) {
	c.alarmsMu.Lock()
	defer c.alarmsMu.Unlock()
	if c.alarmValues == nil {
		c.alarmValues = make(map[string]int)
	}
	prev = c.alarmValues[topic]
	if prev != value {
		c.alarmValues[topic] = value
	}
	return prev, prev != value
}

// prettyAlarmName splits CamelCase/underscore alarm names into words:
// "HighCellVoltage" / "high_cell_voltage" -> "High Cell Voltage".
func prettyAlarmName(name string) string {
	var b strings.Builder
	for i, w := range splitCamel(name) {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(capitalize(w))
	}
	return b.String()
}

// prettyServiceName turns "battery_512" into "Battery 512", "vebus" -> "Vebus".
func prettyServiceName(service string) string {
	name, inst, found := strings.Cut(service, "_")
	if found && isAllDigits(inst) {
		return capitalize(name) + " " + inst
	}
	return capitalize(service)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func splitCamel(s string) []string {
	var words []string
	current := ""
	for _, ch := range s {
		switch {
		case ch >= 'A' && ch <= 'Z' && current != "" && lastIsLower(current):
			words = append(words, current)
			current = string(ch)
		case ch == '_' || ch == '-' || ch == ' ':
			if current != "" {
				words = append(words, current)
				current = ""
			}
		default:
			current += string(ch)
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

func lastIsLower(s string) bool {
	last := s[len(s)-1]
	return last >= 'a' && last <= 'z'
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// platformSlotState mirrors Venus GUIv2 Notifications/<slot> fields.
type platformSlotState struct {
	inst         uint32
	slot         uint32
	description  string
	deviceName   string
	service      string
	dateTime     int64
	notifType    int64
	hasType      bool
	active       *bool
	acknowledged bool
}

func (ps *platformSlotState) bannerID() string {
	return fmt.Sprintf("victron-platform-%d-%d", ps.inst, ps.slot)
}

func (ps *platformSlotState) toNotification() (state.Notification, bool) {
	if ps.acknowledged {
		return state.Notification{}, false
	}
	desc := strings.TrimSpace(ps.description)
	if desc == "" {
		return state.Notification{}, false
	}
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
		ID:     ps.bannerID(),
		Level:  level,
		Title:  desc,
		Body:   body,
		Source: "victron",
		Ts:     ts,
	}, true
}

// onPlatformNotificationMessage handles
// N/<portal>/platform/<inst>/Notifications/<slot>/<Field>.
func (c *Client) onPlatformNotificationMessage(_ mqtt.Client, msg mqtt.Message) {
	parts := strings.Split(msg.Topic(), "/")
	// N / portal / platform / inst / Notifications / slot / Field
	if len(parts) < 7 || parts[2] != "platform" || parts[4] != "Notifications" {
		return
	}
	inst64, err1 := strconv.ParseUint(parts[3], 10, 32)
	slot64, err2 := strconv.ParseUint(parts[5], 10, 32)
	if err1 != nil || err2 != nil {
		return
	}
	field := parts[6]
	var payload struct {
		Value interface{} `json:"value"`
	}
	if len(msg.Payload()) > 0 {
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			return
		}
	}

	key := fmt.Sprintf("%d-%d", inst64, slot64)
	c.platformMu.Lock()
	c.platformNotifsSeen = true
	if c.platformSlots == nil {
		c.platformSlots = make(map[string]*platformSlotState)
	}
	ps, ok := c.platformSlots[key]
	if !ok {
		ps = &platformSlotState{inst: uint32(inst64), slot: uint32(slot64)}
		c.platformSlots[key] = ps
	}
	if len(msg.Payload()) == 0 {
		delete(c.platformSlots, key)
	} else if payload.Value == nil {
		switch field {
		case "Description":
			ps.description = ""
		case "DeviceName":
			ps.deviceName = ""
		case "Service":
			ps.service = ""
		case "DateTime":
			ps.dateTime = 0
		case "Type":
			ps.hasType = false
		case "Acknowledged":
			ps.acknowledged = false
		case "Active":
			ps.active = nil
		}
	}
	switch field {
	case "Description":
		if str, ok := payload.Value.(string); ok {
			ps.description = strings.TrimSpace(str)
		}
	case "DeviceName":
		if str, ok := payload.Value.(string); ok {
			ps.deviceName = strings.TrimSpace(str)
		}
	case "Service":
		if str, ok := payload.Value.(string); ok {
			ps.service = strings.TrimSpace(str)
		}
	case "DateTime":
		if n, ok := toFloat(payload.Value); ok {
			ps.dateTime = int64(n)
		}
	case "Type":
		if n, ok := toFloat(payload.Value); ok {
			ps.notifType = int64(n)
			ps.hasType = true
		}
	case "Acknowledged":
		if n, ok := toFloat(payload.Value); ok {
			ps.acknowledged = n != 0
		} else if b, ok := payload.Value.(bool); ok {
			ps.acknowledged = b
		}
	case "Active":
		if n, ok := toFloat(payload.Value); ok {
			v := n != 0
			ps.active = &v
		} else if b, ok := payload.Value.(bool); ok {
			ps.active = &b
		}
	}
	snapshot := make([]*platformSlotState, 0, len(c.platformSlots))
	for _, v := range c.platformSlots {
		cp := *v
		snapshot = append(snapshot, &cp)
	}
	c.platformMu.Unlock()

	// Rebuild banners: keep inverter-control pushes; replace all victron-* with platform slots.
	c.stateMu.Lock()
	st := c.state
	kept := st.Notifications[:0]
	for _, n := range st.Notifications {
		if !strings.HasPrefix(n.ID, "victron-") {
			kept = append(kept, n)
		}
	}
	for _, ps := range snapshot {
		if n, ok := ps.toNotification(); ok {
			kept = append(kept, n)
		}
	}
	if len(kept) > maxNotifications {
		kept = kept[len(kept)-maxNotifications:]
	}
	st.Notifications = kept
	c.stateMu.Unlock()
	c.triggerHandler()
}
