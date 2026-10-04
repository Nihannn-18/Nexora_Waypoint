package planning

import (
	"fmt"

	"waypoint.lk/api/internal/domain"
)

// TripVerdict is the outcome of checking one candidate trip against every hard
// rule. It lists every rule's verdict (passed and failed), matching the
// explainability the dispatcher's rule panel needs.
type TripVerdict struct {
	// OK is true only when no rule failed.
	OK bool `json:"ok"`
	// Results holds one entry per rule that was applied, passed or failed.
	Results []domain.ConstraintResult `json:"results"`
	// Violations lists the codes that failed, for quick display.
	Violations []domain.ConstraintCode `json:"violations"`
	// Trip is the computed metrics (time breakdown), useful even on failure.
	Trip PlannedTrip `json:"trip"`
	// Reason is a plain-language sentence naming the first binding rule.
	Reason string `json:"reason"`
}

func fail(verdict *TripVerdict, code domain.ConstraintCode, detail, reason string) {
	verdict.Results = append(verdict.Results, domain.ConstraintResult{Code: code, Passed: false, Detail: detail})
	verdict.Violations = append(verdict.Violations, code)
	if verdict.Reason == "" {
		verdict.Reason = reason
	}
	verdict.OK = false
}

func pass(verdict *TripVerdict, code domain.ConstraintCode, detail string) {
	verdict.Results = append(verdict.Results, domain.ConstraintResult{Code: code, Passed: true, Detail: detail})
}

// Candidate is a proposed set of orders for one vehicle and trip. It is checked
// as a whole; an order is never split, so the candidate is the assignment unit.
type Candidate struct {
	Vehicle Vehicle
	TripNo  int // 1 or 2
	Orders  []Order
	// DayBudget is what this vehicle has already committed on the planning date,
	// from trips 1..tripNo-1 of the same run.
	DayBudget VehicleDayBudget
	// WeeklyFuelUsedL is the vehicle's ISO-week fuel already consumed.
	WeeklyFuelUsedL float64
	// DepartureMin is the minutes-since-midnight the trip leaves the depot. The
	// engine supplies it from the documented departure assumption.
	DepartureMin int
}

