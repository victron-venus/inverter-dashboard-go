package push

import (
	"testing"
	"time"
)

func testProcessor(t *testing.T) (*Store, *Processor, time.Time) {
	t.Helper()
	s := testStore(t)
	_ = s.Register(validTestSubscription(t), DefaultPreferences())
	p := NewProcessor(s)
	now := time.Now().UTC()
	if err := p.Reset("gateway", true, now); err != nil {
		t.Fatal(err)
	}
	return s, p, now
}
func native(id string, at time.Time) Native {
	return Native{ID: id, Source: "victron", Level: "warning", Title: "Alarm", Body: "Device", Timestamp: at}
}
func TestProcessorPrimesReplayUnknownHydrationAndFreshRecurrence(t *testing.T) {
	s, p, now := testProcessor(t)
	unknown := native("slot", time.Time{})
	_ = p.Observe(Observation{Notifications: []Native{native("old", now.Add(-75*time.Minute)), unknown}}, now)
	_ = p.Observe(Observation{Notifications: []Native{native("slot", now), native("old", now.Add(-75*time.Minute))}}, now.Add(time.Second))
	if queueSize(s) != 0 {
		t.Fatal("initial/hydrated replay notified")
	}
	_ = p.Observe(Observation{Notifications: []Native{}}, now.Add(2*time.Second))
	_ = p.Observe(Observation{Notifications: []Native{native("slot", now.Add(3*time.Second))}}, now.Add(3*time.Second))
	if queueSize(s) != 1 {
		t.Fatal("fresh reused slot swallowed")
	}
	_ = p.Reset("gateway", false, now.Add(4*time.Second))
	if queueSize(s) != 0 {
		t.Fatal("disconnect kept pending")
	}
	_ = p.Reset("gateway", true, now.Add(5*time.Second))
	_ = p.Observe(Observation{Notifications: []Native{native("slot", now.Add(3*time.Second))}}, now.Add(5*time.Second))
	if queueSize(s) != 0 {
		t.Fatal("reconnect replay")
	}
}
func TestProcessorMQTTHydrationWindowAndTimeBounds(t *testing.T) {
	s, p, now := testProcessor(t)
	_ = p.Reset("mqtt", true, now)
	for _, at := range []time.Duration{0, 5 * time.Second} {
		_ = p.Observe(Observation{Notifications: []Native{native(at.String(), now.Add(at))}}, now.Add(at))
	}
	_ = p.Observe(Observation{Notifications: []Native{native("too-old", now.Add(-31*time.Second)), native("future", now.Add(time.Minute))}}, now.Add(11*time.Second))
	if queueSize(s) != 0 {
		t.Fatal("prime or source-age boundary bypass")
	}
	_ = p.Observe(Observation{Notifications: []Native{native("fresh", now.Add(12*time.Second))}}, now.Add(12*time.Second))
	if queueSize(s) != 1 {
		t.Fatal("fresh event missing")
	}
}
func sample(kind, id string, value float64) Sample { return Sample{Kind: kind, ID: id, Value: &value} }
func TestProcessorSyntheticFreshBaselineOnlyAndNoGrid(t *testing.T) {
	for _, kind := range []string{"pump", "valve", "ev", "soc"} {
		t.Run(kind, func(t *testing.T) {
			s, p, now := testProcessor(t)
			a, b := 0., 1.
			if kind == "ev" {
				b = 100
			}
			if kind == "soc" {
				a, b = 25, 19
			}
			_ = p.Observe(Observation{Samples: []Sample{sample(kind, "device", a)}, Retained: true}, now)
			_ = p.Observe(Observation{Samples: []Sample{sample(kind, "device", b)}}, now.Add(time.Second))
			if queueSize(s) != 0 {
				t.Fatal("retained baseline notified")
			}
			_ = p.Observe(Observation{Samples: []Sample{sample(kind, "device", a)}}, now.Add(2*time.Second))
			_ = p.Observe(Observation{Samples: []Sample{sample(kind, "device", b)}}, now.Add(3*time.Second))
			if queueSize(s) == 0 {
				t.Fatal("fresh transition missing")
			}
			_ = p.Reset("gateway", true, now.Add(4*time.Second))
			_ = p.Observe(Observation{Samples: []Sample{sample(kind, "device", a)}}, now.Add(5*time.Second))
			_ = p.Observe(Observation{Samples: []Sample{sample(kind, "device", b)}}, now.Add(36*time.Second))
			if queueSize(s) != 0 {
				t.Fatal("stale prior sample used")
			}
		})
	}
	s, p, now := testProcessor(t)
	_ = p.Observe(Observation{Samples: []Sample{sample("grid", "a", 500)}}, now)
	_ = p.Observe(Observation{Samples: []Sample{sample("grid", "a", 0)}}, now.Add(time.Second))
	if queueSize(s) != 0 {
		t.Fatal("grid inferred")
	}
}
func TestProcessorMissingCompleteSnapshotAndSelectionChangeReprime(t *testing.T) {
	s, p, now := testProcessor(t)
	_ = p.Observe(Observation{Samples: []Sample{sample("pump", "a", 0)}, Complete: true}, now)
	_ = p.Observe(Observation{Complete: true}, now.Add(time.Second))
	_ = p.Observe(Observation{Samples: []Sample{sample("pump", "a", 1)}}, now.Add(2*time.Second))
	if queueSize(s) != 0 {
		t.Fatal("unknown baseline survives")
	}
	_ = p.Observe(Observation{Samples: []Sample{sample("ev", "a", 0)}}, now)
	_ = p.Observe(Observation{Samples: []Sample{sample("ev", "b", 0)}}, now)
	_ = p.Observe(Observation{Samples: []Sample{sample("ev", "a", 100)}}, now.Add(time.Second))
	if queueSize(s) != 0 {
		t.Fatal("device switch reused baseline")
	}
}

