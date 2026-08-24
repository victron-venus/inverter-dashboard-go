package mqtt

import (
	"encoding/json"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// onCameraMessage handles the optional camera topic (desktop CameraEvent
// contract): JSON {agent_name, video_url, timestamp}, or a raw string
// payload treated as a direct stream/snapshot URL.
func (c *Client) onCameraMessage(_ mqtt.Client, msg mqtt.Message) {
	payload := strings.TrimSpace(string(msg.Payload()))
	ev := &state.CameraEvent{Camera: "Camera"}

	var parsed struct {
		AgentName string `json:"agent_name"`
		VideoURL  string `json:"video_url"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err == nil &&
		(parsed.AgentName != "" || parsed.VideoURL != "") {
		if parsed.AgentName != "" {
			ev.Camera = parsed.AgentName
		}
		ev.URL = parsed.VideoURL
		ev.Ts = parsed.Timestamp
	} else {
		ev.URL = payload
	}

	c.stateMu.Lock()
	c.state.CameraEvent = ev
	c.stateMu.Unlock()
	c.triggerHandler()
}