// CheckTrip validates a candidate against every hard rule, returning the full
// verdict. It is pure: same inputs, same verdict. Whether a third trip is
// attempted is the engine's decision, which never passes TripNo other than 1|2.
//
// This function is the single feasibility authority for a proposed trip.
func CheckTrip(in Input, c Candidate) TripVerdict {
	v := TripVerdict{OK: true}

	// --- Trip number -------------------------------------------------------
	if c.TripNo != 1 && c.TripNo != 2 {
		fail(&v, domain.ConstraintTripNumberInvalid, "",
			"A vehicle may run at most two trips a day.")
	} else {
		pass(&v, domain.ConstraintTripNumberInvalid, fmt.Sprintf("trip %d", c.TripNo))
	}

	// --- Vehicle availability ---------------------------------------------
	if !c.Vehicle.Available {
		fail(&v, domain.ConstraintVehicleUnavailable, c.Vehicle.VehicleID,
			fmt.Sprintf("%s is not available on the planning date.", c.Vehicle.VehicleID))
	} else {
		pass(&v, domain.ConstraintVehicleUnavailable, "available")
	}

	// --- Depot match -------------------------------------------------------
	depotOK := true
	for _, o := range c.Orders {
		if o.DepotID != c.Vehicle.DepotID {
			depotOK = false
			fail(&v, domain.ConstraintDepotMismatch,
				fmt.Sprintf("%s (order depot %s) vs %s (vehicle depot %s)", o.OrderNumber, o.DepotID, c.Vehicle.VehicleID, c.Vehicle.DepotID),
				fmt.Sprintf("%s is not based at %s's depot.", c.Vehicle.VehicleID, o.OrderNumber))
			break
		}
	}
	if depotOK {
		pass(&v, domain.ConstraintDepotMismatch, c.Vehicle.DepotID)
	}

	// --- Brand + district homogeneity -------------------------------------
	brand := domain.BrandFresh
	district := ""
	mixed := false
	for i, o := range c.Orders {
		if i == 0 {
			brand, district = o.Brand, o.District
			continue
		}
		if o.Brand != brand || o.District != district {
			mixed = true
			break
		}
	}
	if mixed {
		fail(&v, domain.ConstraintBrandDistrictMix, "",
			"A trip serves one brand and one district only.")
	} else {
		pass(&v, domain.ConstraintBrandDistrictMix, fmt.Sprintf("%s / %s", brand, district))
	}

	// --- Capacity ----------------------------------------------------------
	var weight, volume float64
	for _, o := range c.Orders {
		weight += o.TotalWeightKg
		volume += o.TotalVolumeM3
	}
	if weight > c.Vehicle.WeightCapKg {
		fail(&v, domain.ConstraintWeightExceeded,
			fmt.Sprintf("%.2f / %.2f kg", weight, c.Vehicle.WeightCapKg),
			fmt.Sprintf("The trip weighs %.1f kg, over the %.1f kg capacity of %s.", weight, c.Vehicle.WeightCapKg, c.Vehicle.VehicleID))
	} else {
		pass(&v, domain.ConstraintWeightExceeded, fmt.Sprintf("%.2f / %.2f kg", weight, c.Vehicle.WeightCapKg))
	}
	if volume > c.Vehicle.VolumeCapM3 {
		fail(&v, domain.ConstraintVolumeExceeded,
			fmt.Sprintf("%.3f / %.3f m3", volume, c.Vehicle.VolumeCapM3),
			fmt.Sprintf("The trip occupies %.2f m3, over the %.2f m3 capacity of %s.", volume, c.Vehicle.VolumeCapM3, c.Vehicle.VehicleID))
	} else {
		pass(&v, domain.ConstraintVolumeExceeded, fmt.Sprintf("%.3f / %.3f m3", volume, c.Vehicle.VolumeCapM3))
	}

	// --- Temperature / reefer ---------------------------------------------
	reeferOK := true
	for _, o := range c.Orders {
		if !c.Vehicle.CanCarryTemp(o.TempRequirement) {
			reeferOK = false
			fail(&v, domain.ConstraintReeferRequired, c.Vehicle.VehicleID,
				fmt.Sprintf("%s requires refrigeration and %s is an ambient vehicle.", o.OrderNumber, c.Vehicle.VehicleID))
			break
		}
	}
	if reeferOK {
		pass(&v, domain.ConstraintReeferRequired, c.Vehicle.TempClass.String())
	}

	// --- Van-only access ---------------------------------------------------
	vanOK := true
	for _, o := range c.Orders {
		if o.ParkingConstraint == domain.ParkingVanOnly && c.Vehicle.Type != domain.VehicleVan {
			vanOK = false
			fail(&v, domain.ConstraintVanOnlyAccess, c.Vehicle.VehicleID,
				fmt.Sprintf("%s can only be reached by a van, and %s is a %s.", o.OutletID, c.Vehicle.VehicleID, c.Vehicle.Type))
			break
		}
	}
	if vanOK {
		pass(&v, domain.ConstraintVanOnlyAccess, "ok")
	}

	// --- Trip time (official formula) + arrivals --------------------------
	trip, arrivals := computeTrip(in, c, weight, volume)
	v.Trip = trip

	budget := CheckTimeBudget(brand, trip.TotalTripMin, c.DayBudget)
	if brand == domain.BrandFresh {
		if !budget.Fits {
			fail(&v, domain.ConstraintFreshTimeBudget,
				fmt.Sprintf("%d / %d min", c.DayBudget.FreshMinutesUsed+trip.TotalTripMin, FreshTimeBudgetMin),
				fmt.Sprintf("Fresh minutes for %s would reach %d, over the %d-minute daily budget.",
					c.Vehicle.VehicleID, c.DayBudget.FreshMinutesUsed+trip.TotalTripMin, FreshTimeBudgetMin))
		} else {
			pass(&v, domain.ConstraintFreshTimeBudget, fmt.Sprintf("%d / %d min", c.DayBudget.FreshMinutesUsed+trip.TotalTripMin, FreshTimeBudgetMin))
		}
	} else {
		if !budget.Fits {
			fail(&v, domain.ConstraintStyleTechBudget,
				fmt.Sprintf("%d / %d min", c.DayBudget.StyleTechMinutesUsed+trip.TotalTripMin, StyleTechTimeBudgetMin),
				fmt.Sprintf("Style+Tech minutes for %s would reach %d, over the shared %d-minute daily budget.",
					c.Vehicle.VehicleID, c.DayBudget.StyleTechMinutesUsed+trip.TotalTripMin, StyleTechTimeBudgetMin))
		} else {
			pass(&v, domain.ConstraintStyleTechBudget, fmt.Sprintf("%d / %d min", c.DayBudget.StyleTechMinutesUsed+trip.TotalTripMin, StyleTechTimeBudgetMin))
		}
	}

	// --- Delivery + mall windows ------------------------------------------
	for i, o := range c.Orders {
		if i < len(arrivals) {
			checkStopWindows(&v, o, arrivals[i])
		}
	}
	pass(&v, domain.ConstraintWindowMissed, "windows honoured")
	pass(&v, domain.ConstraintMallWindowMissed, "mall windows honoured")

	// --- Fresh must complete before 08:00 ---------------------------------
	if brand == domain.BrandFresh && trip.LastArrival != "" {
		if cmp, err := compareClock(trip.LastArrival, freshCompletionDeadline); err == nil && cmp > 0 {
			fail(&v, domain.ConstraintWindowMissed, trip.LastArrival,
				fmt.Sprintf("Fresh must complete before 08:00; the last stop is planned for %s.", trip.LastArrival))
		}
	}

	// --- Fuel --------------------------------------------------------------
	// The weekly quota is a hard constraint whenever the vehicle has one
	// (vehicle.weekly_fuel_quota_l > 0). Distance uses the agreed reference
	// approximation (outbound + inter-stop; no return), and fuel is the weekly
	// ledger usage plus this run's already-committed trips plus this candidate.
	//
	// A non-positive km_per_l makes the fuel division unsafe: the candidate is
	// rejected with FUEL_EFFICIENCY_INVALID, never divided by zero and never
	// silently skipped. FUEL_QUOTA_EXCEEDED is reserved for an actual quota
	// breach.
	if quota, ok := in.FuelQuotaL[c.Vehicle.VehicleID]; ok && quota > 0 {
		if c.Vehicle.KmPerL <= 0 {
			fail(&v, domain.ConstraintFuelEfficiencyInvalid, c.Vehicle.VehicleID,
				fmt.Sprintf("%s has no usable km/l figure, so its fuel use cannot be calculated.", c.Vehicle.VehicleID))
		} else {
			projected, err := tripFuelLitres(in, trip, c.Vehicle)
			switch {
			case err != nil:
				fail(&v, domain.ConstraintFuelEfficiencyInvalid, c.Vehicle.VehicleID,
					fmt.Sprintf("%s has no usable km/l figure, so its fuel use cannot be calculated.", c.Vehicle.VehicleID))
			case !WithinWeeklyFuelQuota(c.WeeklyFuelUsedL, projected, quota):
				fail(&v, domain.ConstraintFuelQuotaExceeded,
					fmt.Sprintf("%.1f + %.1f / %.1f L", c.WeeklyFuelUsedL, projected, quota),
					fmt.Sprintf("Projected weekly fuel for %s would exceed its %.0f L quota.", c.Vehicle.VehicleID, quota))
			default:
				pass(&v, domain.ConstraintFuelQuotaExceeded, fmt.Sprintf("%.1f + %.1f / %.1f L", c.WeeklyFuelUsedL, projected, quota))
			}
		}
	}

	return v
}

