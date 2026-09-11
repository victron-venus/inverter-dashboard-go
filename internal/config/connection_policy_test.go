package config

import "testing"

func TestChooseStartupSource(t *testing.T) {
	cases := []struct {
		name                 string
		mqtt, igw, reachable bool
		want                 DataSource
	}{
		{"mqtt only", true, false, false, DataSourceMQTT},
		{"igw only", false, true, false, DataSourceIGW},
		{"both mqtt up", true, true, true, DataSourceMQTT},
		{"both mqtt down", true, true, false, DataSourceIGW},
		{"none", false, false, false, DataSourceNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ChooseStartupSource(tc.mqtt, tc.igw, tc.reachable)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
