package planning

import (
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// These vectors are mirrored in libs/shared-types/src/lib/trip-time.spec.ts.
// The two implementations must agree digit for digit. If you change a number
// here, change it there in the same commit.

func TestCalculateTripTime(t *testing.T) {
	tests := []struct {
		name string
		in   TripTimeInput
		want TripTimeBreakdown
	}{
		{
			// The worked example printed in the Challenge Booklet: a three-order
			// Fresh trip to Gampaha totals 101 minutes.
			name: "booklet worked example",
			in: TripTimeInput{
				OutboundFreeflowMin:  37,
				InterStopFreeflowMin: 9,
				ServiceAllowancesMin: []int{15, 15, 16},
			},
			want: TripTimeBreakdown{
				OrderCount:   3,
				OutboundMin:  37,
				InterStopMin: 18,
				HandlingMin:  46,
				TotalTripMin: 101,
			},
		},
		{
			name: "one-order trip has zero inter-stop journeys",
			in: TripTimeInput{
				OutboundFreeflowMin:  37,
				InterStopFreeflowMin: 9,
				ServiceAllowancesMin: []int{15},
			},
			want: TripTimeBreakdown{
				OrderCount:   1,
				OutboundMin:  37,
				InterStopMin: 0,
				HandlingMin:  15,
				TotalTripMin: 52,
			},
		},
		{
			name: "three-order trip has two inter-stop journeys",
			in: TripTimeInput{
				OutboundFreeflowMin:  20,
				InterStopFreeflowMin: 10,
				ServiceAllowancesMin: []int{5, 5, 5},
			},
			want: TripTimeBreakdown{
				OrderCount:   3,
				OutboundMin:  20,
				InterStopMin: 20,
				HandlingMin:  15,
				TotalTripMin: 55,
			},
		},
		{
			name: "empty trip consumes nothing",
			in: TripTimeInput{
				OutboundFreeflowMin:  37,
				InterStopFreeflowMin: 9,
				ServiceAllowancesMin: nil,
			},
			want: TripTimeBreakdown{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateTripTime(tc.in)
			if got != tc.want {
				t.Errorf("CalculateTripTime() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCheckTimeBudget(t *testing.T) {
	tests := []struct {
		name     string
		brand    domain.Brand
		trip     int
		used     VehicleDayBudget
		wantFits bool
		wantOver int
	}{
		{
			name:     "Fresh exactly at 270 is accepted",
			brand:    domain.BrandFresh,
			trip:     70,
			used:     VehicleDayBudget{FreshMinutesUsed: 200},
			wantFits: true,
		},
		{
			name:     "Fresh at 271 is rejected",
			brand:    domain.BrandFresh,
			trip:     71,
			used:     VehicleDayBudget{FreshMinutesUsed: 200},
			wantFits: false,
			wantOver: 1,
		},
		{
			name:     "Style+Tech exactly at 480 is accepted",
			brand:    domain.BrandStyle,
			trip:     80,
			used:     VehicleDayBudget{StyleTechMinutesUsed: 400},
			wantFits: true,
		},
		{
			name:     "Style+Tech at 481 is rejected",
			brand:    domain.BrandTech,
			trip:     81,
			used:     VehicleDayBudget{StyleTechMinutesUsed: 400},
			wantFits: false,
			wantOver: 1,
		},
		{
			// The shared pool is the easy thing to get wrong: Style minutes
			// already spent must reduce what Tech can still do.
			name:     "Style and Tech draw on one shared pool",
			brand:    domain.BrandTech,
			trip:     200,
			used:     VehicleDayBudget{StyleTechMinutesUsed: 300},
			wantFits: false,
			wantOver: 20,
		},
		{
			name:     "Fresh pool is independent of the Style/Tech pool",
			brand:    domain.BrandFresh,
			trip:     200,
			used:     VehicleDayBudget{StyleTechMinutesUsed: 470},
			wantFits: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckTimeBudget(tc.brand, tc.trip, tc.used)
			if got.Fits != tc.wantFits {
				t.Errorf("Fits = %v, want %v (%+v)", got.Fits, tc.wantFits, got)
			}
			if got.OverByMin != tc.wantOver {
				t.Errorf("OverByMin = %d, want %d", got.OverByMin, tc.wantOver)
			}
		})
	}
}

func TestAssessWindow(t *testing.T) {
	tests := []struct {
		name        string
		arrival     string
		open        string
		close       string
		wantWaiting int
		wantStart   string
		wantLate    bool
		wantLateBy  int
	}{
		{"waits for a window not yet open", "05:40", "06:00", "08:00", 20, "06:00", false, 0},
		{"starts immediately inside the window", "07:15", "06:00", "08:00", 0, "07:15", false, 0},
		{"exactly at close is not late", "08:00", "06:00", "08:00", 0, "08:00", false, 0},
		{"one minute after close is late", "08:01", "06:00", "08:00", 0, "08:01", true, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AssessWindow(tc.arrival, tc.open, tc.close)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.WaitingMin != tc.wantWaiting {
				t.Errorf("WaitingMin = %d, want %d", got.WaitingMin, tc.wantWaiting)
			}
			if got.ServiceStart != tc.wantStart {
				t.Errorf("ServiceStart = %q, want %q", got.ServiceStart, tc.wantStart)
			}
			if got.Late != tc.wantLate {
				t.Errorf("Late = %v, want %v", got.Late, tc.wantLate)
			}
			if got.LateByMin != tc.wantLateBy {
				t.Errorf("LateByMin = %d, want %d", got.LateByMin, tc.wantLateBy)
			}
		})
	}
}

func TestParseClockRejectsRubbish(t *testing.T) {
	for _, bad := range []string{"not-a-time", "25:00", "07:61", ""} {
		if _, err := ParseClock(bad); !errors.Is(err, ErrInvalidClock) {
			t.Errorf("ParseClock(%q) error = %v, want ErrInvalidClock", bad, err)
		}
	}
}

func TestFormatClockPads(t *testing.T) {
	if got := FormatClock(5); got != "00:05" {
		t.Errorf("FormatClock(5) = %q, want \"00:05\"", got)
	}
	if got := FormatClock(454); got != "07:34" {
		t.Errorf("FormatClock(454) = %q, want \"07:34\"", got)
	}
}

func TestFuel(t *testing.T) {
	litres, err := EstimateFuelLitres(120, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if litres != 15 {
		t.Errorf("EstimateFuelLitres(120, 8) = %v, want 15", litres)
	}

	if _, err := EstimateFuelLitres(100, 0); !errors.Is(err, ErrInvalidEfficiency) {
		t.Errorf("zero efficiency should be refused, got %v", err)
	}

	if !WithinWeeklyFuelQuota(90, 10, 100) {
		t.Error("usage exactly at quota should be allowed")
	}
	if WithinWeeklyFuelQuota(90, 11, 100) {
		t.Error("usage over quota should be rejected")
	}
}

func TestRequiresReefer(t *testing.T) {
	if !domain.TempChilled.RequiresReefer() || !domain.TempFrozen.RequiresReefer() {
		t.Error("chilled and frozen must require a reefer")
	}
	if domain.TempAmbient.RequiresReefer() {
		t.Error("ambient must not require a reefer")
	}
}
