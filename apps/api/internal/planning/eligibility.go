package planning

import (
	"time"

	"waypoint.lk/api/internal/domain"
)

// Eligibility reports whether an order may be planned on the planning date, and
// if not, the reason it must be reported (never silently dropped).
//
// The rules are the ones the repository already establishes, nothing more:
//
//   - the order is CONFIRMED (the queue is closed for planning at CONFIRMED;
//     PLACED orders are not yet committed, and any later status is already
//     allocated or finished);
//   - its requested delivery date is the date being planned;
//   - the planning date is an operating day.
type Eligibility struct {
	Eligible bool
	Reason   string
	// Constraint is set when the reason maps to a hard rule; empty for a
	// non-constraint reason (e.g. "not this run's delivery date").
	Constraint domain.ConstraintCode
}

// EligibleOrder applies the eligibility rules. It never mutates the order.
//
// An ineligible order is the caller's to handle: an order for a different date
// simply is not part of this run (not a deferral); a non-operating day is a
// run-level failure, reported per order as NON_OPERATING_DAY.
func EligibleOrder(o Order, planningDate time.Time, day CalendarDay) Eligibility {
	if !sameDate(day.Date, planningDate) {
		// Defensive: the caller should pass the requested day's calendar row.
		return Eligibility{Reason: "no calendar entry for the planning date"}
	}
	if !day.IsOperating {
		return Eligibility{
			Reason:     "the planning date is not an operating day",
			Constraint: domain.ConstraintNonOperatingDay,
		}
	}
	if !sameDate(o.RequestedDeliveryDate, planningDate) {
		return Eligibility{Reason: "not scheduled for this delivery date"}
	}
	return Eligibility{Eligible: true}
}

// sameDate compares two instants by calendar date in UTC, matching the DATE
// columns the seed and orders layers store.
func sameDate(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}
