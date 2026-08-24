package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds application configuration matching Python exactly
type Config struct {
	MQTT              MQTTConfig
	Web               WebConfig
	GitHub            GitHubConfig
	Cerbo             CerboConfig
	DashboardSecret   string
	SelfUpdateEnabled bool
	HomeAssistant     *HomeAssistantConfig
}

// CerboConfig selects the dbus-pump water topics on the Cerbo MQTT broker.
// Empty PortalID disables water.
type CerboConfig struct {
	PortalID      string
	TankInstance  int
	PumpInstance  int
	ValveInstance int
}

// MQTTConfig from environment with Python defaults
type MQTTConfig struct {
	Host string
	Port int
}

// WebConfig from environment with Python defaults
type WebConfig struct {
	Port int
	Host string
}

// GitHubConfig holds repository info
type GitHubConfig struct {
	Repository string
	RawURL     string
}

type EntityConfig struct {
	Key    string `yaml:"key"`
	Entity string `yaml:"entity"`
	Label  string `yaml:"label"`
	Order  int    `yaml:"order"`
}

type BooleanEntityConfig struct {
	Key    string `yaml:"key"`
	Entity string `yaml:"entity"`
	Order  int    `yaml:"order"`
}

// convertMapToBooleanEntitySlice converts a map[string]interface{} from YAML to []BooleanEntityConfig
func convertMapToBooleanEntitySlice(input map[string]interface{}) []BooleanEntityConfig {
	var result []BooleanEntityConfig
	for key, value := range input {
		var entity string
		var order int

		switch v := value.(type) {
		case string:
			entity = v
			order = 0
		case map[string]interface{}:
			if ent, ok := v["entity"].(string); ok {
				entity = ent
			}
			if ord, ok := v["order"].(int); ok {
				order = ord
			} else if ord, ok := v["order"].(float64); ok {
				order = int(ord)
			}
		}

		result = append(result, BooleanEntityConfig{
			Key:    key,
			Entity: entity,
			Order:  order,
		})
	}
	return result
}

// defaultLabel creates a default label from a state key by stripping "HOME_" prefix and converting underscores to spaces
func defaultLabel(key string) string {
	s := strings.ToUpper(key)
	s = strings.TrimPrefix(s, "HOME_")
	return strings.ReplaceAll(s, "_", " ")
}

// convertMapToEntitySlice converts a map[string]interface{} from YAML to []EntityConfig
func convertMapToEntitySlice(input map[string]interface{}) []EntityConfig {
	var result []EntityConfig
	for key, value := range input {
		btn := EntityConfig{
			Key:   key,
			Order: 0,
		}

		switch v := value.(type) {
		case string:
			btn.Entity = v
			btn.Label = defaultLabel(key)

		case []interface{}:
			if len(v) > 0 {
				if entityID, ok := v[0].(string); ok {
					btn.Entity = entityID
				}
				if len(v) > 1 {
					if label, ok := v[1].(string); ok && label != "" {
						btn.Label = label
					}
				}
			}

		case []string:
			if len(v) > 0 {
				btn.Entity = v[0]
				if len(v) > 1 && v[1] != "" {
					btn.Label = v[1]
				}
			}

		case map[string]interface{}:
			if entityID, ok := v["entity"].(string); ok {
				btn.Entity = entityID
			}
			if label, ok := v["label"].(string); ok && label != "" {
				btn.Label = label
			} else if label, ok := v["short"].(string); ok && label != "" {
				btn.Label = label
			} else if label, ok := v["name"].(string); ok && label != "" {
				btn.Label = label
			}
			if order, ok := v["order"].(int); ok {
				btn.Order = order
			} else if order, ok := v["order"].(float64); ok {
				btn.Order = int(order)
			}
		}

		if btn.Label == "" {
			btn.Label = defaultLabel(key)
		}

		result = append(result, btn)
	}
	return result
}

// HomeAssistantConfig from config.yaml
type HomeAssistantConfig struct {
	URL                string
	Token              string
	DirectControls     bool
	PollInterval       float64
	BooleanEntities    []BooleanEntityConfig
	SwitchEntities     []EntityConfig
	CarSOCEntity       string
	EVChargingKWEntity string
	EVPowerEntity      string
	ApplianceEntities  map[string]string
	VueSensors         map[string]string
	SensorEntities     map[string]string
}

