package mqtt

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

func send(c *Client, kind, path string, value interface{}) {
	c.onCerboLiveMessage(nil, cerboMsg("N/p1/"+kind+"/"+path, value))
}
func TestDirectDiscoveryIsolationAndFreshness(t *testing.T) {
	c := NewClient("localhost", 1883)
	if c.GetState().TelemetryAvailable["gt"] {
		t.Fatal("undiscovered grid must be unavailable")
	}
	send(c, "system/0", "Serial", "p1")
	if c.PortalID() != "p1" {
		t.Fatal("native discovery requires inverter-control")
	}
	send(c, "system/0", "Ac/Grid/L1/Power", 42)
	if c.LastStateTime().IsZero() {
		t.Fatal("native readings did not update freshness")
	}
	c.onCerboLiveMessage(nil, cerboMsg("N/foreign/system/0/Ac/Grid/L1/Power", 999))
	c.onPortalMessage(nil, &fakeMessage{topic: "inverter/portal", payload: []byte("foreign")})
	if c.GetState().GT != 42 || c.PortalID() != "p1" {
		t.Fatal("foreign portal contaminated selected system")
	}
}

func TestFieldOwnershipZerosAndNull(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	c.SetEVConfig(22, 40)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"gt": 150.0, "battery_soc": 80.0, "mppt_total": 500.0, "setpoint": 100.0})
	c.stateMu.Unlock()
	send(c, "vebus/276", "ProductName", "MultiPlus")
	send(c, "battery/512", "ProductName", "Battery")
	if c.GetState().Setpoint != 100 || c.GetState().BatterySOC != 80 || c.GetState().GT != 150 {
		t.Fatal("device metadata stole unrelated fields")
	}
	send(c, "system/0", "Ac/Grid/L1/Power", 0)
	send(c, "ev/22", "Ac/Power", 0)
	send(c, "pump/2", "State", 0)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"gt": 999.0, "ev_power": 999.0, "water_valve": true})
	c.stateMu.Unlock()
	st := c.GetState()
	if st.GT != 0 || st.EVPower != 0 || st.WaterValve || !st.TelemetryAvailable["gt"] || !st.TelemetryAvailable["ev_power"] {
		t.Fatalf("valid zero lost: %+v", st)
	}
	raw, _ := json.Marshal(st)
	for _, field := range []string{`"gt":0`, `"ev_power":0`, `"water_valve":false`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("zero omitted: %s", field)
		}
	}
	send(c, "system/0", "Ac/Grid/L1/Power", nil)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"gt": 999.0})
	c.stateMu.Unlock()
	if c.GetState().GT != 0 || c.GetState().TelemetryAvailable["gt"] {
		t.Fatal("null retained or resurrected stale grid")
	}
}

func TestThreePhasePowerPrecedenceAndInvalidation(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "vebus/276", "Ac/Out/P", 9000)       // Output must not be mistaken for grid import.
	send(c, "system/0", "Ac/ActiveIn/Source", 2) // Generator must not be mistaken for mains.
	send(c, "vebus/276", "Ac/ActiveIn/L1/P", 700)
	if c.GetState().TelemetryAvailable["gt"] {
		t.Fatal("generator/output treated as grid")
	}
	send(c, "grid/30", "Ac/L1/Power", 10)
	send(c, "grid/30", "Ac/L2/Power", 20)
	send(c, "grid/30", "Ac/L3/Power", 30)
	send(c, "system/0", "Ac/Grid/L1/Power", 100)
	send(c, "system/0", "Ac/Consumption/L1/Power", 200)
	send(c, "system/0", "Ac/Consumption/L2/Power", 300)
	send(c, "system/0", "Ac/Consumption/L3/Power", 400)
	st := c.GetState()
	if st.GT != 150 || st.G3 != 30 || st.TT != 900 || st.T3 != 400 {
		t.Fatalf("phase totals: %+v", st)
	}
	send(c, "system/0", "Ac/Grid/L1/Power", nil)
	if c.GetState().GT != 60 {
		t.Fatal("null did not reveal lower priority direct meter")
	}
}

