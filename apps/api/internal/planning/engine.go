package planning

import (
	"sort"

	"waypoint.lk/api/internal/domain"
)

// Engine turns loaded inputs into a feasible, deterministic proposal. It holds
// no state and performs no I/O, so a run is a pure function of its Input.
//
// DESIGN — feasibility is the only filter on whether an order can be served;
// the documented prioritisation policy (docs/prioritisation-policy.md) decides
// the order in which feasible orders are considered, so that on a constrained
// day the right outlets are served and the right ones deferred. The policy is a
// deterministic lexicographic key (planning/priority.go), never a numeric score,
// and it can never override a hard rule. See lessOrder for the exact sequence.
//
// The algorithm:
//
//  1. Filter to eligible orders (CONFIRMED, for the planning date, operating day).
//  2. Group by (depot, brand, district) — a trip holds one brand and one
//     district only, so this is the finest compatible group.
//  3. Within a group, order by the documented prioritisation policy: previously
//     deferred, starved, chilled, narrow-window, largest, Fresh first, with a
//     stable identity tie-break. This ordering is not a feasibility rule.
//  4. Walk the group's available depot vehicles in vehicle-id order; greedily
//     extend the vehicle's open trip, else open trip 1, else trip 2.
//  5. Each candidate is checked by CheckTrip. An order that does not fit is left
//     for another vehicle or deferred. An order is never split.
//  6. Orders not placed are deferred with the binding constraint from the
//     best-effort candidate that failed.
type Engine struct{}

// New builds a planning engine.
func New() *Engine { return &Engine{} }

// runTrip is the engine's mutable working state for one trip.
type runTrip struct {
	vehicleID string
	tripNo    int
	brand     domain.Brand
	district  string
	depotID   string
	orders    []Order
	computed  PlannedTrip
}

// Plan runs the deterministic assignment over one depot's eligible orders.
func (e *Engine) Plan(in Input) Result {
	eligible, ineligible := partitionEligible(in)
	groups := groupOrders(eligible)

	budget := map[string]VehicleDayBudget{}
	committed := map[string]int{} // trips per vehicle
	openTrips := map[string]int{} // vehicleID -> index into runTrips, -1 if none
	// fuelAccrued is the litres committed by trips already placed in this run.
	// It is added to the vehicle's authoritative weekly ledger usage so trip 2 is
	// checked against base weekly usage + trip 1's fuel, and never against the
	// base alone.
	fuelAccrued := map[string]float64{}
	runTrips := []runTrip{}

	for _, key := range sortedGroupKeys(groups) {
		g := parseGroupKey(key)
		for _, o := range groups[key] {
			if !tryPlace(in, o, g, budget, committed, openTrips, fuelAccrued, &runTrips) {
				// No vehicle in this group could take it: defer with the best
				// explanation we can produce.
			}
		}
	}

	// Build final trips and deferrals.
	trips := make([]PlannedTrip, 0, len(runTrips))
	served := map[string]bool{}
	for _, rt := range runTrips {
		trips = append(trips, rt.computed)
		for _, o := range rt.orders {
			served[o.OrderID] = true
		}
	}
	sortTrips(trips)

	deferred := make([]DeferredOrder, 0)
	for _, o := range eligible {
		if served[o.OrderID] {
			continue
		}
		deferred = append(deferred, bestEffortDeferral(in, o, budget, committed, fuelAccrued))
	}
	deferred = append(deferred, ineligible...)
	sortDeferrals(deferred)

	return Result{Trips: trips, Deferred: deferred}
}