// Load reads configuration matching Python config.py behavior exactly
func Load(configPath string) (*Config, error) {
	log.Printf("[CONFIG DEBUG] Load() called")
	// Parse Home Assistant secrets from python file if present

	cfg := &Config{
		MQTT: MQTTConfig{
			Host: getEnvDefault("MQTT_HOST", "192.168.160.150"),
			Port: getEnvIntDefault("MQTT_PORT", 1883),
		},
		Web: WebConfig{
			Port: getEnvIntDefault("WEB_PORT", 8080),
			Host: "0.0.0.0",
		},
		GitHub: GitHubConfig{
			Repository: "victron-venus/inverter-dashboard-go",
			RawURL:     "https://raw.githubusercontent.com/victron-venus/inverter-dashboard-go/main",
		},
		Cerbo: CerboConfig{
			PortalID:      getEnvDefault("CERBO_PORTAL_ID", ""),
			TankInstance:  getEnvIntDefault("WATER_TANK_INSTANCE", 21),
			PumpInstance:  getEnvIntDefault("WATER_PUMP_INSTANCE", 1),
			ValveInstance: getEnvIntDefault("WATER_VALVE_INSTANCE", 2),
		},
		DashboardSecret:   getEnvDefault("DASHBOARD_SECRET", ""),
		SelfUpdateEnabled: getEnvDefault("SELF_UPDATE_ENABLED", "") == "true",
	}

	// Load HomeAssistant configuration from config.yaml if present
	if err := loadConfigYAML(cfg); err != nil {
		log.Printf("[CONFIG] No config.yaml found or invalid: %v (continuing with MQTT-only mode)", err)
	} else if cfg.HomeAssistant != nil {
		log.Printf("[CONFIG] HomeAssistant configuration loaded successfully")
		logHomeAssistantConfig(cfg.HomeAssistant)
	}

	return cfg, nil
}

