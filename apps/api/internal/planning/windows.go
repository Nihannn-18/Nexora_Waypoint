package planning

import (
	"fmt"

	"waypoint.lk/api/internal/domain"
)

// Fresh and Style/Tech trip-1 departures are the team's documented operational
// assumptions (CLAUDE.md §5): Fresh loading 03:30, Fresh trip 1 departs 03:45;
// Style/Tech trip 1 departs 09:00. Trip 2 adds a 20-minute reload to the base.
// These drive arrival checks only; the official daily budget counts trip minutes
// with no return journey.
const (
	freshTrip1DepartureMin  = 3*60 + 45 // 03:45
	styleTechTrip1Departure = 9 * 60    // 09:00
	reloadMin               = 20
	freshCompletionDeadline = "08:00"
)

// tripDepartureMin is the departure time, in minutes since midnight, for a trip.
func tripDepartureMin(brand domain.Brand, tripNo int) int {
	base := freshTrip1DepartureMin
	if brand != domain.BrandFresh {
		base = styleTechTrip1Departure
	}
	if tripNo >= 2 {
		base += reloadMin
	}
	return base
}

// arrivalsFor walks the trip's stops in order and returns the planned arrival at
// each stop as "HH:mm". The outbound leg is counted once at the start; each
// subsequent stop adds an inter-stop leg. A stop's handling time and any wait
// for its window to open delay the next departure, so the walk cascades.
func arrivalsFor(in Input, c Candidate, travel Travel) []string {
	if len(c.Orders) == 0 {
		return nil
	}
	brand := c.Orders[0].Brand
	clock := tripDepartureMin(brand, c.TripNo) + travel.DepotToDistrictMin

	arrivals := make([]string, 0, len(c.Orders))
	for i, o := range c.Orders {
		if i > 0 {
			clock += travel.InterStopMin
		}
		arrival := FormatClock(clock)
		arrivals = append(arrivals, arrival)

		if assess, err := AssessWindow(arrival, o.WindowOpen, o.WindowClose); err == nil && assess.WaitingMin > 0 {
			clock += assess.WaitingMin
		}
		clock += in.ServiceAllowances[string(brand)+"|"+dockKey(in, o)]
	}
	return arrivals
}

// compareClock compares two "HH:mm" times, returning -1, 0 or +1.
func compareClock(a, b string) (int, error) {
	am, err := ParseClock(a)
	if err != nil {
		return 0, err
	}
	bm, err := ParseClock(b)
	if err != nil {
		return 0, err
	}
	switch {
	case am < bm:
		return -1, nil
	case am > bm:
		return 1, nil
	default:
		return 0, nil
	}
}

// checkStopWindows records delivery-window and mall-window failures for one
// stop. It never re-checks the Fresh 08:00 rule, which is a completion deadline
// applied once across the whole trip.
func checkStopWindows(v *TripVerdict, o Order, arrival string) {
	if o.WindowClose != "" {
		if assess, err := AssessWindow(arrival, o.WindowOpen, o.WindowClose); err == nil && assess.Late {
			fail(v, domain.ConstraintWindowMissed,
				fmt.Sprintf("arrival %s > close %s", arrival, o.WindowClose),
				fmt.Sprintf("%s would be reached at %s, after its %s window closes.", o.OutletID, arrival, o.WindowClose))
		}
	}
	if o.MallWindowOpen != "" && o.MallWindowClose != "" {
		if assess, err := AssessWindow(arrival, o.MallWindowOpen, o.MallWindowClose); err == nil && assess.Late {
			fail(v, domain.ConstraintMallWindowMissed,
				fmt.Sprintf("arrival %s > mall close %s", arrival, o.MallWindowClose),
				fmt.Sprintf("%s is a mall dock; %s is outside its %s-%s access window.",
					o.OutletID, arrival, o.MallWindowOpen, o.MallWindowClose))
		}
	}
}
