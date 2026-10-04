package routes

import (
	"testing"
	"time"
)

// TestPlannedArrivalTSAnchorsToBusinessZone is the regression for the wall-clock
// arrival shift: the arrival is a business-time "HH:MM", so it must be stored as
// the instant that clock reads in the business zone, not as the same number of
// hours UTC. Reading it back with a session-timezone to_char must yield the same
// clock.
func TestPlannedArrivalTSAnchorsToBusinessZone(t *testing.T) {
	colombo, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatalf("load Asia/Colombo: %v", err)
	}

	leg := RouteLeg{PlannedArrival: "05:52"}
	got := plannedArrivalTS("2026-09-26", colombo, leg)
	ts, ok := got.(time.Time)
	if !ok {
		t.Fatalf("plannedArrivalTS returned %T, want time.Time", got)
	}
	if ts.Location() != colombo && ts.UTC().Format("15:04") != "00:22" {
		// Sanity: the instant must be 05:52 in Colombo, i.e. 00:22 UTC.
		t.Fatalf("instant %v does not represent 05:52 Colombo", ts)
	}
	if clock := ts.In(colombo).Format("15:04"); clock != "05:52" {
		t.Fatalf("business clock = %q, want 05:52", clock)
	}
	if utcClock := ts.UTC().Format("15:04"); utcClock != "00:22" {
		t.Fatalf("UTC clock = %q, want 00:22 (05:52 +05:30)", utcClock)
	}

	// Empty stays NULL.
	if got := plannedArrivalTS("2026-09-26", colombo, RouteLeg{}); got != nil {
		t.Fatalf("empty arrival = %v, want nil", got)
	}
	// A nil location falls back to UTC rather than panicking.
	legUTC := RouteLeg{PlannedArrival: "05:52"}
	if ts, ok := plannedArrivalTS("2026-09-26", nil, legUTC).(time.Time); !ok || ts.UTC().Format("15:04") != "05:52" {
		t.Fatalf("nil location must anchor to UTC 05:52, got %v", ts)
	}
}