// computeTrip fills a PlannedTrip's metrics for a candidate, using the official
// formula via CalculateTripTime, and returns the per-stop arrivals.
func computeTrip(in Input, c Candidate, weight, volume float64) (PlannedTrip, []string) {
	brand := domain.BrandFresh
	district := ""
	if len(c.Orders) > 0 {
		brand, district = c.Orders[0].Brand, c.Orders[0].District
	}

	travel := in.Travel[c.Vehicle.DepotID+"|"+district]

	allowances := make([]int, 0, len(c.Orders))
	for _, o := range c.Orders {
		allowances = append(allowances, in.ServiceAllowances[string(brand)+"|"+dockKey(in, o)])
	}
	breakdown := CalculateTripTime(TripTimeInput{
		OutboundFreeflowMin:  travel.DepotToDistrictMin,
		InterStopFreeflowMin: travel.InterStopMin,
		ServiceAllowancesMin: allowances,
	})

	arrivals := arrivalsFor(in, c, travel)
	last := ""
	if len(arrivals) > 0 {
		last = arrivals[len(arrivals)-1]
	}

	trip := PlannedTrip{
		VehicleID:    c.Vehicle.VehicleID,
		TripNo:       c.TripNo,
		Brand:        brand,
		District:     district,
		DepotID:      c.Vehicle.DepotID,
		OrderIDs:     orderIDs(c.Orders),
		WeightKg:     weight,
		VolumeM3:     volume,
		OutboundMin:  breakdown.OutboundMin,
		InterStopMin: breakdown.InterStopMin,
		HandlingMin:  breakdown.HandlingMin,
		TotalTripMin: breakdown.TotalTripMin,
		LastArrival:  last,
	}
	return trip, arrivals
}

