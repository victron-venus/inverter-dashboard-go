package homeassistant

import (
	"reflect"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func fdoc(id, state string, attrs map[string]interface{}) *EntityState {
	return &EntityState{EntityID: id, State: state, Attributes: attrs}
}

func TestFilteredEmptyAndAll(t *testing.T) {
	if !filteredEmpty(nil) {
		t.Error("nil config must be empty")
	}
	if !filteredEmpty(&FilteredEntityConfig{}) {
		t.Error("zero config must be empty")
	}
	cfg := &FilteredEntityConfig{
		Covers:  []string{"cover.b", "cover.b"},
		Sensors: []string{"sensor.t"},
		Weather: "weather.h",
	}
	got := filteredAll(cfg)
	want := []string{"cover.b", "sensor.t", "weather.h"} // duplicates removed
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filteredAll = %v, want %v", got, want)
	}
}

func TestBuildFilteredDisplaysEmpty(t *testing.T) {
	out := BuildFilteredDisplays(map[string]*EntityState{}, nil)
	for _, k := range []string{"sensors", "numbers", "covers", "media_players", "scenes"} {
		if s, _ := out[k].([]interface{}); len(s) != 0 {
			t.Errorf("%s should be empty, got %v", k, out[k])
		}
	}
	if out["weather"] != nil {
		t.Error("weather should be nil")
	}
}

func TestBuildFilteredDisplaysMappings(t *testing.T) {
	docs := map[string]*EntityState{
		"sensor.t":       fdoc("sensor.t", "21.5", map[string]interface{}{"friendly_name": "Temp", "unit_of_measurement": "°C"}),
		"number.i":       fdoc("number.i", "12", map[string]interface{}{"min": float64(6), "max": float64(32), "step": float64(2)}),
		"number.bad":     fdoc("number.bad", "unavailable", nil),
		"cover.b":        fdoc("cover.b", "open", map[string]interface{}{"current_position": float64(42)}),
		"cover.nopos":    fdoc("cover.nopos", "closed", nil),
		"media_player.s": fdoc("media_player.s", "playing", nil),
		"scene.m":        fdoc("scene.m", "scening", nil),
		"weather.h": fdoc("weather.h", "sunny", map[string]interface{}{
			"temperature": 24.5, "temperature_unit": "°C",
			"forecast": []interface{}{map[string]interface{}{"datetime": "d1"}},
		}),
	}
	cfg := &FilteredEntityConfig{
		Covers:       []string{"cover.b", "cover.nopos"},
		MediaPlayers: []string{"media_player.s"},
		Scenes:       []string{"scene.m"},
		Numbers:      []string{"number.i", "number.bad"},
		Sensors:      []string{"sensor.t"},
		Weather:      "weather.h",
	}
	out := BuildFilteredDisplays(docs, cfg)

	sensors := out["sensors"].([]interface{})
	if len(sensors) != 1 || sensors[0].(map[string]interface{})["unit"] != "°C" {
		t.Errorf("sensors mapping wrong: %v", sensors)
	}

	numbers := out["numbers"].([]interface{})
	if len(numbers) != 1 {
		t.Fatalf("unavailable number must be skipped, got %v", numbers)
	}
	n := numbers[0].(map[string]interface{})
	if n["value"].(float64) != 12 || n["min"].(float64) != 6 || n["step"].(float64) != 2 {
		t.Errorf("number mapping wrong: %v", n)
	}

	covers := out["covers"].([]interface{})
	if covers[0].(map[string]interface{})["position"].(int) != 42 {
		t.Errorf("cover position attr not used: %v", covers[0])
	}
	if covers[1].(map[string]interface{})["position"].(int) != 0 {
		t.Errorf("closed cover without position must be 0: %v", covers[1])
	}

	if mp := out["media_players"].([]interface{}); mp[0].(map[string]interface{})["state"] != "playing" {
		t.Errorf("media player state wrong: %v", mp)
	}
	if sc := out["scenes"].([]interface{}); len(sc) != 1 {
		t.Errorf("scenes wrong: %v", sc)
	}

	w := out["weather"].(map[string]interface{})
	if w["temperature"].(float64) != 24.5 || w["state"] != "sunny" {
		t.Errorf("weather mapping wrong: %v", w)
	}
	if fc, ok := w["forecast"].([]interface{}); !ok || len(fc) != 1 {
		t.Errorf("forecast passthrough wrong: %v", w["forecast"])
	}
}

func TestBuildFilteredDisplaysMissingDocs(t *testing.T) {
	cfg := &config.FilteredEntityConfig{
		Covers: []string{"cover.ghost"}, Weather: "weather.ghost",
	}
	out := BuildFilteredDisplays(map[string]*EntityState{}, cfg)
	if len(out["covers"].([]interface{})) != 0 || out["weather"] != nil {
		t.Errorf("missing docs must leave sections empty: %v", out)
	}
}
