// Package planning implements the deterministic trip-time arithmetic.
//
// This is the authoritative implementation. The TypeScript mirror in
// libs/shared-types exists only so the dispatcher's board can update its meters
// without a round-trip; the server revalidates every plan regardless.
//
// The formula is fixed by the Challenge Booklet and must not be replaced by a
// routing API:
//
//	outbound_min   = depot_to_district_freeflow_min            (counted once)
//	inter_stop_min = inter_stop_freeflow_min × max(n-1, 0)
//	handling_min   = Σ service_allowance(brand, outlet.dock_type)
//	trip_minutes   = outbound + inter_stop + handling
//
// The return journey to the depot is NOT counted.
package planning

import (
	"errors"
	"fmt"

	"waypoint.lk/api/internal/domain"
)

// Official per-vehicle, per-day minute budgets.
const (
	// FreshTimeBudgetMin is Fresh's own daily budget per vehicle.
	FreshTimeBudgetMin = 270
	// StyleTechTimeBudgetMin is shared by Style and Tech together, not each.
	StyleTechTimeBudgetMin = 480
	// MaxTripsPerVehiclePerDay caps a vehicle at two outbound runs.
	MaxTripsPerVehiclePerDay = 2
)

// TripTimeInput is the reference data for one trip, read from district_travel.csv
// and service_allowance.csv.
type TripTimeInput struct {
	// OutboundFreeflowMin is depot_to_district_freeflow_min for this depot/district.
	OutboundFreeflowMin int
	// InterStopFreeflowMin is inter_stop_freeflow_min for this district.
	InterStopFreeflowMin int
	// ServiceAllowancesMin holds one allowance per order, by brand + dock type.
	ServiceAllowancesMin []int
}

// TripTimeBreakdown is the component-by-component result, kept separate so the
// dispatcher can see which part of a trip consumed the budget.
type TripTimeBreakdown struct {
	OrderCount   int `json:"orderCount"`
	OutboundMin  int `json:"outboundMin"`
	InterStopMin int `json:"interStopMin"`
	HandlingMin  int `json:"handlingMin"`
	TotalTripMin int `json:"totalTripMin"`
}

// CalculateTripTime applies the official formula.
//
// A trip with no orders costs nothing: a vehicle that was never dispatched must
// not consume any of its daily budget.
func CalculateTripTime(in TripTimeInput) TripTimeBreakdown {
	n := len(in.ServiceAllowancesMin)
	if n == 0 {
		return TripTimeBreakdown{}
	}

	interStop := 0
	if n > 1 {
		interStop = in.InterStopFreeflowMin * (n - 1)
	}

	handling := 0
	for _, min := range in.ServiceAllowancesMin {
		handling += min
	}

	return TripTimeBreakdown{
		OrderCount:   n,
		OutboundMin:  in.OutboundFreeflowMin,
		InterStopMin: interStop,
		HandlingMin:  handling,
		TotalTripMin: in.OutboundFreeflowMin + interStop + handling,
	}
}

// TimeBudgetForBrand returns the daily pool a brand draws on. Style and Tech
// share one pool; Fresh has its own.
func TimeBudgetForBrand(b domain.Brand) int {
	if b == domain.BrandFresh {
		return FreshTimeBudgetMin
	}
	return StyleTechTimeBudgetMin
}

// VehicleDayBudget is how much of each pool a vehicle has already committed today.
type VehicleDayBudget struct {
	FreshMinutesUsed     int
	StyleTechMinutesUsed int
}

// BudgetCheck reports whether a candidate trip fits, and by how much it misses.
type BudgetCheck struct {
	Fits         bool `json:"fits"`
	BudgetMin    int  `json:"budgetMin"`
	UsedMin      int  `json:"usedMin"`
	RemainingMin int  `json:"remainingMin"`
	// OverByMin is 0 when the trip fits.
	OverByMin int `json:"overByMin"`
}

// CheckTimeBudget tests a candidate trip against the vehicle's remaining budget.
// Exactly at budget is allowed; one minute over is not.
func CheckTimeBudget(b domain.Brand, tripMinutes int, used VehicleDayBudget) BudgetCheck {
	budget := TimeBudgetForBrand(b)

	usedMin := used.StyleTechMinutesUsed
	if b == domain.BrandFresh {
		usedMin = used.FreshMinutesUsed
	}

	projected := usedMin + tripMinutes

	return BudgetCheck{
		Fits:         projected <= budget,
		BudgetMin:    budget,
		UsedMin:      usedMin,
		RemainingMin: max(budget-usedMin, 0),
		OverByMin:    max(projected-budget, 0),
	}
}

// ---------------------------------------------------------------------------
// Delivery windows
// ---------------------------------------------------------------------------

// ErrInvalidClock is returned for a time that is not 24-hour HH:mm.
var ErrInvalidClock = errors.New("invalid clock time: expected HH:mm")

// ParseClock converts 24-hour "HH:mm" to minutes since midnight.
func ParseClock(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%2d:%2d", &h, &m); err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidClock, s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidClock, s)
	}
	return h*60 + m, nil
}

// FormatClock renders minutes since midnight as 24-hour "HH:mm".
func FormatClock(total int) string {
	wrapped := ((total % 1440) + 1440) % 1440
	return fmt.Sprintf("%02d:%02d", wrapped/60, wrapped%60)
}

// WindowAssessment is the outcome of arriving at an outlet at a given time.
type WindowAssessment struct {
	// ServiceStart is when handling actually begins: an early vehicle waits.
	ServiceStart string `json:"serviceStart"`
	WaitingMin   int    `json:"waitingMin"`
	// Late means arrival after the window CLOSES — the official definition,
	// shared with Datathon Task 1 so the app and the model agree.
	Late      bool `json:"late"`
	LateByMin int  `json:"lateByMin"`
}

// AssessWindow applies the official window rules: an early arrival waits until
// the window opens; lateness is arrival after it closes.
func AssessWindow(arrival, windowOpen, windowClose string) (WindowAssessment, error) {
	a, err := ParseClock(arrival)
	if err != nil {
		return WindowAssessment{}, err
	}
	open, err := ParseClock(windowOpen)
	if err != nil {
		return WindowAssessment{}, err
	}
	closesAt, err := ParseClock(windowClose)
	if err != nil {
		return WindowAssessment{}, err
	}

	waiting := 0
	if a < open {
		waiting = open - a
	}

	lateBy := 0
	if a > closesAt {
		lateBy = a - closesAt
	}

	return WindowAssessment{
		ServiceStart: FormatClock(max(a, open)),
		WaitingMin:   waiting,
		Late:         lateBy > 0,
		LateByMin:    lateBy,
	}, nil
}

// ---------------------------------------------------------------------------
// Fuel
// ---------------------------------------------------------------------------

// ErrInvalidEfficiency guards against a division by zero producing +Inf litres.
var ErrInvalidEfficiency = errors.New("kmPerL must be greater than zero")

// EstimateFuelLitres converts route distance into litres for quota accounting.
func EstimateFuelLitres(distanceKm, kmPerL float64) (float64, error) {
	if kmPerL <= 0 {
		return 0, ErrInvalidEfficiency
	}
	return distanceKm / kmPerL, nil
}

// WithinWeeklyFuelQuota reports whether adding projectedL keeps the vehicle
// inside its weekly allowance. Exactly at quota is allowed.
func WithinWeeklyFuelQuota(weeklyUsedL, projectedL, quotaL float64) bool {
	return weeklyUsedL+projectedL <= quotaL
}