// loadConfigYAML loads HomeAssistant configuration from config.yaml
func loadConfigYAML(cfg *Config) error {
	const yamlFile = "config.yaml"
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", yamlFile, err)
	}

	// Define a nested struct to match the YAML structure
	type yamlMQTT struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}
	type yamlWeb struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	}
	type yamlCerbo struct {
		PortalID      string `yaml:"portal_id"`
		TankInstance  *int   `yaml:"tank_instance"`
		PumpInstance  *int   `yaml:"pump_instance"`
		ValveInstance *int   `yaml:"valve_instance"`
	}
	type yamlTop struct {
		MQTT          *yamlMQTT  `yaml:"mqtt"`
		Web           *yamlWeb   `yaml:"web"`
		Cerbo         *yamlCerbo `yaml:"cerbo"`
		HomeAssistant *struct {
			URL                 string                 `yaml:"url"`
			Token               string                 `yaml:"token"`
			DirectControls      *bool                  `yaml:"direct_controls"`
			PollIntervalSeconds float64                `yaml:"poll_interval_seconds"`
			BooleanEntities     map[string]interface{} `yaml:"boolean_entities"`
			SwitchEntities      map[string]interface{} `yaml:"switch_entities"`
			CarSOCEntity        string                 `yaml:"car_soc_entity"`
			EVChargingKWEntity  string                 `yaml:"ev_charging_kw_entity"`
			EVPowerEntity       string                 `yaml:"ev_power_entity"`
			ApplianceEntities   map[string]string      `yaml:"appliance_entities"`
			VueSensors          map[string]string      `yaml:"vue_sensors"`
		} `yaml:"homeassistant"`
	}

	var top yamlTop
	if err := yaml.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("failed to parse %s: %w", yamlFile, err)
	}

	// Apply MQTT config from YAML if present
	if top.MQTT != nil {
		if top.MQTT.Host != "" {
			cfg.MQTT.Host = top.MQTT.Host
		}
		if top.MQTT.Port > 0 {
			cfg.MQTT.Port = top.MQTT.Port
		}
	}

	if top.Cerbo != nil {
		if top.Cerbo.PortalID != "" {
			cfg.Cerbo.PortalID = top.Cerbo.PortalID
		}
		if top.Cerbo.TankInstance != nil {
			cfg.Cerbo.TankInstance = *top.Cerbo.TankInstance
		}
		if top.Cerbo.PumpInstance != nil {
			cfg.Cerbo.PumpInstance = *top.Cerbo.PumpInstance
		}
		if top.Cerbo.ValveInstance != nil {
			cfg.Cerbo.ValveInstance = *top.Cerbo.ValveInstance
		}
	}

	if top.Web != nil {
		if top.Web.Host != "" {
			cfg.Web.Host = top.Web.Host
		}
		if top.Web.Port > 0 {
			cfg.Web.Port = top.Web.Port
		}
	}

	if top.HomeAssistant != nil {
		// Default DirectControls to true (Python behavior), unless explicitly set to false
		directControls := true
		if top.HomeAssistant.DirectControls != nil {
			directControls = *top.HomeAssistant.DirectControls
		}
		ha := &HomeAssistantConfig{
			URL:                top.HomeAssistant.URL,
			Token:              top.HomeAssistant.Token,
			DirectControls:     directControls,
			PollInterval:       pollOrDefault(top.HomeAssistant.PollIntervalSeconds),
			BooleanEntities:    convertMapToBooleanEntitySlice(top.HomeAssistant.BooleanEntities),
			SwitchEntities:     convertMapToEntitySlice(top.HomeAssistant.SwitchEntities),
			CarSOCEntity:       top.HomeAssistant.CarSOCEntity,
			EVChargingKWEntity: top.HomeAssistant.EVChargingKWEntity,
			EVPowerEntity:      top.HomeAssistant.EVPowerEntity,
			ApplianceEntities:  top.HomeAssistant.ApplianceEntities,
			VueSensors:         top.HomeAssistant.VueSensors,
		}
		cfg.HomeAssistant = ha
	}

	return nil
}

// pollOrDefault guards against a zero/absent poll interval: haPoller would
// panic in time.NewTicker and crash the whole process.
func pollOrDefault(v float64) float64 {
	if v <= 0 {
		return 12.0 // matches the Python dashboard default
	}
	return v
}

// logHomeAssistantConfig logs all HomeAssistant configuration values
func logHomeAssistantConfig(cfg *HomeAssistantConfig) {
	if cfg == nil {
		return
	}
	log.Println("=== HomeAssistant Configuration Values ===")
	log.Printf("URL: %s", cfg.URL)
	log.Printf("Token: %s...%s (truncated)", cfg.Token[:10], cfg.Token[len(cfg.Token)-5:])
	log.Printf("Direct Controls: %v", cfg.DirectControls)
	log.Printf("Poll Interval: %.1f seconds", cfg.PollInterval)
	log.Printf("Car SoC Entity: %s", cfg.CarSOCEntity)
	log.Printf("EV Charging KW Entity: %s", cfg.EVChargingKWEntity)
	log.Printf("EV Power Entity: %s", cfg.EVPowerEntity)
	log.Printf("Boolean Entities: %d entries", len(cfg.BooleanEntities))
	for _, entity := range cfg.BooleanEntities {
		log.Printf(" - %s: %s", entity.Key, entity.Entity)
	}
	log.Printf("Switch Entities: %d entries", len(cfg.SwitchEntities))
	for _, entity := range cfg.SwitchEntities {
		log.Printf(" - %s: %v", entity.Key, entity)
	}
	log.Printf("Appliance Entities: %d entries", len(cfg.ApplianceEntities))
	for key, entity := range cfg.ApplianceEntities {
		log.Printf("  - %s: %s", key, entity)
	}
}

// getEnvDefault reads environment variable or returns default
func getEnvDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntDefault reads environment variable as int or returns default
func getEnvIntDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetExampleConfigPath returns path to example config (for reference)
func GetExampleConfigPath() (string, error) {
	return "", nil // No config file in Python version
}