func TestNewPostPrimeIncompleteEventWaitsThenNotifies(t *testing.T) {
	s, p, now := testProcessor(t)
	_ = p.Observe(Observation{Notifications: []Native{}}, now)
	_ = p.Observe(Observation{Notifications: []Native{native("new", time.Time{})}}, now.Add(time.Second))
	if queueSize(s) != 0 {
		t.Fatal("unknown event notified")
	}
	_ = p.Observe(Observation{Notifications: []Native{native("new", now.Add(2*time.Second))}}, now.Add(2*time.Second))
	if queueSize(s) != 1 {
		t.Fatal("new partial event was treated as old baseline hydration")
	}
}

func TestFirstMQTTEventAfterQuietHydrationWindowIsNew(t *testing.T) {
	s, p, now := testProcessor(t)
	_ = p.Reset("mqtt", true, now)
	_ = p.Observe(Observation{Notifications: []Native{native("first-new", now.Add(11*time.Second))}}, now.Add(11*time.Second))
	if queueSize(s) != 1 {
		t.Fatal("first event after a quiet start was silently primed")
	}
}

func TestUnknownBaselineMarkerIsConsumedByFirstValidOccurrence(t *testing.T) {
	s, p, now := testProcessor(t)
	_ = p.Observe(Observation{Notifications: []Native{native("slot", time.Time{})}}, now)
	first := native("slot", now.Add(time.Second))
	_ = p.Observe(Observation{Notifications: []Native{first}}, now.Add(time.Second))
	_ = p.Observe(Observation{Notifications: []Native{first}}, now.Add(2*time.Second))
	if queueSize(s) != 0 {
		t.Fatal("hydrated original occurrence notified")
	}
	_ = p.Observe(Observation{Notifications: []Native{native("slot", now.Add(3*time.Second))}}, now.Add(3*time.Second))
	if queueSize(s) != 1 {
		t.Fatal("same visible slot new occurrence suppressed")
	}
}
