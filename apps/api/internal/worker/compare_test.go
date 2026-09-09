package worker_test

import (
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/worker"
)

func TestCompareOperators(t *testing.T) {
	cases := []struct {
		op       string
		value    float64
		thresh   float64
		expected bool
	}{
		{">", 91, 90, true},
		{">", 90, 90, false},
		{">=", 90, 90, true},
		{"<", 10, 85, true},
		{"==", 1, 1, true},
		{"??", 1, 1, false},
	}
	for _, c := range cases {
		if got := worker.CompareOp(c.op, c.value, c.thresh); got != c.expected {
			t.Fatalf("%s %v %v => %v want %v", c.op, c.value, c.thresh, got, c.expected)
		}
	}
}

func TestSustainedBreach(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	mk := func(agoSec int, v float64) (time.Time, float64) {
		return now.Add(-time.Duration(agoSec) * time.Second), v
	}

	t.Run("duration0_latest_only", func(t *testing.T) {
		ts, v := mk(5, 95)
		ok, latest := worker.SustainedBreach([]time.Time{ts}, []float64{v}, ">", 90, 0, now)
		if !ok || latest != 95 {
			t.Fatalf("got ok=%v latest=%v", ok, latest)
		}
	})

	t.Run("spike_rejected", func(t *testing.T) {
		// Only last 30s above threshold for a 300s rule
		var times []time.Time
		var vals []float64
		for i := 300; i >= 0; i -= 10 {
			ts, v := mk(i, 50)
			if i <= 30 {
				v = 95
			}
			times = append(times, ts)
			vals = append(vals, v)
		}
		ok, _ := worker.SustainedBreach(times, vals, ">", 90, 300, now)
		if ok {
			t.Fatal("short spike should not fire")
		}
	})

	t.Run("sustained_fires", func(t *testing.T) {
		var times []time.Time
		var vals []float64
		for i := 300; i >= 0; i -= 10 {
			ts, v := mk(i, 95)
			times = append(times, ts)
			vals = append(vals, v)
		}
		ok, latest := worker.SustainedBreach(times, vals, ">", 90, 300, now)
		if !ok || latest != 95 {
			t.Fatalf("sustained breach should fire, ok=%v latest=%v", ok, latest)
		}
	})

	t.Run("recovery_in_window", func(t *testing.T) {
		var times []time.Time
		var vals []float64
		for i := 300; i >= 0; i -= 10 {
			ts, v := mk(i, 95)
			if i == 60 {
				v = 10
			}
			times = append(times, ts)
			vals = append(vals, v)
		}
		ok, _ := worker.SustainedBreach(times, vals, ">", 90, 300, now)
		if ok {
			t.Fatal("dip below threshold should clear")
		}
	})
}

func TestCooldownActive(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * time.Minute)
	old := now.Add(-10 * time.Minute)
	if !worker.CooldownActive(&recent, 300, now) {
		t.Fatal("expected cooldown active")
	}
	if worker.CooldownActive(&old, 300, now) {
		t.Fatal("expected cooldown expired")
	}
	if worker.CooldownActive(nil, 300, now) {
		t.Fatal("nil resolved_at means no cooldown")
	}
	if worker.CooldownActive(&recent, 0, now) {
		t.Fatal("zero cooldown disabled")
	}
}
