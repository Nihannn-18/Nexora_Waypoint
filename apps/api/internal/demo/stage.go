// Package demo owns the judge-walkthrough controls that exist only under
// DEMO_MODE: jumping the API clock to a named stage of the seeded demo day and
// resetting the operational data back to the freshly seeded state
// (CLAUDE.md §6, "The demo clock").
//
// Nothing here is mounted when DEMO_MODE is off, so a real deployment has no
// way to move its clock or wipe its data.
package demo

import "time"

// Stage names one point in the judge walkthrough. The seeded demo day is the
// Task 2B S1 scenario: planning on Fri 25 Sep 2026, delivery on Sat 26 Sep.
type Stage string

const (
	// StageBeforeCutoff is Fri 15:40, twenty minutes before the 16:00 cutoff.
	StageBeforeCutoff Stage = "BEFORE_CUTOFF"
	// StageAfterCutoff is Fri 16:05: new orders roll to the next operating day.
	StageAfterCutoff Stage = "AFTER_CUTOFF"
	// StageLoading is Sat 03:30, while the loaders work the dock.
	StageLoading Stage = "LOADING"
	// StageOnRoute is Sat 05:00, when the first Fresh windows open.
	StageOnRoute Stage = "ON_ROUTE"
)

// stageTimes are wall-clock times in the business timezone, in walkthrough
// order. They are facts of the seeded scenario, not configuration.
var stageTimes = []struct {
	stage                   Stage
	year, month, day, h, mi int
}{
	{StageBeforeCutoff, 2026, 9, 25, 15, 40},
	{StageAfterCutoff, 2026, 9, 25, 16, 5},
	{StageLoading, 2026, 9, 26, 3, 30},
	{StageOnRoute, 2026, 9, 26, 5, 0},
}

// Stages lists every stage in walkthrough order.
func Stages() []Stage {
	out := make([]Stage, 0, len(stageTimes))
	for _, s := range stageTimes {
		out = append(out, s.stage)
	}
	return out
}

// At returns the instant a stage stands for in loc. ok is false for a stage
// this package does not know.
func (s Stage) At(loc *time.Location) (t time.Time, ok bool) {
	for _, st := range stageTimes {
		if st.stage == s {
			return time.Date(st.year, time.Month(st.month), st.day, st.h, st.mi, 0, 0, loc), true
		}
	}
	return time.Time{}, false
}
