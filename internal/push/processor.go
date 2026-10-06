package push

import (
	"math"
	"sync"
	"time"
)

// Native is normalized source data. Timestamp must be the source event time.
type Native struct {
	ID, Source, Level, Title, Body string
	Timestamp                      time.Time
}

// Sample is one explicitly observed native leaf, never an inferred display value.
// Nil and retained observations invalidate the previous transition baseline.
type Sample struct {
	Kind, ID string
	Value    *float64
}
type Observation struct {
	Notifications []Native
	Samples       []Sample
	Retained      bool
	Complete      bool
}
type baseline struct {
	value float64
	at    time.Time
}
type Processor struct {
	mu        sync.Mutex
	store     *Store
	epoch     string
	source    string
	start     time.Time
	connected bool
	primed    bool
	baseline  map[string]baseline
	unknown   map[string]string
	selected  map[string]string
}

func NewProcessor(store *Store) *Processor { return &Processor{store: store} }
func (p *Processor) Reset(source string, connected bool, now time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	epoch, err := randomID()
	if err != nil {
		return err
	}
	if err = p.store.ResetEpoch(epoch); err != nil {
		return err
	}
	p.epoch = epoch
	p.source = source
	p.start = now
	p.connected = connected
	p.primed = false
	p.baseline = map[string]baseline{}
	p.unknown = map[string]string{}
	p.selected = map[string]string{}
	return nil
}
func (p *Processor) Observe(ob Observation, now time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.connected {
		return nil
	}
	if ob.Notifications != nil {
		current := map[string]bool{}
		prime := (p.source != "mqtt" && !p.primed) || (p.source == "mqtt" && now.Sub(p.start) < 10*time.Second) || ob.Retained
		for _, n := range ob.Notifications {
			identity := n.Title + "\x00" + n.Body
			current[n.ID] = true
			if old, ok := p.unknown[n.ID]; ok && old != identity {
				delete(p.unknown, n.ID)
			}
			if n.Timestamp.IsZero() {
				if prime {
					p.unknown[n.ID] = identity
				}
				continue
			}
			wasUnknown := p.unknown[n.ID] == identity
			source := "system"
			if n.Source == "victron" {
				source = "victron"
			}
			payload := makePayload("native", source, n.ID, n.Title, n.Body, n.Timestamp, now)
			silent := prime || wasUnknown || (n.Level != "warning" && n.Level != "error") || !payload.current(now) || n.Timestamp.Before(p.start.Add(-maxFutureSkew))
			if err := p.store.Enqueue(payload, p.epoch, silent); err != nil {
				return err
			}
			delete(p.unknown, n.ID)
		}
		for id := range p.unknown {
			if !current[id] {
				delete(p.unknown, id)
			}
		}
		p.primed = true
	}
	seenSamples := map[string]bool{}
	for _, sample := range ob.Samples {
		key := sample.Kind + ":" + sample.ID
		seenSamples[key] = true
		if old := p.selected[sample.Kind]; old != "" && old != key {
			delete(p.baseline, old)
		}
		p.selected[sample.Kind] = key
		previous, exists := p.baseline[key]
		if sample.Value == nil || ob.Retained || math.IsNaN(*sample.Value) || math.IsInf(*sample.Value, 0) {
			delete(p.baseline, key)
			continue
		}
		p.baseline[key] = baseline{*sample.Value, now}
		if !exists || now.Sub(previous.at) > 30*time.Second || now.Before(previous.at) {
			continue
		}
		kind, title, body := transition(sample.Kind, previous.value, *sample.Value)
		if kind == "" {
			continue
		}
		payload := makePayload(kind, "victron", key, title, body, now, now)
		if err := p.store.Enqueue(payload, p.epoch, false); err != nil {
			return err
		}
	}
	if ob.Complete {
		for key := range p.baseline {
			if !seenSamples[key] {
				delete(p.baseline, key)
			}
		}
	}
	return nil
}
func transition(kind string, before, after float64) (string, string, string) {
	switch kind {
	case "ev":
		if (before > 10) == (after > 10) {
			break
		}
		if after > 10 {
			return "ev", "EV charging started", "The charger is now delivering power."
		}
		return "ev", "EV charging stopped", "The charger is no longer delivering power."
	case "pump", "valve":
		if (before != 0 && before != 1) || (after != 0 && after != 1) || before == after {
			break
		}
		if kind == "pump" {
			if after == 1 {
				return "water", "Water pump on", "The pump switched on."
			}
			return "water", "Water pump off", "The pump switched off."
		}
		if after == 1 {
			return "water", "Water valve open", "The water valve opened."
		}
		return "water", "Water valve closed", "The water valve closed."
	case "soc":
		if before > 20 && before <= 100 && after >= 0 && after <= 20 {
			return "lowBattery", "Battery charge low", "The measured battery state of charge reached 20% or lower."
		}
	}
	return "", "", ""
}
