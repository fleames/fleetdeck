package worker_test

import (
	"testing"

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
