package mqtt

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/push"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func TestNativePushSamplesUseActualSelectedLeaves(t *testing.T) {
	all := map[string]map[string]interface{}{
		"system":  {"0/Dc/Battery/Soc": 19., "0/Dc/Battery/Instance": float64(2), "0/Dc/Battery/Voltage": 48.},
		"battery": {"1/Soc": 10., "2/Soc": 25.}, "pump": {"1/State": 0., "2/State": 1.},
		"ev": {"4/Ac/Power": 4000.}, "evcharger": {"5/Ac/Power": 2000., "6/Ac/Power": 5000.},
		"grid": {"0/Ac/Power": 0.},
	}
	opt := CerboOptions{PumpInstance: 1, ValveInstance: 2, EVChargerInstance: 5}
	samples := NativePushSamples(all, opt, "")
	if len(samples) != 4 {
		t.Fatal(samples)
	}
	for _, s := range samples {
		switch s.Kind {
		case "soc":
			if s.ID != "system/0" || s.Value == nil || *s.Value != 19 {
				t.Fatal(s)
			}
		case "ev":
			if s.ID != "evcharger/5" || s.Value == nil || *s.Value != 2000 {
				t.Fatal(s)
			}
		}
	}
	if got := NativePushSamples(all, opt, "system/0/Dc/Battery/Voltage"); len(got) != 0 {
		t.Fatal("cached synthetic measurements refreshed", got)
	}
	if got := NativePushSamples(all, opt, "ev/4/Ac/Power"); len(got) != 0 {
		t.Fatal("driving power used", got)
	}
	delete(all["system"], "0/Dc/Battery/Soc")
	samples = NativePushSamples(all, opt, "")
	for _, s := range samples {
		if s.Kind == "soc" && (s.ID != "battery/2" || s.Value == nil || *s.Value != 25) {
			t.Fatal("selected native battery ignored", s)
		}
	}
	delete(all["system"], "0/Dc/Battery/Instance")
	for _, s := range NativePushSamples(all, opt, "") {
		if s.Kind == "soc" {
			t.Fatal("arbitrary multi-battery fallback")
		}
	}
	delete(all["battery"], "1/Soc")
	hasSOC := false
	for _, s := range NativePushSamples(all, opt, "") {
		if s.Kind == "soc" {
			hasSOC = true
		}
	}
	if !hasSOC {
		t.Fatal("sole battery omitted")
	}
}
func TestNativePushSamplesUnknownRetainedAndDisconnectAreExplicit(t *testing.T) {
	opt := CerboOptions{PumpInstance: 1, ValveInstance: 2, EVChargerInstance: 5}
	all := map[string]map[string]interface{}{"pump": {"1/State": nil}, "evcharger": {"5/Ac/Power": 20., "5/Connected": 0.}}
	samples := NativePushSamples(all, opt, "pump/1/State")
	if len(samples) != 1 || samples[0].Value != nil {
		t.Fatal("unknown pump state")
	}
	samples = NativePushSamples(all, opt, "evcharger/5/Connected")
	if len(samples) == 0 {
		t.Fatal("disconnect not observed")
	}
	for _, s := range samples {
		if s.Value != nil {
			t.Fatal("disconnect refreshed value")
		}
	}
}
func TestPushServiceObservesWithoutWebsocketAndRejectsRetiredGeneration(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "push")
	service := push.NewService(push.Config{Enabled: true, DataDir: dir})
	defer service.Close()
	if !service.Available() {
		t.Skip("platform without persistence")
	}
	c := NewClient("", 0)
	c.SetPushService(service)
	c.resetPush("gateway", true)
	generation := c.pushGenerationNow()
	c.observePush("gateway", generation, push.Observation{Notifications: []push.Native{}})
	now := time.Now()
	c.observePush("gateway", generation, push.Observation{Notifications: []push.Native{{ID: "fresh", Source: "victron", Level: "warning", Title: "Alarm", Timestamp: now}}})
	readDedupe := func() int {
		raw, err := os.ReadFile(filepath.Join(dir, "state.json"))
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Dedupe []string `json:"dedupe"`
		}
		if json.Unmarshal(raw, &data) != nil {
			t.Fatal("state parse")
		}
		return len(data.Dedupe)
	}
	if readDedupe() != 1 {
		t.Fatal("no-client processing skipped")
	}
	c.resetPush("mqtt", true)
	c.observePush("gateway", generation, push.Observation{Notifications: []push.Native{{ID: "retired", Source: "victron", Level: "warning", Title: "Alarm", Timestamp: now}}})
	if readDedupe() != 1 {
		t.Fatal("retired callback accepted")
	}
	c.EnableGatewayMode()
	source := c.pushSource
	c.invalidateCerbo()
	if c.pushSource != source {
		t.Fatal("old MQTT connection retired active gateway push epoch")
	}
	c.SetGatewayConnected(true)
	c.ApplyState(&state.State{Notifications: []state.Notification{}, PushSamples: []state.PushSample{{Kind: "pump", ID: "pump/1"}}})
	if c.state.PushSamples != nil {
		t.Fatal("internal push observations leaked into cached state")
	}
}
func TestNativePushSeverityAndOriginalTimezone(t *testing.T) {
	result := nativePushNotifications([]state.Notification{{ID: "slot", Level: "alarm", Source: "victron", Ts: "2026-10-05T12:00:00-07:00"}, {ID: "unknown", Level: "warning"}})
	if result[0].Level != "error" || result[0].Timestamp.UTC().Format(time.RFC3339) != "2026-10-05T19:00:00Z" || !result[1].Timestamp.IsZero() {
		t.Fatal(result)
	}
}