func TestSystemBatteryPriorityAndNoVoltageEstimate(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "battery/512", "CustomName", "SmartShunt")
	send(c, "battery/512", "Dc/0/Voltage", 47.2)
	if c.GetState().TelemetryAvailable["battery_soc"] || c.GetState().Batteries[0].TelemetryAvailable["soc"] {
		t.Fatal("voltage fabricated SOC")
	}
	send(c, "battery/512", "Soc", 63)
	send(c, "battery/512", "Dc/0/Current", -2)
	if c.GetState().BatteryPower != -94.4 {
		t.Fatal("V*I power fallback missing")
	}
	send(c, "system/0", "Dc/Battery/Soc", 0)
	send(c, "system/0", "Dc/Battery/Voltage", 51)
	if c.GetState().BatterySOC != 0 || c.GetState().BatteryVoltage != 51 {
		t.Fatal("selected system battery did not win")
	}
	send(c, "battery/513", "Dc/0/Voltage", 48)
	send(c, "system/0", "Dc/Battery/Voltage", nil)
	if c.GetState().TelemetryAvailable["battery_voltage"] {
		t.Fatal("ambiguous batteries silently selected")
	}
	send(c, "system/0", "Dc/Battery/Instance", 512)
	if c.GetState().BatteryVoltage != 47.2 {
		t.Fatal("selected battery instance not used")
	}
}

func TestPVAndACLoadTotalsNeverDoubleCount(t *testing.T) {
	c := NewClient("localhost", 1883)
	for _, kind := range []string{"pvinverter/10", "acload/80"} {
		send(c, kind, "Ac/L1/Power", 100)
		send(c, kind, "Ac/L2/Power", 200)
		send(c, kind, "Ac/L3/Power", 300)
		send(c, kind, "Ac/Power", 600)
		send(c, kind, "Ac/L1/Power", 110)
	}
	st := c.GetState()
	if st.PVInverterTotal != 600 || st.Loads["ac_load_80"] != 600 {
		t.Fatal("L1 overwrote total or phases double counted")
	}
	send(c, "pvinverter/10", "Ac/Power", nil)
	if c.GetState().PVInverterTotal != 610 {
		t.Fatal("phase fallback missing")
	}
	send(c, "solarcharger/1", "Dc/0/Voltage", 50)
	send(c, "solarcharger/1", "Dc/0/Current", 10)
	if st = c.GetState(); st.MpptTotal != 500 || st.PVTotal != 500 || st.SolarTotal != 1110 {
		t.Fatalf("solar components: %+v", st)
	}
	send(c, "acload/80", "CustomName", "Oven")
	if st = c.GetState(); len(st.Loads) != 1 || st.Loads["Oven"] != 600 {
		t.Fatalf("rename left stale map keys: %v", st.Loads)
	}
	c.onCerboLiveMessage(nil, &fakeMessage{topic: "N/p1/acload/80/Ac/Power", payload: nil})
	if len(c.GetState().Loads) != 0 || c.GetState().TelemetryAvailable["loads"] {
		t.Fatal("service removal retained load")
	}
}

func TestEVWaterUnitsAndMalformedReadings(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	c.SetEVConfig(22, 40)
	send(c, "ev/22", "Ac/Power", 3200)
	send(c, "evcharger/40", "Ac/Power", 7200)
	send(c, "tank/21", "Level", .91)
	send(c, "pump/1", "Mode", 2)
	if st := c.GetState(); st.EVPower != 3200 || st.EVChargingKW != 7.2 || st.WaterLevel != .91 || st.PumpMode != 2 {
		t.Fatalf("wrong units/config: %+v", st)
	}
	for _, value := range []interface{}{true, "100", "NaN"} {
		send(c, "ev/22", "Ac/Power", value)
	}
	c.onCerboLiveMessage(nil, &fakeMessage{topic: "N/p1/ev/22/Ac/Power", payload: []byte(`{"other":99}`)})
	if c.GetState().EVPower != 3200 {
		t.Fatal("malformed telemetry clobbered last valid sample")
	}
}

