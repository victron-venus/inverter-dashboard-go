package mqtt

import (
	"testing"
)

func TestCameraEventJSONPayload(t *testing.T) {
	c := newTestClient()
	c.onCameraMessage(nil, &fakeMessage{topic: "frigate/front/events", payload: []byte(
		`{"agent_name":"Front Door","video_url":"http://f/clip.mp4","timestamp":"2026-08-24"}`)})

	ev := c.state.CameraEvent
	if ev == nil {
		t.Fatal("expected camera event")
	}
	if ev.Camera != "Front Door" || ev.URL != "http://f/clip.mp4" || ev.Ts != "2026-08-24" {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestCameraEventRawURLFallback(t *testing.T) {
	c := newTestClient()
	c.onCameraMessage(nil, &fakeMessage{topic: "cam", payload: []byte("http://frigate/api/snapshot.jpg")})
	if c.state.CameraEvent == nil {
		t.Fatal("expected camera event")
	}
	if c.state.CameraEvent.URL != "http://frigate/api/snapshot.jpg" || c.state.CameraEvent.Camera != "Camera" {
		t.Errorf("unexpected raw fallback: %+v", c.state.CameraEvent)
	}
}

func TestCameraEventBadJSONIsRaw(t *testing.T) {
	c := newTestClient()
	c.onCameraMessage(nil, &fakeMessage{topic: "cam", payload: []byte("{not json")})
	if c.state.CameraEvent == nil || c.state.CameraEvent.URL != "{not json" {
		t.Errorf("bad JSON must fall back to raw URL: %+v", c.state.CameraEvent)
	}
}

func TestSetCameraTopicSubscribePath(t *testing.T) {
	c := newTestClient()
	if c.cameraTopic != "" {
		t.Error("camera topic must default to empty (disabled)")
	}
	c.SetCameraTopic("frigate/+/events")
	if c.cameraTopic != "frigate/+/events" {
		t.Errorf("setter failed: %q", c.cameraTopic)
	}
}
