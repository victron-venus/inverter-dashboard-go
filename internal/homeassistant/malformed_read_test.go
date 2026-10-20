package homeassistant

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/victron-venus/inverter-dashboard-go/internal/config"
)

func assertEntityReadHealth(t *testing.T, c *Client, valid bool) {
	t.Helper()
	o, err := c.FetchStatesOnce()
	if err != nil || o.HADirectConnected != valid || c.ControlsAvailable() != valid {
		t.Fatalf("health=%v controls=%v error=%v want=%v", o.HADirectConnected, c.ControlsAvailable(), err, valid)
	}
	if valid && o.AdditionalFields["home_lamp"] != true {
		t.Fatal("valid observation did not recover after malformed response")
	}
}

func TestMalformedEntityReadDoesNotRenewHealth(t *testing.T) {
	for _, bad := range []string{"null", "[]", "{}", `{"state":null}`, `{"state":1}`, `{"state":[]}`, `{"state":"on","attributes":[]}`} {
		t.Run(bad, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{
				URL: "http://ha.test", Token: "fixture", DirectControls: true,
				SwitchEntities: []config.EntityConfig{{Key: "home_lamp", Entity: "switch.lamp"}},
			})
			body := `{"state":"on"}`
			c.httpClient = &http.Client{Transport: ownershipTransport(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			for _, valid := range []bool{true, false, true} {
				body = bad
				if valid {
					body = `{"state":"on"}`
				}
				assertEntityReadHealth(t, c, valid)
			}
		})
	}
}

func TestNonFiniteHAReadRemainsJSONSerializable(t *testing.T) {
	for _, raw := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity", "1e309"} {
		t.Run(raw, func(t *testing.T) {
			c := NewClient(&config.HomeAssistantConfig{
				URL: "http://ha.test", Token: "fixture", DirectControls: true,
				SensorEntities: map[string]string{"room_temperature": "sensor.room"},
				ApplianceEntities: map[string]string{
					"washer_time": "sensor.washer_time", "washer_power": "sensor.washer_power",
				},
				FilteredEntities: &config.FilteredEntityConfig{Numbers: []string{"number.limit"}},
			})
			c.httpClient = &http.Client{Transport: ownershipTransport(func(_ *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]string{"state": raw})
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			o, err := c.FetchStatesOnce()
			if err != nil || !o.HADirectConnected {
				t.Fatalf("decoded HA response lost transport health: %v", err)
			}
			if o.AdditionalFields["room_temperature"] != nil || o.AdditionalFields["washer_time"] != 0 || o.AdditionalFields["washer_power"] != false {
				t.Fatalf("invalid numeric data acquired a value: %#v", o.AdditionalFields)
			}
			if _, err := json.Marshal(o.AdditionalFields); err != nil {
				t.Fatalf("HA observation broke dashboard JSON: %v", err)
			}
		})
	}
}

func TestDurationRejectsOverflowAndMalformedComponents(t *testing.T) {
	for _, raw := range []string{"1e100", "-1", "9007199254740992", "150119987579016:32", "2501999792984:00:00", "9223372036854775807", "9223372036854775807:00", "1:bad:20", "1:-1:20", "0:60:00", "+1:20", "-0:20", "1: 20", "1:２０"} {
		if got := parseStateToSeconds(raw); got != 0 {
			t.Errorf("invalid duration %q became %d", raw, got)
		}
	}
	for raw, want := range map[string]int{"1.9": 1, "100:00:00": 360000, " 90:00 ": 5400} {
		if got := parseStateToSeconds(raw); got != want {
			t.Errorf("valid duration %q became %d; want %d", raw, got, want)
		}
	}
	var maxSafe int64 = 9007199254740991
	want := 0
	if strconv.IntSize == 64 {
		want = int(maxSafe)
	}
	if got := parseStateToSeconds("150119987579016:31"); got != want {
		t.Errorf("maximum safe duration became %d; want %d", got, want)
	}
}

func TestNonFiniteFilteredAttributesRemainSerializable(t *testing.T) {
	docs := map[string]*EntityState{
		"number.limit": {State: "5", Attributes: map[string]interface{}{"min": "NaN", "max": "Inf", "step": "NaN"}},
		"weather.home": {State: "sunny", Attributes: map[string]interface{}{"temperature": "Inf", "humidity": "NaN"}},
	}
	out := BuildFilteredDisplays(docs, &FilteredEntityConfig{Numbers: []string{"number.limit"}, Weather: "weather.home"})
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("invalid numeric attributes broke dashboard JSON: %v", err)
	}
}

func TestInvalidNumberBoundsDoNotAuthorizeActions(t *testing.T) {
	for _, limits := range []string{`"min":"NaN","max":100`, `"min":0,"max":"Inf"`, `"min":null,"max":100`, `"min":10,"max":1`} {
		c := NewClient(&config.HomeAssistantConfig{
			URL: "http://ha.test", Token: "fixture", DirectControls: true,
			FilteredEntities: &config.FilteredEntityConfig{Numbers: []string{"number.limit"}},
		})
		c.httpClient = &http.Client{Transport: ownershipTransport(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Error("invalid bounds allowed a service write")
			}
			body := `{"state":"5","attributes":{` + limits + `}}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		result, err := c.FetchStatesOnce()
		if err != nil {
			t.Fatal(err)
		}
		displays := result.AdditionalFields["ha_filtered"].(map[string]interface{})
		if len(displays["numbers"].([]interface{})) != 0 {
			t.Fatal("invalid explicit limits still displayed a number control")
		}
		if len(c.numberRanges) != 0 {
			t.Fatal("invalid explicit limits acquired default control range")
		}
		if err := c.PerformAction("number_set", "number.limit", map[string]interface{}{"value": 5.0}); err == nil {
			t.Fatal("invalid explicit bounds authorized a number action")
		}
	}
}

func TestValidNumberBoundsMatchDisplayAndAuthorization(t *testing.T) {
	for _, tc := range []struct {
		attributes string
		minimum    float64
		maximum    float64
	}{
		{`{}`, 0, 100},
		{`{"min":0,"max":0}`, 0, 100},
		{`{"min":"-10","max":"20"}`, -10, 20},
	} {
		c := NewClient(&config.HomeAssistantConfig{
			URL: "http://ha.test", Token: "fixture", DirectControls: true,
			FilteredEntities: &config.FilteredEntityConfig{Numbers: []string{"number.limit"}},
		})
		c.httpClient = &http.Client{Transport: ownershipTransport(func(*http.Request) (*http.Response, error) {
			body := `{"state":"5","attributes":` + tc.attributes + `}`
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		result, err := c.FetchStatesOnce()
		if err != nil {
			t.Fatal(err)
		}
		displays := result.AdditionalFields["ha_filtered"].(map[string]interface{})
		numbers := displays["numbers"].([]interface{})
		if len(numbers) != 1 {
			t.Fatalf("valid bounds did not display a number: %s", tc.attributes)
		}
		number := numbers[0].(map[string]interface{})
		if number["min"] != tc.minimum || number["max"] != tc.maximum ||
			c.numberRanges["number.limit"] != [2]float64{tc.minimum, tc.maximum} {
			t.Fatalf("display and action bounds differ for %s", tc.attributes)
		}
	}
}