// RouteStopSchedule is one stop's planned arrival and handling time for a
// confirmed route. ArrivalMin is minutes since midnight (business timezone; the
// route date supplies the day); ServiceMin is the handling allowance the official
// formula applies to the stop.
type RouteStopSchedule struct {
	ArrivalMin int
	ServiceMin int
}

// ScheduleRoute reuses the authoritative arrival walk to compute each stop's
// planned arrival and service time for a confirmed route, in the given order.
// Confirmation persists these so the loader and driver read a stored fact rather
// than re-deriving the formula. It performs no feasibility checks; it is the same
// arithmetic the trip-time and window rules use.
func ScheduleRoute(in Input, vehicle Vehicle, tripNo int, orders []Order) ([]RouteStopSchedule, error) {
	if len(orders) == 0 {
		return nil, nil
	}
	cand := Candidate{Vehicle: vehicle, TripNo: tripNo, Orders: orders}
	brand := orders[0].Brand
	district := orders[0].District
	travel := in.Travel[vehicle.DepotID+"|"+district]

	// The arrival walk returns "HH:MM" strings; convert to minutes since
	// midnight so the caller can place them on the route date in its timezone.
	arrivals := arrivalsFor(in, cand, travel)
	out := make([]RouteStopSchedule, 0, len(orders))
	for i, o := range orders {
		svc := in.ServiceAllowances[string(brand)+"|"+dockKey(in, o)]
		arrivalMin := 0
		if i < len(arrivals) {
			m, err := ParseClock(arrivals[i])
			if err != nil {
				return nil, err
			}
			arrivalMin = m
		}
		out = append(out, RouteStopSchedule{ArrivalMin: arrivalMin, ServiceMin: svc})
	}
	return out, nil
}

// orderIDs returns order ids in the candidate's (already stable) order.
func orderIDs(orders []Order) []string {
	out := make([]string, len(orders))
	for i, o := range orders {
		out[i] = o.OrderID
	}
	return out
}

// dockKey returns the dock type string for an order's outlet, defaulting to
// STREET when the outlet is unknown, so a missing outlet cannot panic a run.
func dockKey(in Input, o Order) string {
	if out, ok := in.Outlets[o.OutletID]; ok {
		return string(out.DockType)
	}
	return string(domain.DockStreet)
}

// tripFuelLitres projects the fuel a trip consumes, using the agreed
// reference-based distance: outbound + inter-stop legs only, no return.
func tripFuelLitres(in Input, trip PlannedTrip, v Vehicle) (float64, error) {
	travel := in.Travel[v.DepotID+"|"+trip.District]
	n := len(trip.OrderIDs)
	distance := travel.DepotToDistrictKm + travel.InterStopKm*float64(max(n-1, 0))
	if v.KmPerL <= 0 {
		return 0, ErrInvalidEfficiency
	}
	return EstimateFuelLitres(distance, v.KmPerL)
}