func TestSolarDoesNotMixStaleControllerComponents(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"mppt_total": 900.0, "pv_total": 900.0, "solar_total": 1500.0})
	c.stateMu.Unlock()
	send(c, "pvinverter/10", "Ac/Power", 200)
	if c.GetState().SolarTotal != 200 {
		t.Fatal("direct total mixed stale daemon components")
	}
}

type doneToken struct{}

func (doneToken) Wait() bool                     { return true }
func (doneToken) WaitTimeout(time.Duration) bool { return true }
func (doneToken) Done() <-chan struct{}          { ch := make(chan struct{}); close(ch); return ch }
func (doneToken) Error() error                   { return nil }

type recordingBroker struct {
	paho.Client
	mu       sync.Mutex
	filters  []string
	writes   []string
	handlers map[string]paho.MessageHandler
}

func (b *recordingBroker) IsConnected() bool      { return true }
func (b *recordingBroker) IsConnectionOpen() bool { return true }
func (b *recordingBroker) Subscribe(topic string, qos byte, h paho.MessageHandler) paho.Token {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.filters = append(b.filters, topic)
	if b.handlers == nil {
		b.handlers = map[string]paho.MessageHandler{}
	}
	b.handlers[topic] = h
	return doneToken{}
}
func (b *recordingBroker) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes = append(b.writes, topic+"="+payload.(string))
	return doneToken{}
}
func TestSubscriptionsBootstrapAndReconnect(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	b := &recordingBroker{}
	c.client = b
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	defer c.stopKeepalive()
	if len(b.writes) != 1 || b.writes[0] != "R/p1/keepalive=" {
		t.Fatalf("snapshot bootstrap missing: %v", b.writes)
	}
	for _, filter := range b.filters {
		if strings.HasPrefix(filter, "N/+") {
			t.Fatalf("configured portal uses unrestricted subscription %q", filter)
		}
	}
	send(c, "system/0", "Ac/Grid/L1/Power", 100)
	n := len(b.filters)
	if err := c.Subscribe(); err != nil {
		t.Fatal(err)
	}
	if len(b.filters) != 2*n || c.GetState().TelemetryAvailable["gt"] {
		t.Fatal("resubscribe failed to reset stale direct state")
	}
	c.publishKeepalive(true)
	if got := b.writes[len(b.writes)-1]; got != `R/p1/keepalive={"keepalive-options":["suppress-republish"]}` {
		t.Fatalf("periodic keepalive: %s", got)
	}
}

func TestNativeAlarmPathIncludesServiceInstanceAndAlarmName(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "battery/512", "Alarms/HighCellVoltage", 2)
	n := c.GetState().Notifications
	if len(n) != 1 || n[0].Title != "Battery 512" || n[0].Body != "High Cell Voltage: Alarm" {
		t.Fatalf("native alarm mislabeled: %+v", n)
	}
}

func TestSystemSolarWinsOverPartialDeviceInventory(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "system/0", "Dc/Pv/Power", 1200)
	send(c, "system/0", "Ac/PvOnGrid/L1/Power", 900)
	send(c, "solarcharger/1", "Yield/Power", 300)
	send(c, "pvinverter/10", "Ac/Power", 200)
	if st := c.GetState(); st.MpptTotal != 1200 || st.PVInverterTotal != 900 || st.SolarTotal != 2100 {
		t.Fatalf("partial device totals replaced system aggregates: %+v", st)
	}
}

func TestInitialNullDoesNotReviveControllerReading(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	send(c, "system/0", "Ac/Grid/L1/Power", nil)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"g1": 900.0, "gt": 900.0})
	c.stateMu.Unlock()
	if c.GetState().GT != 0 || c.GetState().TelemetryAvailable["gt"] {
		t.Fatal("initial native null was replaced by old daemon telemetry")
	}
}

