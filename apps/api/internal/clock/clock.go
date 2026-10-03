// Package clock is the API's only source of "now".
//
// Business code takes a Clock rather than calling time.Now(), because the
// seeded demo day (Task 2B S1, Fri 25 Sep → Sat 26 Sep 2026) is in the past:
// on the wall clock every countdown, cutoff and "today" would be wrong. With
// DEMO_MODE on, the API runs a Demo clock that starts at DEMO_CLOCK_START and
// ticks forward in real time from there (CLAUDE.md §6).
package clock

import "time"

// Clock reports the current instant in the business timezone.
type Clock interface {
	Now() time.Time
}

// System is the wall clock, expressed in the business timezone.
type System struct {
	loc *time.Location
}

// NewSystem returns the wall clock in loc.
func NewSystem(loc *time.Location) System { return System{loc: loc} }

// Now returns the current wall-clock time in the business timezone.
func (s System) Now() time.Time { return time.Now().In(s.loc) }

// Demo starts at a fixed instant and advances at the rate of an underlying
// clock, so a judge sees time pass normally from the seeded starting point.
type Demo struct {
	start  time.Time
	anchor time.Time
	base   Clock
}

// NewDemo returns a clock that reads start now and then advances as base does.
// The result is reported in start's location.
func NewDemo(start time.Time, base Clock) *Demo {
	return &Demo{start: start, anchor: base.Now(), base: base}
}

// Now returns start plus the time base has advanced since NewDemo.
func (d *Demo) Now() time.Time {
	return d.start.Add(d.base.Now().Sub(d.anchor)).In(d.start.Location())
}

// New selects the API clock: the Demo clock from demoStart when demoMode is on,
// otherwise the wall clock. Both report times in loc.
func New(demoMode bool, demoStart time.Time, loc *time.Location) Clock {
	system := NewSystem(loc)
	if !demoMode {
		return system
	}
	return NewDemo(demoStart.In(loc), system)
}
