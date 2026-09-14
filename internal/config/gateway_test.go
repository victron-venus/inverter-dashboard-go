package config

import (
	"os"
	"testing"
)

func TestGatewayConfiguredCredentials(t *testing.T) {
	for _, tc := range []struct {
		name              string
		id, secret, token string
		want              bool
	}{
		{"native bearer", "", "", "token", true},
		{"cloudflare and bearer", "id", "secret", "token", true},
		{"cloudflare only", "id", "secret", "", true},
		{"missing credentials", "", "", "", false},
		{"partial Access id", "id", "", "token", false},
		{"partial Access secret", "", "secret", "token", false},
		{"whitespace credentials", " ", " ", " ", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Gateway: GatewayConfig{Enabled: true, URL: "https://igw.example:9151", AccessClientID: tc.id, AccessClientSecret: tc.secret, APIToken: tc.token}}
			if got := cfg.GatewayConfigured(); got != tc.want {
				t.Fatalf("GatewayConfigured = %t, want %t", got, tc.want)
			}
			cfg.Gateway.Enabled = false
			if cfg.GatewayConfigured() {
				t.Fatal("disabled gateway configured")
			}
		})
	}
}

func TestGatewayEnvironmentOverridesMountedYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	data := []byte(`gateway:
  enabled: false
  url: http://old-gateway:30150
  access_client_id: old-id
  access_client_secret: old-secret
  api_token: old-token
  poll_interval_seconds: 9
`)
	if err := os.WriteFile("config.yaml", data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_ENABLED", "true")
	t.Setenv("GATEWAY_URL", "https://igw.s.2560801.xyz:9151")
	t.Setenv("GATEWAY_ACCESS_CLIENT_ID", "")
	t.Setenv("GATEWAY_ACCESS_CLIENT_SECRET", "")
	t.Setenv("GATEWAY_API_TOKEN", "new-token")
	t.Setenv("GATEWAY_POLL_INTERVAL_SECONDS", "2")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GatewayConfigured() || cfg.Gateway.URL != "https://igw.s.2560801.xyz:9151" || cfg.Gateway.PollIntervalSec != 2 {
		t.Fatal("environment gateway settings were overridden by mounted YAML")
	}
	if cfg.Gateway.AccessClientID != "" || cfg.Gateway.AccessClientSecret != "" || cfg.Gateway.APIToken != "new-token" {
		t.Fatal("old YAML credentials overrode explicit environment settings")
	}
}

func TestGatewayYAMLWithoutEnvironment(t *testing.T) {
	for _, key := range []string{"GATEWAY_ENABLED", "GATEWAY_URL", "GATEWAY_ACCESS_CLIENT_ID", "GATEWAY_ACCESS_CLIENT_SECRET", "GATEWAY_API_TOKEN", "GATEWAY_POLL_INTERVAL_SECONDS"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.yaml", []byte(`gateway:
  enabled: true
  url: https://igw.example:9151
  api_token: yaml-token
  poll_interval_seconds: 3
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GatewayConfigured() || cfg.Gateway.URL != "https://igw.example:9151" || cfg.Gateway.APIToken != "yaml-token" || cfg.Gateway.PollIntervalSec != 3 {
		t.Fatal("standalone YAML gateway configuration was not loaded")
	}
}
