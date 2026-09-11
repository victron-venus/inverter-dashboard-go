package config

import "time"

// DataSource is the exclusive live telemetry transport.
type DataSource string

const (
	DataSourceMQTT DataSource = "mqtt"
	DataSourceIGW  DataSource = "igw"
	DataSourceNone DataSource = "none"
)

// MQTTRecoveryProbeInterval is how often to probe Cerbo MQTT while on IGW
// after a dual-path failover (desktop connectionPolicy MQTT_RECOVERY_PROBE_MS).
const MQTTRecoveryProbeInterval = 60 * time.Second

// MQTTConnectWatchdog documents the soft window for initial MQTT connect
// attempts before preferring IGW when both are configured (desktop ~15s).
const MQTTConnectWatchdog = 15 * time.Second

// ChooseStartupSource picks the exclusive live path (desktop parity):
// MQTT wins when configured and reachable; otherwise IGW; single-transport
// configs are obvious.
func ChooseStartupSource(mqttConfigured, igwConfigured, mqttReachable bool) DataSource {
	if mqttConfigured && igwConfigured {
		if mqttReachable {
			return DataSourceMQTT
		}
		return DataSourceIGW
	}
	if mqttConfigured {
		return DataSourceMQTT
	}
	if igwConfigured {
		return DataSourceIGW
	}
	return DataSourceNone
}
