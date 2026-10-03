package clock

import (
	"testing"
	"time"
)

// manual is a Clock the test moves by hand.
type manual struct{ t time.Time }

func (m *manual) Now() time.Time { return m.t }

func colombo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestDemoStartsAtStartAndTicksWithBase(t *testing.T) {
	loc := colombo(t)
	start := time.Date(2026, 9, 25, 15, 40, 0, 0, loc)
	base := &manual{t: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)}

	demo := NewDemo(start, base)
	if got := demo.Now(); !got.Equal(start) {
		t.Fatalf("Now() at creation = %v, want %v", got, start)
	}

	base.t = base.t.Add(25 * time.Minute)
	want := time.Date(2026, 9, 25, 16, 5, 0, 0, loc)
	if got := demo.Now(); !got.Equal(want) {
		t.Fatalf("Now() after 25m = %v, want %v", got, want)
	}
	if got := demo.Now().Location(); got != loc {
		t.Fatalf("location = %v, want %v", got, loc)
	}
}

func TestNewSelectsClockFromDemoMode(t *testing.T) {
	loc := colombo(t)
	start := time.Date(2026, 9, 25, 15, 40, 0, 0, loc)

	t.Run("demo mode starts at the demo instant", func(t *testing.T) {
		got := New(true, start, loc).Now()
		if got.Before(start) || got.Sub(start) > time.Minute {
			t.Fatalf("Now() = %v, want just after %v", got, start)
		}
		if got.Location() != loc {
			t.Fatalf("location = %v, want %v", got.Location(), loc)
		}
	})

	t.Run("otherwise the wall clock in the business zone", func(t *testing.T) {
		before := time.Now()
		got := New(false, start, loc).Now()
		if got.Before(before) || got.Sub(before) > time.Minute {
			t.Fatalf("Now() = %v, want the wall clock (%v)", got, before)
		}
		if got.Location() != loc {
			t.Fatalf("location = %v, want %v", got.Location(), loc)
		}
	})
}