// tryPlace attempts to place one whole order onto a run trip. It returns true on
// success.
//
// Budget accounting: budget[v] is the vehicle's minutes already committed for
// the day, including the current contribution of its open trip. When we test an
// extension we first subtract the open trip's old minutes so the candidate's
// total is measured once, then re-accrue the new total. Accruing the delta (not
// the whole trip again) is what makes trip 2 see trip 1's final, possibly
// extended, minutes — the bug that let a Fresh vehicle exceed 270.
func tryPlace(in Input, o Order, g groupKey, budget map[string]VehicleDayBudget, committed map[string]int, openTrips map[string]int, fuelAccrued map[string]float64, runTrips *[]runTrip) bool {
	for _, v := range compatibleVehicles(in, g) {
		weeklyUsed := in.FuelUsedL[v.VehicleID] + fuelAccrued[v.VehicleID]
		// 1. Try to extend the vehicle's open trip (if it is for this group).
		if idx, ok := openTrips[v.VehicleID]; ok && idx >= 0 && idx < len(*runTrips) {
			rt := &(*runTrips)[idx]
			if rt.brand == g.Brand && rt.district == g.District {
				extended := append(append([]Order{}, rt.orders...), o)
				// The vehicle's budget excluding this trip's current minutes, so
				// the candidate's full total is checked exactly once.
				others := withoutTrip(budget[v.VehicleID], rt.computed)
				cand := Candidate{
					Vehicle: v, TripNo: rt.tripNo, Orders: extended,
					DayBudget: others, WeeklyFuelUsedL: weeklyUsed,
				}
				verdict := CheckTrip(in, cand)
				if verdict.OK {
					// Replace this trip's budget contribution with the extended
					// trip's total, leaving other trips' minutes in place.
					budget[v.VehicleID] = addTrip(others, verdict.Trip)
					rt.orders = extended
					rt.computed = verdict.Trip
					return true
				}
			}
		}

		// 2. Open a new trip when the vehicle has room (max two per day).
		next := committed[v.VehicleID] + 1
		if next > MaxTripsPerVehiclePerDay {
			continue
		}
		cand := Candidate{
			Vehicle: v, TripNo: next, Orders: []Order{o},
			DayBudget: budget[v.VehicleID], WeeklyFuelUsedL: weeklyUsed,
		}
		verdict := CheckTrip(in, cand)
		if verdict.OK {
			*runTrips = append(*runTrips, runTrip{
				vehicleID: v.VehicleID, tripNo: next, brand: g.Brand,
				district: g.District, depotID: g.DepotID, orders: []Order{o}, computed: verdict.Trip,
			})
			openTrips[v.VehicleID] = len(*runTrips) - 1
			committed[v.VehicleID]++
			budget[v.VehicleID] = addTrip(budget[v.VehicleID], verdict.Trip)
			if fuel, err := tripFuelLitres(in, verdict.Trip, v); err == nil {
				fuelAccrued[v.VehicleID] += fuel
			}
			return true
		}
	}
	return false
}

// compatibleVehicles returns the available vehicles at the group's depot, in
// stable vehicle-id order. Temperature and van capability are enforced per order
// by CheckTrip, so they are not filtered here.
func compatibleVehicles(in Input, g groupKey) []Vehicle {
	out := make([]Vehicle, 0)
	for _, v := range in.Vehicles {
		if !v.Available || v.DepotID != g.DepotID {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VehicleID < out[j].VehicleID })
	return out
}

// addTrip returns the budget with one more completed trip's minutes added to the
// pool that trip's brand draws on (Fresh alone; Style and Tech share one pool).
func addTrip(b VehicleDayBudget, t PlannedTrip) VehicleDayBudget {
	if t.Brand == domain.BrandFresh {
		b.FreshMinutesUsed += t.TotalTripMin
	} else {
		b.StyleTechMinutesUsed += t.TotalTripMin
	}
	return b
}

// withoutTrip returns the budget with one trip's minutes removed from its pool.
// It is used when re-checking an extension: the vehicle's other committed trips
// must remain counted, but the trip being extended is measured fresh.
func withoutTrip(b VehicleDayBudget, t PlannedTrip) VehicleDayBudget {
	if t.Brand == domain.BrandFresh {
		b.FreshMinutesUsed -= t.TotalTripMin
	} else {
		b.StyleTechMinutesUsed -= t.TotalTripMin
	}
	return b
}

