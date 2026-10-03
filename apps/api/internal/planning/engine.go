package planning

import (
	"sort"

	"waypoint.lk/api/internal/domain"
)

// Engine turns loaded inputs into a feasible, deterministic proposal. It holds
// no state and performs no I/O, so a run is a pure function of its Input.
//
// DESIGN — no optimization objective is implemented. Feasibility is the only
// filter; among feasible options the engine takes a documented, stable order and
// nothing else. There is no distance, fuel, utilisation, priority or fairness
// score. The prioritisation policy (docs/prioritisation-policy.md) is a soft
// ordering the dispatcher may apply during review; it is not a feasibility rule
// and does not select between feasible vehicles here.
//
// The algorithm:
//
//  1. Filter to eligible orders (CONFIRMED, for the planning date, operating day).
//  2. Group by (depot, brand, district) — a trip holds one brand and one
//     district only, so this is the finest compatible group.
//  3. Within a group, order deterministically by outlet id, then order number,
//     then input sequence.
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
	runTrips := []runTrip{}

	for _, key := range sortedGroupKeys(groups) {
		g := parseGroupKey(key)
		for _, o := range groups[key] {
			if !tryPlace(in, o, g, budget, committed, openTrips, &runTrips) {
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
		deferred = append(deferred, bestEffortDeferral(in, o, budget, committed))
	}
	deferred = append(deferred, ineligible...)
	sortDeferrals(deferred)

	return Result{Trips: trips, Deferred: deferred}
}

// tryPlace attempts to place one whole order onto a run trip. It returns true on
// success.
func tryPlace(in Input, o Order, g groupKey, budget map[string]VehicleDayBudget, committed map[string]int, openTrips map[string]int, runTrips *[]runTrip) bool {
	for _, v := range compatibleVehicles(in, g) {
		// 1. Try to extend the vehicle's open trip (if it is for this group).
		if idx, ok := openTrips[v.VehicleID]; ok && idx >= 0 && idx < len(*runTrips) {
			rt := &(*runTrips)[idx]
			if rt.brand == g.Brand && rt.district == g.District {
				extended := append(append([]Order{}, rt.orders...), o)
				cand := Candidate{
					Vehicle: v, TripNo: rt.tripNo, Orders: extended,
					DayBudget: budget[v.VehicleID], WeeklyFuelUsedL: in.FuelUsedL[v.VehicleID],
				}
				verdict := CheckTrip(in, cand)
				if verdict.OK {
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
			DayBudget: budget[v.VehicleID], WeeklyFuelUsedL: in.FuelUsedL[v.VehicleID],
		}
		verdict := CheckTrip(in, cand)
		if verdict.OK {
			*runTrips = append(*runTrips, runTrip{
				vehicleID: v.VehicleID, tripNo: next, brand: g.Brand,
				district: g.District, depotID: g.DepotID, orders: []Order{o}, computed: verdict.Trip,
			})
			openTrips[v.VehicleID] = len(*runTrips) - 1
			committed[v.VehicleID]++
			accrue(budget, v.VehicleID, verdict.Trip)
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

// accrue adds a completed trip's minutes to the vehicle's day budget.
func accrue(budget map[string]VehicleDayBudget, vehicleID string, t PlannedTrip) {
	b := budget[vehicleID]
	if t.Brand == domain.BrandFresh {
		b.FreshMinutesUsed += t.TotalTripMin
	} else {
		b.StyleTechMinutesUsed += t.TotalTripMin
	}
	budget[vehicleID] = b
}

// bestEffortDeferral explains why an order could not be placed: it re-checks it
// against the group's fleet and reports the binding constraint.
func bestEffortDeferral(in Input, o Order, budget map[string]VehicleDayBudget, committed map[string]int) DeferredOrder {
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
			DayBudget: budget[v.VehicleID], WeeklyFuelUsedL: in.FuelUsedL[v.VehicleID],
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
			a, b := groups[k][i], groups[k][j]
			if a.OutletID != b.OutletID {
				return a.OutletID < b.OutletID
			}
			if a.OrderNumber != b.OrderNumber {
				return a.OrderNumber < b.OrderNumber
			}
			return a.Sequence < b.Sequence
		})
	}
	return groups
}

func sortedGroupKeys(groups map[string][]Order) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
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
