package mqtt

import (
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

	var payload struct {
		Value interface{} `json:"value"`
	}
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		return
	}
	num, ok := toFloat(payload.Value)
	if !ok {
		return
	}
	value := int(num)

	if _, changed := c.setAlarmValue(topic, value); !changed {
		return
	}

	id := "victron-" + topic
	parts := strings.Split(topic, "/")
	service := "device"
	name := topic
	if len(parts) > 4 {
		service = parts[2]
		name = parts[4]
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