// bestEffortDeferral explains why an order could not be placed: it re-checks it
// against the group's fleet and reports the binding constraint. It uses the same
// weekly-used figure as the placement path (ledger + this run's accrued fuel).
func bestEffortDeferral(in Input, o Order, budget map[string]VehicleDayBudget, committed map[string]int, fuelAccrued map[string]float64) DeferredOrder {
	base := DeferredOrder{
		OrderID: o.OrderID, OrderNumber: o.OrderNumber, OutletID: o.OutletID,
		Brand: o.Brand, District: o.District,
	}

	g := groupKey{DepotID: o.DepotID, Brand: o.Brand, District: o.District}
	vehicles := compatibleVehicles(in, g)
	if len(vehicles) == 0 {
		base.Constraint = domain.ConstraintVehicleUnavailable
		base.Reason = "No available vehicle is based at this depot for the order's brand and district."
		return base
	}

	// If every vehicle is already at the two-trip cap, that is the reason.
	allCapped := true
	for _, v := range vehicles {
		if committed[v.VehicleID] < MaxTripsPerVehiclePerDay {
			allCapped = false
			break
		}
	}
	if allCapped {
		base.Constraint = domain.ConstraintTripLimitExceeded
		base.Reason = "Every compatible vehicle is already running its two permitted trips."
		return base
	}

	// Otherwise report the first binding constraint across the compatible fleet.
	var best *TripVerdict
	for _, v := range vehicles {
		next := committed[v.VehicleID] + 1
		if next > MaxTripsPerVehiclePerDay {
			next = MaxTripsPerVehiclePerDay
		}
		cand := Candidate{
			Vehicle: v, TripNo: next, Orders: []Order{o},
			DayBudget: budget[v.VehicleID], WeeklyFuelUsedL: in.FuelUsedL[v.VehicleID] + fuelAccrued[v.VehicleID],
		}
		verdict := CheckTrip(in, cand)
		if best == nil || fewerViolations(verdict, *best) {
			local := verdict
			best = &local
		}
	}
	if best != nil && len(best.Violations) > 0 {
		base.Constraint = best.Violations[0]
		base.Reason = best.Reason
	} else {
		base.Constraint = domain.ConstraintOrderSplit
		base.Reason = "The order did not fit any feasible trip under the daily budgets."
	}
	return base
}

func fewerViolations(a, b TripVerdict) bool { return len(a.Violations) < len(b.Violations) }

// sortTrips orders the final trips deterministically: vehicle id, then trip no.
func sortTrips(trips []PlannedTrip) {
	sort.Slice(trips, func(i, j int) bool {
		if trips[i].VehicleID != trips[j].VehicleID {
			return trips[i].VehicleID < trips[j].VehicleID
		}
		return trips[i].TripNo < trips[j].TripNo
	})
}

// sortDeferrals makes the deferral list deterministic by outlet, then order.
func sortDeferrals(d []DeferredOrder) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].OutletID != d[j].OutletID {
			return d[i].OutletID < d[j].OutletID
		}
		return d[i].OrderNumber < d[j].OrderNumber
	})
}

// groupKey identifies one compatible (depot, brand, district) grouping.
type groupKey struct {
	DepotID  string
	Brand    domain.Brand
	District string
}

func (g groupKey) String() string { return g.DepotID + "\x00" + string(g.Brand) + "\x00" + g.District }

func parseGroupKey(s string) groupKey {
	parts := splitN(s, '\x00', 3)
	var g groupKey
	if len(parts) == 3 {
		g.DepotID = parts[0]
		g.Brand = domain.Brand(parts[1])
		g.District = parts[2]
	}
	return g
}

// groupOrders buckets eligible orders by compatibility key, each bucket ordered
// deterministically (outlet id, order number, input sequence). This ordering is
// a deterministic tie-break, not a business priority.
func groupOrders(orders []Order) map[string][]Order {
	groups := map[string][]Order{}
	for _, o := range orders {
		k := groupKey{DepotID: o.DepotID, Brand: o.Brand, District: o.District}.String()
		groups[k] = append(groups[k], o)
	}
	for k := range groups {
		sort.SliceStable(groups[k], func(i, j int) bool {
			return lessOrder(groups[k][i], groups[k][j])
		})
	}
	return groups
}

func sortedGroupKeys(groups map[string][]Order) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	// Groups are planned in prioritised order too, so an outlet that was
	// deferred yesterday (or is starved, or is carrying the chilled goods) is
	// not left behind because it happens to sort late on district name.
	sort.Slice(keys, func(i, j int) bool {
		return lessGroup(groups[keys[i]], groups[keys[j]], keys[i], keys[j])
	})
	return keys
}

// partitionEligible splits orders into those eligible for the run and those that
// must be reported instead.
func partitionEligible(in Input) (eligible []Order, ineligible []DeferredOrder) {
	for _, o := range in.Orders {
		el := EligibleOrder(o, in.PlanningDate, in.Calendar)
		if el.Eligible {
			eligible = append(eligible, o)
			continue
		}
		if el.Constraint != "" {
			ineligible = append(ineligible, DeferredOrder{
				OrderID: o.OrderID, OrderNumber: o.OrderNumber, OutletID: o.OutletID,
				Brand: o.Brand, District: o.District, Constraint: el.Constraint, Reason: el.Reason,
			})
		}
	}
	return eligible, ineligible
}

// splitN splits s on sep into at most n parts.
func splitN(s string, sep byte, n int) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != sep || len(parts) == n-1 {
			continue
		}
		parts = append(parts, s[start:i])
		start = i + 1
	}
	parts = append(parts, s[start:])
	return parts
}
