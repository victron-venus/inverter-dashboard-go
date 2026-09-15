package gateway

import (
	"encoding/json"
	"testing"
)

func TestNativeSnapshotZeroPhaseAndUnits(t *testing.T) {
	var snap Snapshot
	err := json.Unmarshal([]byte(`{
      "system": {"0/Ac/Grid/L1/Power":100,"0/Ac/Grid/L2/Power":200,"0/Ac/Grid/L3/Power":-300,"0/Dc/Battery/Soc":99,"0/Dc/Battery/Voltage":40},
      "solarcharger": {"2/Yield/Power":0,"2/Dc/0/Power":900},
      "pvinverter": {"3/Ac/Power":0,"3/Ac/L1/Power":800},
      "tank": {"21/Level":0.5},
      "ev": {"22/Ac/Power":3200},
      "evcharger": {"40/Ac/Power":7200}
    }`), &snap)
	if err != nil {
		t.Fatal(err)
	}
	st := SnapshotToState(&snap, MapOptions{})
	if st.GT != 0 || st.G3 != -300 || !st.TelemetryAvailable["gt"] || !st.TelemetryAvailable["battery_soc"] {
		t.Fatalf("zero/phase/SOC lost: %+v", st)
	}
	if st.MpptTotal != 0 || st.SolarTotal != 0 || st.WaterLevel != 0.5 || st.EVPower != 3200 || st.EVChargingKW != 7.2 {
		t.Fatalf("native priorities or units wrong: %+v", st)
	}
	empty := SnapshotToState(&Snapshot{}, MapOptions{})
	if empty.TelemetryAvailable["gt"] || empty.TelemetryAvailable["battery_soc"] || len(empty.Batteries) != 0 {
		t.Fatalf("empty snapshot fabricated measurements: %+v", empty)
	}
}

func TestExplicitZeroWaterInstancesRemainExact(t *testing.T) {
	var snap Snapshot
	if err := json.Unmarshal([]byte(`{"tank":{"0/Level":0.5,"21/Level":99},"pump":{"0/Mode":2,"0/State":0,"1/Mode":1,"2/Mode":0},"capabilities":{"water_mode":true}}`), &snap); err != nil {
		t.Fatal(err)
	}
	st := SnapshotToState(&snap, MapOptions{WaterInstancesConfigured: true})
	if st.WaterLevel != .5 || st.PumpMode != 2 || st.WaterValveMode != 2 || !st.TelemetryAvailable["pump_mode"] || !st.GatewayCapabilities["water_mode"] {
		t.Fatal("explicit instance zero mapped to default device")
	}
}