func pushWireFixture(t *testing.T) (*Client, *push.Service, func() int) {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "push")
	service := push.NewService(push.Config{Enabled: true, DataDir: dir})
	t.Cleanup(service.Close)
	if !service.Available() {
		t.Skip("platform without persistence")
	}
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"subscription": map[string]any{"endpoint": "https://fcm.googleapis.com/test", "expirationTime": nil, "keys": map[string]string{"p256dh": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), "auth": base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}, "preferences": push.DefaultPreferences()})
	router := gin.New()
	push.RegisterRoutes(router, service)
	req := httptest.NewRequest("POST", "https://dashboard.example/api/notifications/subscription", strings.NewReader(string(body)))
	req.Header.Set("Origin", "https://dashboard.example")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	count := func() int {
		raw, err := os.ReadFile(filepath.Join(dir, "state.json"))
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Queue []json.RawMessage `json:"queue"`
		}
		if json.Unmarshal(raw, &data) != nil {
			t.Fatal("state parse")
		}
		return len(data.Queue)
	}
	c := NewClient("", 0)
	c.SetPushService(service)
	c.resetPush("mqtt", true)
	service.Reset("mqtt", true, time.Now().Add(-20*time.Second))

	return c, service, count
}
func TestPlatformPushWaitsForSourceTimeAndExplicitSeverity(t *testing.T) {
	c, _, count := pushWireFixture(t)
	for _, slot := range []struct{ id, level int }{{1, 2}, {2, 0}} {
		prefix := "N/test/platform/0/Notifications/" + string(rune('0'+slot.id)) + "/"
		c.onPlatformNotificationMessage(nil, waterMsg(prefix+"Description", "new event"))
		c.onPlatformNotificationMessage(nil, waterMsg(prefix+"DateTime", time.Now().Unix()))
		if count() != 0 {
			t.Fatal("missing source severity caused notification")
		}
		c.onPlatformNotificationMessage(nil, waterMsg(prefix+"Type", slot.level))
	}
	if count() != 1 {
		t.Fatal("fresh partial warning not delivered exactly once", count())
	}
}

