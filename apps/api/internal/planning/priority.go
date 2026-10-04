package planning

import (
	"math"

	"waypoint.lk/api/internal/domain"
)

// Ordering follows the documented prioritisation policy
// (docs/prioritisation-policy.md) exactly. It is not a numeric score: it is a
// deterministic lexicographic key applied only after hard feasibility, so a
// hard constraint can never be relaxed by a soft preference.
//
// The policy order is:
//
//  1. never defer an outlet deferred yesterday;
//  2. protect outlets unserved for seven or more days;
//  3. chilled/frozen before ambient;
//  4. narrower delivery windows before wide ones;
//  5. among equals, the largest order that still fits;
//  6. Fresh before Style/Tech (Style and Tech are the deliveries that tolerate
//     moving when something must move).
//
// Every step falls through to a stable identifier tie-break, so a run is
// reproducible from its input.

// lessOrder reports whether a should be placed before b under the policy.
func lessOrder(a, b Order) bool {
	// 1. previously deferred outlet first.
	if a.DeferredYesterday != b.DeferredYesterday {
		return a.DeferredYesterday
	}

	// 2. outlets unserved seven or more days first; then the longer-ignored.
	aStarved := a.DaysSinceLastServed >= 7
	bStarved := b.DaysSinceLastServed >= 7
	if aStarved != bStarved {
		return aStarved
	}
	if a.DaysSinceLastServed != b.DaysSinceLastServed {
		return a.DaysSinceLastServed > b.DaysSinceLastServed
	}

	// 3. chilled/frozen before ambient.
	aCold := a.TempRequirement.RequiresReefer()
	bCold := b.TempRequirement.RequiresReefer()
	if aCold != bCold {
		return aCold
	}

	// 4. narrower window first.
	aw, bw := windowWidthMin(a), windowWidthMin(b)
	if aw != bw {
		return aw < bw
	}

	// 5. larger order first (weight, then volume).
	if a.TotalWeightKg != b.TotalWeightKg {
		return a.TotalWeightKg > b.TotalWeightKg
	}
	if a.TotalVolumeM3 != b.TotalVolumeM3 {
		return a.TotalVolumeM3 > b.TotalVolumeM3
	}

	// 6. Fresh before Style/Tech.
	if brandRank(a.Brand) != brandRank(b.Brand) {
		return brandRank(a.Brand) < brandRank(b.Brand)
	}

	// Deterministic identity tie-break.
	if a.OutletID != b.OutletID {
		return a.OutletID < b.OutletID
	}
	if a.OrderNumber != b.OrderNumber {
		return a.OrderNumber < b.OrderNumber
	}
	return a.Sequence < b.Sequence
}

// windowWidthMin is the length of an order's delivery window in minutes. A
// missing or malformed window is treated as the widest possible, so it sorts
// last among rule 4 rather than being guessed at.
func windowWidthMin(o Order) int {
	open, err := ParseClock(o.WindowOpen)
	if err != nil {
		return math.MaxInt
	}
	close, err := ParseClock(o.WindowClose)
	if err != nil || close < open {
		return math.MaxInt
	}
	return close - open
}

// brandRank orders brands for rule 6: Fresh is protected, Style and Tech share
// the lower rank because either may move.
func brandRank(b domain.Brand) int {
	if b == domain.BrandFresh {
		return 0
	}
	return 1
}

// lessGroup compares two compatible (depot, brand, district) groups by their
// prioritised order lists, lexicographically. Both lists must already be sorted
// by lessOrder. The key is the final, stable tie-break.
func lessGroup(a, b []Order, keyA, keyB string) bool {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if lessOrder(a[i], b[i]) {
			return true
		}
		if lessOrder(b[i], a[i]) {
			return false
		}
	}
	return keyA < keyB
}
