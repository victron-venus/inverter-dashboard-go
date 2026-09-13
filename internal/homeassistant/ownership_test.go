package homeassistant

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

type ownershipTransport func(*http.Request) (*http.Response, error)

func (f ownershipTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPollKeepsAppliancesAndSkipsCerboMirrors(t *testing.T) {
	states := map[string]string{
		"input_boolean.holiday": "on", "light.kitchen": "off",
		"sensor.washer_remaining": "01:30:00", "binary_sensor.washer_running": "on",
		"sensor.room_temperature": "21.5",
	}
	var requested []string
	transport := ownershipTransport(func(r *http.Request) (*http.Response, error) {
		entity := r.URL.Path[len("/api/states/"):]
		requested = append(requested, entity)
		value, exists := states[entity]
		if !exists {
			t.Errorf("polled MQTT-owned mirror %s", entity)
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		body, _ := json.Marshal(EntityState{EntityID: entity, State: value})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	c := NewClient(&config.HomeAssistantConfig{
		URL: "http://ha.test", Token: "test", DirectControls: true,
		BooleanEntities: []config.BooleanEntityConfig{
			{Key: "only_charging", Entity: "input_boolean.only_charging"},
			{Key: "legacy_alias", Entity: "binary_sensor.no_feed"},
			{Key: "holiday", Entity: "input_boolean.holiday"},
		},
		SwitchEntities: []config.EntityConfig{
			{Key: "pump_switch", Entity: "switch.old_pump"},
			{Key: "home_light", Entity: "light.kitchen"},
		},
		SensorEntities:    map[string]string{"battery_soc": "sensor.old_soc", "room_temperature": "sensor.room_temperature"},
		ApplianceEntities: map[string]string{"washer_time": "sensor.washer_remaining", "washer_power": "binary_sensor.washer_running"},
		VueSensors:        map[string]string{"old_load": "sensor.old_load"},
	})
	c.httpClient = &http.Client{Transport: transport}
	o, err := c.FetchStatesOnce()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(o.Booleans, map[string]bool{"holiday": true}) || o.AdditionalFields["washer_time"] != 5400 || o.AdditionalFields["washer_power"] != true {
		t.Fatalf("HA appliance poll lost data: %#v", o)
	}
	want := []string{"holiday", "home_light", "room_temperature", "washer_power", "washer_time"}
	got := c.GetManagedKeys()
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("HA outage would reset wrong keys: %v", got)
	}
	if c.IsToggleAllowed("input_boolean.only_charging") || c.IsToggleAllowed("binary_sensor.no_feed") || c.IsToggleAllowed("switch.old_pump") || !c.IsToggleAllowed("light.kitchen") {
		t.Fatal("HA control ownership is incorrect")
	}
	if len(requested) != len(states) {
		t.Errorf("polled %v, want %v", requested, states)
	}
}
