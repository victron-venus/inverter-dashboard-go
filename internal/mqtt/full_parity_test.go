package mqtt

import (
	"encoding/json"
	"testing"
)

func TestExplicitUnknownGridPhaseNeverResurrectsFallback(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	send(c, "grid/30", "Ac/L1/Power", 900)
	send(c, "grid/30", "Ac/L2/Power", 800)
	send(c, "system/0", "Ac/Grid/L1/Power", 0)
	send(c, "system/0", "Ac/Grid/L2/Power", nil)
	st := c.GetState()
	if st.GridL1Available == nil || !*st.GridL1Available || st.GridL2Available == nil || *st.GridL2Available || st.G1 != 0 || st.G2 != 0 || st.GT != 0 {
		t.Fatalf("invalid phase result: %+v", st)
	}
	send(c, "system/0", "Ac/Grid/L1/Power", nil)
	send(c, "grid/30", "Ac/Power", 1700)
	st = c.GetState()
	if st.TelemetryAvailable["gt"] || st.GT != 0 {
		t.Fatal("unknown phases resurrected aggregate fallback")
	}
}

func TestDailyGridEnvelopeAndBackupReceiptRemainSourceOwned(t *testing.T) {
	c := NewClient("localhost", 1883)
	c.SetWaterConfig("p1", 21, 1, 2)
	controllerMessage(c, `{"daily_stats":{"grid_energy":{"date":"2026-10-06","complete":false,"opaque":[null,0]},"solar_kwh":12},"grid_backup":{"enabled":true,"available":true,"measurement_time":1700000000},"grid_using_backup":true,"grid_backup_observed_at":99999999999}`)
	st := c.GetState()
	if st.GridBackupObservedAt == nil || *st.GridBackupObservedAt == 99999999999 {
		t.Fatal("receipt forged")
	}
	at := *st.GridBackupObservedAt
	var envelope map[string]interface{}
	if json.Unmarshal(st.DailyStats.GridEnergy, &envelope) != nil || envelope["complete"] != false || st.DailyStats.SolarKWh != 12 {
		t.Fatal("native energy envelope lost")
	}
	send(c, "system/0", "Ac/Grid/L1/Power", 123)
	if got := c.GetState(); got.GridBackupObservedAt == nil || *got.GridBackupObservedAt != at {
		t.Fatal("unrelated native sample renewed backup")
	}
	c.onStateMessage(nil, &retainedESSMessage{&fakeMessage{payload: []byte(`{"grid_backup":{"enabled":true,"available":true},"grid_backup_observed_at":99999999999}`)}})
	if c.GetState().GridBackupObservedAt != nil {
		t.Fatal("backup missing measurement time looks fresh")
	}
}

func TestGridAndConsumptionAlwaysRequireNativeMeasurement(t *testing.T) {
	c := NewClient("localhost", 1883)
	controllerMessage(c, `{"gt":999,"g1":999,"g2":999,"tt":777,"t1":777,"t2":777,"battery_soc":88,"grid_l1_available":true}`)
	st := c.GetState()
	if st.GT != 0 || st.TT != 0 || st.G1 != 0 || st.GridL1Available != nil || st.BatterySOC != 0 || st.TelemetryAvailable["gt"] || st.TelemetryAvailable["tt"] {
		t.Fatal("daemon invented native grid/consumption measurements")
	}
}
