package mqtt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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
	c.applyNotificationMessage(msg, c.pushGenerationNow(), c.mqttSession.Load())
}
func (c *Client) applyNotificationMessage(msg mqtt.Message, generation, session uint64) {
	if generation != c.pushGenerationNow() {
		return
	}
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
	raw.ID, raw.Title = strings.TrimSpace(raw.ID), strings.TrimSpace(raw.Title)
	if raw.ID == "" && raw.Title == "" {
		return
	}
	if raw.ID == "" {
		raw.ID = raw.Title
	}
	if raw.Title == "" {
		raw.Title = raw.ID
	}
	if raw.Source == "" {
		raw.Source = "inverter-control"
	}
	raw.Level = strings.ToLower(strings.TrimSpace(raw.Level))
	if raw.Level != "info" && raw.Level != "warning" && raw.Level != "alarm" {
		raw.Level = "info"
	}

	// Decode can overlap a reconnect or source switch. Fence the state mutation,
	// not only push delivery, just like native platform notification callbacks.
	c.gatewayMu.RLock()
	defer c.gatewayMu.RUnlock()
	if c.gatewayMode || generation != c.pushGenerationNow() || session != c.mqttSession.Load() {
		return
	}
	c.stateMu.Lock()
	st := c.state
	st.Notifications = upsertNotification(st.Notifications, state.Notification{
		ID:     raw.ID,
		Level:  raw.Level,
		Title:  raw.Title,
		Body:   raw.Body,
		Source: raw.Source,
		Ts:     raw.Ts,
	})
	c.stateMu.Unlock()
	c.notificationPush(generation, msg.Retained())
	c.triggerHandler()
}