func TestHeartbeatDiscoveryRejectsUnavailablePayloads(t *testing.T) {
	c := NewClient("localhost", 1883)
	for _, value := range []interface{}{nil, true, ""} {
		c.onCerboLiveMessage(nil, cerboMsg("N/invalid/heartbeat", value))
	}
	if c.PortalID() != "" {
		t.Fatal("invalid native sample selected portal")
	}
	c.onCerboLiveMessage(nil, cerboMsg("N/p1/heartbeat", 1234))
	if c.PortalID() != "p1" {
		t.Fatal("native heartbeat failed to discover portal")
	}
}

func TestDisconnectInvalidatesTelemetryImmediately(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "system/0", "Ac/Grid/L1/Power", 100)
	c.invalidateCerbo()
	st := c.GetState()
	if st.TelemetryAvailable["gt"] || st.GT != 0 || !c.LastStateTime().IsZero() {
		t.Fatal("disconnected telemetry remained fresh")
	}
	raw, _ := json.Marshal(st)
	if !strings.Contains(string(raw), `"batteries":[]`) || !strings.Contains(string(raw), `"loads":{}`) {
		t.Fatal("disconnect did not emit collection resets")
	}
}

type failedToken struct{ doneToken }

func (failedToken) Error() error { return errors.New("temporary publish failure") }

type retryBroker struct {
	recordingBroker
	failed bool
}

func (b *retryBroker) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	b.recordingBroker.Publish(topic, qos, retained, payload)
	if !b.failed {
		b.failed = true
		return failedToken{}
	}
	return doneToken{}
}
func TestKeepaliveRetriesFullSnapshotBeforeSuppressing(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	b := &retryBroker{}
	c.client = b
	c.publishKeepalive(false)
	c.publishKeepalive(true)
	c.publishKeepalive(true)
	if len(b.writes) != 3 || b.writes[0] != "R/p1/keepalive=" || b.writes[1] != "R/p1/keepalive=" || b.writes[2] != `R/p1/keepalive={"keepalive-options":["suppress-republish"]}` {
		t.Fatalf("failed bootstrap was never retried: %v", b.writes)
	}
}

func TestLastBroadcastSurvivesConcurrentUpdate(t *testing.T) {
	c := NewClient("localhost", 1883)
	first, release, second := make(chan struct{}), make(chan struct{}), make(chan struct{})
	calls := 0
	c.SetMessageHandler(func() {
		calls++
		if calls == 1 {
			close(first)
			<-release
		} else {
			close(second)
		}
	})
	c.triggerHandler()
	<-first
	c.triggerHandler()
	close(release)
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("last update was dropped by coalescer")
	}
}

func TestDisconnectedVebusInputCannotSupplyGrid(t *testing.T) {
	c := NewClient("localhost", 1883)
	send(c, "system/0", "Ac/ActiveIn/Source", 1)
	send(c, "vebus/276", "Ac/ActiveIn/Connected", 0)
	send(c, "vebus/276", "Ac/ActiveIn/L1/P", 800)
	if c.GetState().TelemetryAvailable["gt"] {
		t.Fatal("disconnected VE.Bus input supplied stale grid")
	}
}

