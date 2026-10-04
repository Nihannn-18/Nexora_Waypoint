// Package clock is the API's only source of "now".
//
// Business code takes a Clock rather than calling time.Now(), because the
// seeded demo day (Task 2B S1, Fri 25 Sep → Sat 26 Sep 2026) is in the past:
// on the wall clock every countdown, cutoff and "today" would be wrong. With
// DEMO_MODE on, the API runs a Demo clock that starts at DEMO_CLOCK_START and
// ticks forward in real time from there (CLAUDE.md §6).
package clock

import (
	"sync"
	"time"
)

// Clock reports the current instant in the business timezone.
type Clock interface {
	Now() time.Time
}

// Settable is a Clock that can be moved to a new instant and keeps ticking from
// there. Only the Demo clock implements it: the wall clock is never moved.
type Settable interface {
	Clock
	Set(t time.Time)
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
// Set jumps it to a walkthrough stage (POST /demo/clock); it is safe for
// concurrent use because requests read it while a dispatcher may move it.
type Demo struct {
	mu     sync.RWMutex
	start  time.Time
	anchor time.Time
	base   Clock
}

// NewDemo returns a clock that reads start now and then advances as base does.
// The result is reported in start's location.
func NewDemo(start time.Time, base Clock) *Demo {
	return &Demo{start: start, anchor: base.Now(), base: base}
}

// Now returns start plus the time base has advanced since NewDemo or the last
// Set.
func (d *Demo) Now() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.start.Add(d.base.Now().Sub(d.anchor)).In(d.start.Location())
}

// Set moves the clock to t; it keeps ticking from there at base's rate. The
// result stays in the original start's location, so the business timezone
// never changes with a jump.
func (d *Demo) Set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.start = t.In(d.start.Location())
	d.anchor = d.base.Now()
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
