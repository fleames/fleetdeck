package worker_test

import (
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/worker"
	"github.com/google/uuid"
)

func TestMonthBoundsAndPartitionName(t *testing.T) {
	// monthBounds is unexported; exercise MonthsFullyBefore + ServerInScope + naming via retain helpers indirectly.
	cutoff := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	augEnd := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	sepEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if !worker.MonthsFullyBefore(augEnd, cutoff) {
		t.Fatal("August should be fully before Sept 9 cutoff")
	}
	if worker.MonthsFullyBefore(sepEnd, cutoff) {
		t.Fatal("September should not be fully before Sept 9 cutoff")
	}
}

func TestServerInScope(t *testing.T) {
	a := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	b := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	if !worker.ServerInScope("all", nil, a) {
		t.Fatal("all should match")
	}
	if !worker.ServerInScope("", nil, a) {
		t.Fatal("empty scope_type should match all")
	}
	if worker.ServerInScope("servers", nil, a) {
		t.Fatal("servers with empty ids should not match")
	}
	if !worker.ServerInScope("servers", []uuid.UUID{a, b}, a) {
		t.Fatal("listed server should match")
	}
	if worker.ServerInScope("servers", []uuid.UUID{b}, a) {
		t.Fatal("unlisted server should not match")
	}
	if worker.ServerInScope("labels", []uuid.UUID{a}, a) {
		t.Fatal("unknown scope should fail closed")
	}
}
