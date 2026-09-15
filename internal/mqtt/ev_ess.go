package mqtt

import (
	"fmt"
	"strconv"

	"github.com/victron-venus/inverter-dashboard-go/internal/state"
)

func evSOC(d leaves) (float64, bool) {
	n, ok := d.num("Soc")
	return n, ok && n >= 0 && n <= 100
}

// Negative preferences discover the first connected device with actual readings.
// Explicit instances, including zero, never silently select a different vehicle.
func selectEV(ds map[string]leaves, preferred int) leaves {
	if preferred >= 0 {
		return ds[strconv.Itoa(preferred)]
	}
	for _, id := range sortedStringKeys(ds) {
		d := ds[id]
		_, hasSOC := evSOC(d)
		_, hasPower := d.acPower()
		if hasSOC || hasPower {
			return d
		}
	}
	return nil
}

func applyEVOverlay(all map[string]map[string]interface{}, o CerboOptions, out map[string]interface{}) {
	evs, chargers := devices(all, "ev"), devices(all, "evcharger")
	inventory := []state.CerboDevice{}
	for _, group := range []struct {
		kind string
		ds   map[string]leaves
	}{{"ev", evs}, {"evcharger", chargers}, {"tank", devices(all, "tank")}, {"pump", devices(all, "pump")}} {
		for _, id := range sortedStringKeys(group.ds) {
			d := group.ds[id]
			inst, _ := strconv.Atoi(id)
			row := state.CerboDevice{Kind: group.kind, Instance: inst, Name: deviceName(d, group.kind+" "+id)}
			if n, ok := evSOC(d); ok {
				row.SOC = &n
			}
			if n, ok := d.acPower(); ok {
				row.Power = &n
			}
			inventory = append(inventory, row)
		}
	}
	out["discovered_water_ev"] = inventory
	out["ev_present"] = len(evs) > 0
	out["evcharger_present"] = len(chargers) > 0
	ev, charger := selectEV(evs, o.EVInstance), selectEV(chargers, o.EVChargerInstance)
	if n, ok := ev.acPower(); ok {
		out["ev_power"] = n
		out["car_charging_power"] = n
	}
	if n, ok := charger.acPower(); ok {
		out["ev_charging_power"] = n
		out["ev_charging_kw"] = n / 1000
	}
	if n, ok := evSOC(ev); ok {
		out["car_soc"] = n
	} else if o.EVInstance < 0 {
		// Some dbus-ev versions use the evcharger service name for vehicle SOC.
		if n, ok := evSOC(charger); ok {
			out["car_soc"] = n
		}
	}
}

func applyESSOverlay(all map[string]map[string]interface{}, out map[string]interface{}) {
	settings := devices(all, "settings")
	for _, id := range sortedStringKeys(settings) {
		d := settings[id]
		hub, known := d.num("Settings/CGwacs/Hub4Mode")
		if !known || hub < 0 || hub != float64(int(hub)) {
			continue
		}
		bl, hasBL := d.num("Settings/CGwacs/BatteryLife/State")
		hasBL = hasBL && bl >= 0 && bl == float64(int(bl))
		mode := state.ESSMode{Hub4Mode: int(hub), IsExternal: hub == 3}
		if hasBL {
			mode.BatteryLifeState = int(bl)
		}
		switch int(hub) {
		case 3:
			mode.ModeName = "External control"
		case 1:
			if !hasBL {
				continue
			}
			switch int(bl) {
			case 0, 10:
				mode.ModeName = "Optimized without BatteryLife"
			case 9:
				mode.ModeName = "Keep batteries charged"
			default:
				mode.ModeName = "Optimized (BatteryLife)"
			}
		default:
			mode.ModeName = fmt.Sprintf("Unknown (%d)", int(hub))
		}
		out["ess_mode"] = mode
		return
	}
}
