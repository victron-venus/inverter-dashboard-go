package mqtt

import (
	"strconv"
	"strings"
	"time"

	"github.com/victron-venus/inverter-dashboard-go/internal/push"
	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

// Push handling has its own lifecycle and is never gated on WebSocket clients.
func (c *Client) SetPushService(service *push.Service) {
	c.pushMu.Lock()
	c.pushService = service
	c.pushMu.Unlock()
}
func (c *Client) resetPush(source string, connected bool) {
	c.pushMu.Lock()
	defer c.pushMu.Unlock()
	c.pushGeneration++
	c.pushSource = source
	if c.pushService != nil {
		c.pushService.Reset(source, connected, time.Now())
	}
}
func (c *Client) pushGenerationNow() uint64 {
	c.pushMu.Lock()
	defer c.pushMu.Unlock()
	return c.pushGeneration
}
func (c *Client) observePush(source string, generation uint64, ob push.Observation) {
	c.pushMu.Lock()
	defer c.pushMu.Unlock()
	if c.pushService != nil && c.pushSource == source && c.pushGeneration == generation {
		c.pushService.Observe(ob, time.Now())
	}
}
func nativePushNotifications(notifications []state.Notification) []push.Native {
	result := make([]push.Native, 0, len(notifications))
	for _, n := range notifications {
		timestamp, _ := time.Parse(time.RFC3339Nano, n.Ts)
		if n.PushIncomplete {
			timestamp = time.Time{}
		}
		if n.Level == "alarm" {
			n.Level = "error"
		}
		result = append(result, push.Native{ID: n.ID, Source: n.Source, Level: n.Level, Title: n.Title, Body: n.Body, Timestamp: timestamp})
	}
	return result
}
func (c *Client) notificationPush(generation uint64, retained bool) {
	c.stateMu.RLock()
	notifications := append([]state.Notification(nil), c.state.Notifications...)
	c.stateMu.RUnlock()
	c.observePush("mqtt", generation, push.Observation{Notifications: nativePushNotifications(notifications), Retained: retained})
}

// NativePushSamples reads only actual source leaves. For MQTT, changed identifies
// exactly the leaf received now; other cached leaves cannot refresh a baseline.
// Empty changed means one successful complete gateway snapshot.
func NativePushSamples(all map[string]map[string]interface{}, o CerboOptions, changed string) []state.PushSample {
	samples := []state.PushSample{}
	add := func(kind, id, leaf string, value interface{}, enabled bool, minimum, maximum float64) {
		if changed != "" && changed != id+"/"+leaf && changed != id+"/Connected" {
			return
		}
		sample := state.PushSample{Kind: kind, ID: id}
		// Connected metadata only invalidates; it is not a new measurement.
		if enabled && (changed == "" || changed == id+"/"+leaf) {
			if n, ok := number(value); ok && n >= minimum && n <= maximum {
				if kind != "pump" && kind != "valve" || n == 0 || n == 1 {
					sample.Value = &n
				}
			}
		}
		samples = append(samples, sample)
	}
	for _, entry := range []struct {
		kind     string
		instance int
	}{{"pump", o.PumpInstance}, {"valve", o.ValveInstance}} {
		id := "pump/" + strconv.Itoa(entry.instance)
		d := leaves{}
		for path, value := range all["pump"] {
			if strings.HasPrefix(path, strconv.Itoa(entry.instance)+"/") {
				d[strings.TrimPrefix(path, strconv.Itoa(entry.instance)+"/")] = value
			}
		}
		leaf := "State"
		if _, exists := d[leaf]; !exists {
			leaf = "Status"
		}
		add(entry.kind, id, leaf, d[leaf], d.enabled(), 0, 1)
	}
	chargers := devices(all, "evcharger")
	chargerID := strconv.Itoa(o.EVChargerInstance)
	if o.EVChargerInstance < 0 {
		chargerID = ""
		for _, id := range sortedStringKeys(chargers) {
			if _, ok := chargers[id].num("Ac/Power"); ok {
				chargerID = id
				break
			}
		}
	}
	if chargerID != "" {
		d := chargers[chargerID]
		add("ev", "evcharger/"+chargerID, "Ac/Power", d["Ac/Power"], d != nil, 0, 1e9)
	}
	// System SoC is authoritative when exposed. Otherwise use one native battery;
	// changing selected identity starts a separate baseline automatically.
	systems := devices(all, "system")
	systemSOC := false
	for _, id := range sortedStringKeys(systems) {
		if value, exists := systems[id]["Dc/Battery/Soc"]; exists {
			add("soc", "system/"+id, "Dc/Battery/Soc", value, true, 0, 100)
			systemSOC = true
			break
		}
	}
	if !systemSOC {
		batteries := devices(all, "battery")
		selected := ""
		for _, id := range sortedStringKeys(systems) {
			if n, ok := systems[id].num("Dc/Battery/Instance"); ok && n >= 0 && n == float64(int(n)) {
				selected = strconv.Itoa(int(n))
				break
			}
		}
		if selected == "" && len(batteries) == 1 {
			for id := range batteries {
				selected = id
			}
		}
		if selected == "" && (strings.HasPrefix(changed, "battery/") || strings.HasPrefix(changed, "system/")) {
			samples = append(samples, state.PushSample{Kind: "soc", ID: "battery/unknown"})
		}
		if selected != "" {
			d := batteries[selected]
			add("soc", "battery/"+selected, "Soc", d["Soc"], d != nil, 0, 100)
		}
	}
	if strings.HasSuffix(changed, "/Dc/Battery/Instance") {
		samples = append(samples, state.PushSample{Kind: "soc", ID: "battery/selection"})
	}
	// A disconnect cannot leave the previous selected-device baseline alive.
	if strings.HasSuffix(changed, "/Connected") {
		parts := strings.Split(changed, "/")
		if len(parts) == 3 && parts[0] == "evcharger" {
			samples = append(samples, state.PushSample{Kind: "ev", ID: parts[0] + "/" + parts[1]})
		}
		if len(parts) == 3 && (parts[0] == "battery" || parts[0] == "system") {
			samples = append(samples, state.PushSample{Kind: "soc", ID: parts[0] + "/" + parts[1]})
		}
	}

	return samples
}
func pushSamples(samples []state.PushSample) []push.Sample {
	result := make([]push.Sample, 0, len(samples))
	for _, sample := range samples {
		result = append(result, push.Sample{Kind: sample.Kind, ID: sample.ID, Value: sample.Value})
	}
	return result
}

func (c *Client) resetMQTTPush(connected bool) {
	c.gatewayMu.RLock()
	defer c.gatewayMu.RUnlock()
	if !c.gatewayMode {
		c.resetPush("mqtt", connected)
	}
}