func TestSlimControllerMetadataSurvivesDirectTelemetry(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{
		"daily_stats":        map[string]interface{}{"produced_yesterday": 12.5, "pv_inverter_daily": []float64{1, 2}, "pv_inverter_yesterday": []float64{3, 4}, "mppt_yesterday": []float64{5}},
		"limits":             map[string]interface{}{"min": -1000, "max": 2000},
		"ui_config":          map[string]interface{}{"loads_title": "House"},
		"dvcc_limits":        map[string]interface{}{"charge_current": 40},
		"grid_control_valid": false, "grid_control_reason": "Waiting for grid", "grid_loss_state": "holding", "grid_loss_zero_applied": false,
	})
	c.stateMu.Unlock()
	send(c, "system/0", "Ac/Grid/L1/Power", 40)
	st := c.GetState()
	if st.DailyStats.ProducedYesterday != 12.5 || len(st.DailyStats.PVInverterDaily) != 2 || len(st.DailyStats.MpptYesterday) != 1 || st.Limits["max"] != float64(2000) || st.UIConfig["loads_title"] != "House" || st.GridControlValid == nil || *st.GridControlValid || st.GridLossState != "holding" {
		t.Fatalf("slim controller metadata dropped: %+v", st)
	}
}

func TestGatewayTelemetryPreservesControllerState(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.stateMu.Lock()
	c.mergeDaemonState(map[string]interface{}{"booleans": map[string]interface{}{"no_feed": true}, "dry_run": true, "ess_mode": map[string]interface{}{"mode_name": "External control"}, "daily_stats": map[string]interface{}{"produced_today": 12.5}, "ui_config": map[string]interface{}{"title": "Home"}})
	c.stateMu.Unlock()
	snapshot := CerboSnapshotToState(map[string]map[string]interface{}{"system": {"0/Ac/Grid/L1/Power": 25.0}}, CerboOptions{})
	c.ApplyState(snapshot)
	st := c.GetState()
	if st.Booleans["no_feed"] != true || !st.DryRun || st.ESSMode.ModeName != "External control" || st.DailyStats.ProducedToday != 12.5 || st.UIConfig["title"] != "Home" || st.GT != 25 {
		t.Fatalf("gateway snapshot erased controller metadata: %+v", st)
	}
}

// Paho IsConnected remains true while auto-reconnect is scheduled. That must
// never advertise a live connection or enqueue immediate device commands.
type reconnectingBroker struct{ recordingBroker }

func (b *reconnectingBroker) IsConnectionOpen() bool { return false }

func TestReconnectIntentIsNotLiveTelemetryOrControl(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 3, 8)
	send(c, "pump/3", "Mode", 0)
	broker := &reconnectingBroker{}
	c.client = broker
	if c.IsConnected() || c.CanControlWater() {
		t.Fatal("reconnecting MQTT advertised live telemetry/control")
	}
	if err := c.SetWaterMode("pump", 1); err == nil {
		t.Fatal("water command was accepted while reconnecting")
	}
	if err := c.PublishCommand("toggle", nil); err == nil {
		t.Fatal("immediate controller command was accepted while reconnecting")
	}
	if err := c.Subscribe(); err == nil {
		t.Fatal("subscription attempted without an active broker socket")
	}
	c.publishKeepalive(false)
	if len(broker.writes) != 0 || len(broker.filters) != 0 {
		t.Fatalf("reconnecting client queued network operations: writes=%v filters=%v", broker.writes, broker.filters)
	}
	c.EnableGatewayMode()
	c.SetGatewayConnected(true)
	if !c.IsConnected() || c.CanControlWater() {
		t.Fatal("gateway health/control isolation regressed")
	}
}

func TestGatewayConnectionLossBroadcastsWithoutNewSnapshot(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.EnableGatewayMode()
	c.SetGatewayConnected(true)
	changes := make(chan bool, 2)
	c.SetMessageHandler(func() { changes <- c.IsConnected() })
	c.SetGatewayConnected(false)
	select {
	case connected := <-changes:
		if connected {
			t.Fatal("gateway failure broadcast stale connection state")
		}
	case <-time.After(time.Second):
		t.Fatal("failed gateway poll did not broadcast without ApplyState")
	}
	c.SetGatewayConnected(true)
	select {
	case connected := <-changes:
		if !connected {
			t.Fatal("gateway recovery broadcast stale connection state")
		}
	case <-time.After(time.Second):
		t.Fatal("gateway recovery did not broadcast")
	}
}