// A banner is current state for an event ID, not a log of replayed packets.
func upsertNotification(notifications []state.Notification, notification state.Notification) []state.Notification {
	kept := notifications[:0]
	for _, existing := range notifications {
		if existing.ID != notification.ID {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, notification)
	if len(kept) > maxNotifications {
		kept = kept[len(kept)-maxNotifications:]
	}
	return kept
}

// onAlarmMessage tracks a Victron alarm topic
// (N/<portal>/<service>/Alarms/<Name>, value 0=ok / 1=warning / 2=alarm)
// and emits/clears banner notifications on transitions.
func (c *Client) onAlarmMessage(client mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()

	c.platformMu.Lock()
	defer c.platformMu.Unlock()
	if c.platformNotifsSeen {
		// Desktop: once platform Notifications appear, suppress raw Alarms banners.
		return
	}

	value, valid := decodeAlarmValue(msg.Payload())
	if !valid {
		return
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
		st.Notifications = upsertNotification(st.Notifications, state.Notification{
			ID:     id,
			Level:  level,
			Title:  prettyServiceName(service),
			Body:   fmt.Sprintf("%s: %s", prettyAlarmName(name), stateTxt),
			Source: "victron",
		})
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

func decodeAlarmValue(raw []byte) (int, bool) {
	if len(raw) == 0 {
		return 0, true // A retained-message tombstone clears the alarm.
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(raw, &payload) != nil {
		return 0, false
	}
	value, present := payload["value"]
	if !present {
		return 0, false
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return 0, true
	}
	var level any
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if decoder.Decode(&level) != nil {
		return 0, false
	}
	return state.NativeNotificationLevel(level)
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
	inst          uint32
	slot          uint32
	description   string
	deviceName    string
	service       string
	dateTime      string
	notifType     int64
	hasType       bool
	pushTypeKnown bool
	active        *bool
	acknowledged  bool
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
	// Like Desktop/GUIv2, an inactive condition can still need acknowledgement.
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
	return state.Notification{
		ID:             ps.bannerID(),
		Level:          level,
		Title:          desc,
		Body:           body,
		Source:         "victron",
		Ts:             ps.dateTime,
		PushIncomplete: !ps.pushTypeKnown,
	}, true
}

// onPlatformNotificationMessage handles
// N/<portal>/platform/<inst>/Notifications/<slot>/<Field>.
func (c *Client) onPlatformNotificationMessage(_ mqtt.Client, msg mqtt.Message) {
	c.applyPlatformNotification(msg, c.pushGenerationNow(), c.mqttSession.Load())
}

type platformNotificationUpdate struct {
	inst, slot uint32
	field      string
	value      interface{}
}

func decodePlatformNotification(msg mqtt.Message) (platformNotificationUpdate, bool) {
	parts := strings.Split(msg.Topic(), "/")
	// N / portal / platform / inst / Notifications / slot / Field
	if len(parts) != 7 || parts[0] != "N" || !validPortal(parts[1]) || parts[2] != "platform" || parts[4] != "Notifications" {
		return platformNotificationUpdate{}, false
	}
	inst, slot, field, valid := state.ParsePlatformNotificationKey(strings.Join(parts[3:], "/"))
	if !valid {
		return platformNotificationUpdate{}, false
	}
	raw := msg.Payload()
	var value interface{}
	if len(raw) > 0 {
		var payload map[string]json.RawMessage
		if json.Unmarshal(raw, &payload) != nil {
			return platformNotificationUpdate{}, false
		}
		fieldValue, present := payload["value"]
		if !present {
			return platformNotificationUpdate{}, false
		}
		// Validate native dates, enums and flags before float64 can round a
		// malformed fraction into a valid timestamp, severity or acknowledgment.
		decoder := json.NewDecoder(bytes.NewReader(fieldValue))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return platformNotificationUpdate{}, false
		}
	}

	if !state.ValidPlatformNotificationValue(field, value) {
		return platformNotificationUpdate{}, false
	}
	return platformNotificationUpdate{inst: inst, slot: slot, field: field, value: value}, true
}

func (ps *platformSlotState) clearField(field string) {
	switch field {
	case "Description":
		ps.description = ""
	case "DeviceName":
		ps.deviceName = ""
	case "Service":
		ps.service = ""
	case "DateTime":
		ps.dateTime = ""
	case "Type":
		ps.hasType = false
		ps.pushTypeKnown = false
	case "Acknowledged":
		ps.acknowledged = false
	case "Active":
		ps.active = nil
	}
}

func (ps *platformSlotState) setType(value interface{}) {
	ps.pushTypeKnown = false
	if level, ok := state.NativeNotificationLevel(value); ok {
		_, text := value.(string)
		ps.pushTypeKnown = !text
		ps.notifType = int64(level)
		ps.hasType = true
	}
}

func (ps *platformSlotState) updateField(field string, value interface{}) {
	if value == nil {
		ps.clearField(field)
		return
	}
	switch field {
	case "Description":
		if str, ok := value.(string); ok {
			ps.description = strings.TrimSpace(str)
		}
	case "DeviceName":
		if str, ok := value.(string); ok {
			ps.deviceName = strings.TrimSpace(str)
		}
	case "Service":
		if str, ok := value.(string); ok {
			ps.service = strings.TrimSpace(str)
		}
	case "DateTime":
		ps.dateTime = state.NotificationTime(value)
	case "Type":
		ps.setType(value)
	case "Acknowledged":
		if b, ok := state.NativeNotificationBool(value); ok {
			ps.acknowledged = b
		}
	case "Active":
		if b, ok := state.NativeNotificationBool(value); ok {
			ps.active = &b
		}
	}
}

// updatePlatformSlots requires platformMu through the subsequent state commit.
func (c *Client) updatePlatformSlots(update platformNotificationUpdate) []*platformSlotState {
	key := fmt.Sprintf("%d-%d", update.inst, update.slot)
	c.platformNotifsSeen = true
	if c.platformSlots == nil {
		c.platformSlots = make(map[string]*platformSlotState)
	}
	ps, ok := c.platformSlots[key]
	if !ok {
		ps = &platformSlotState{inst: update.inst, slot: update.slot}
		c.platformSlots[key] = ps
	}
	// Empty retained payloads delete this leaf, never unrelated slot metadata.
	ps.updateField(update.field, update.value)
	snapshot := make([]*platformSlotState, 0, len(c.platformSlots))
	for _, v := range c.platformSlots {
		cp := *v
		snapshot = append(snapshot, &cp)
	}
	return snapshot
}

func (c *Client) applyPlatformNotification(msg mqtt.Message, generation, session uint64) {
	if generation != c.pushGenerationNow() {
		return
	}
	update, ok := decodePlatformNotification(msg)
	if !ok {
		return
	}
	c.gatewayMu.RLock()
	defer c.gatewayMu.RUnlock()
	if c.gatewayMode || generation != c.pushGenerationNow() || session != c.mqttSession.Load() {
		return
	}
	c.platformMu.Lock()
	defer c.platformMu.Unlock()
	snapshot := c.updatePlatformSlots(update)
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
	c.notificationPush(generation, msg.Retained())
	c.triggerHandler()
}

// resetNativeNotifications runs while gatewayMu excludes source callbacks.
// A recovery probe resets its own caches without clearing the active IGW view.
func (c *Client) resetNativeNotifications(clearBanners bool) {
	c.platformMu.Lock()
	defer c.platformMu.Unlock()
	c.platformSlots = nil
	c.platformNotifsSeen = false
	c.alarmsMu.Lock()
	c.alarmValues = nil
	c.alarmsMu.Unlock()
	if !clearBanners {
		return
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	kept := c.state.Notifications[:0]
	for _, notification := range c.state.Notifications {
		if notification.Source != "victron" && !strings.HasPrefix(notification.ID, "victron-") {
			kept = append(kept, notification)
		}
	}
	c.state.Notifications = kept
}