func TestRejectedNumericSourceAndRetainedSamplesCannotRefreshBaseline(t *testing.T) {
	c, _, count := pushWireFixture(t)
	c.SetWaterConfig("test", 21, 1, 2)
	c.onCerboLiveMessage(nil, waterMsg("N/test/pump/1/State", 0))
	c.onCerboLiveMessage(nil, waterMsg("N/test/pump/1/State", "unavailable"))
	c.onCerboLiveMessage(nil, waterMsg("N/test/pump/1/State", 1))
	if count() != 0 {
		t.Fatal("rejected numeric input kept a cached baseline fresh")
	}
	c.onCerboLiveMessage(nil, &retainedESSMessage{waterMsg("N/test/pump/1/State", 1)})
	c.onCerboLiveMessage(nil, waterMsg("N/test/pump/1/State", 0))
	if count() != 0 {
		t.Fatal("retained input kept baseline")
	}
	c.onCerboLiveMessage(nil, waterMsg("N/test/pump/1/State", 1))
	if count() != 1 {
		t.Fatal("fresh transition missing")
	}
}
func TestRetiredNestedPlatformCallbackKeepsOriginalGeneration(t *testing.T) {
	c, service, count := pushWireFixture(t)
	captured := c.pushGenerationNow()
	c.resetPush("mqtt", true)
	service.Reset("mqtt", true, time.Now().Add(-20*time.Second))
	c.applyPlatformNotification(waterMsg("N/test/platform/0/Notifications/0/Description", "retired"), captured)
	if len(c.GetState().Notifications) != 0 || count() != 0 {
		t.Fatal("retired nested callback relabelled as current")
	}
}

func TestSubscribedCallbacksFromPriorSessionCannotCaptureNewEpoch(t *testing.T) {
	c, service, count := pushWireFixture(t)
	c.SetWaterConfig("test", 21, 1, 2)
	broker := &recordingBroker{}
	c.client = broker
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	defer c.stopKeepalive()
	oldNative := broker.handlers["N/test/pump/+/#"]
	oldPlatform := broker.handlers["N/test/platform/+/#"]
	oldController := broker.handlers["inverter/notifications"]
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	service.Reset("mqtt", true, time.Now().Add(-20*time.Second))
	oldNative(nil, waterMsg("N/test/pump/1/State", 0))
	oldNative(nil, waterMsg("N/test/pump/1/State", 1))
	oldPlatform(nil, waterMsg("N/test/platform/0/Notifications/0/Description", "retired"))
	body, _ := json.Marshal(map[string]any{"id": "retired-controller", "level": "warning", "title": "retired", "source": "inverter-control", "ts": time.Now().Format(time.RFC3339Nano)})
	oldController(nil, &fakeMessage{topic: "inverter/notifications", payload: body})
	if count() != 0 || len(c.GetState().Notifications) != 0 {
		t.Fatal("old subscription callback relabelled to new epoch")
	}
	current := broker.handlers["N/test/pump/+/#"]
	current(nil, waterMsg("N/test/pump/1/State", 0))
	current(nil, waterMsg("N/test/pump/1/State", 1))
	if count() != 1 {
		t.Fatal("current subscription did not process fresh event")
	}
}
func TestRecoveryProbeSessionWorksAfterGatewaySourceSwitch(t *testing.T) {
	c, _, count := pushWireFixture(t)
	c.SetWaterConfig("test", 21, 1, 2)
	c.client = &recordingBroker{}
	c.EnableGatewayMode()
	c.SetGatewayConnected(true)
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	defer c.stopKeepalive()
	current := c.client.(*recordingBroker).handlers["N/test/pump/+/#"]
	current(nil, waterMsg("N/test/pump/1/State", 0))
	current(nil, waterMsg("N/test/pump/1/State", 1))
	if count() != 0 {
		t.Fatal("recovery probe became authoritative early")
	}
	c.DisableGatewayMode()
	current(nil, waterMsg("N/test/pump/1/State", 0))
	if count() != 0 {
		t.Fatal("probe observation leaked across source epoch")
	}
	current(nil, waterMsg("N/test/pump/1/State", 1))
	if count() != 1 {
		t.Fatal("current probe subscription invalidated by source switch")
	}
}
