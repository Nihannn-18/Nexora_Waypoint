package planning

import (
	"reflect"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// engineInput builds a small world with several compatible Fresh/Colombo orders
// and a couple of vehicles so grouping and trip packing can be exercised.
func engineInput() Input {
	in := baseInput()
	in.Orders = []Order{
		order("O3", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 1, domain.TempAmbient, domain.ParkingNormal),
		order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 1, domain.TempAmbient, domain.ParkingNormal),
		order("O2", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 1, domain.TempAmbient, domain.ParkingNormal),
	}
	return in
}

func TestEngineGroupsAndPlans(t *testing.T) {
	in := engineInput()
	res := New().Plan(in)

	if len(res.Deferred) != 0 {
		t.Fatalf("expected no deferrals, got %+v", res.Deferred)
	}
	if len(res.Trips) == 0 {
		t.Fatal("expected at least one trip")
	}
	// Every order served exactly once.
	seen := map[string]int{}
	total := 0
	for _, tr := range res.Trips {
		if tr.Brand != domain.BrandFresh {
			t.Fatalf("trip brand = %s", tr.Brand)
		}
		for _, id := range tr.OrderIDs {
			seen[id]++
			total++
		}
	}
	if total != 3 {
		t.Fatalf("served %d order instances, want 3", total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("order %s appears %d times; an order must not be split or duplicated", id, n)
		}
	}
}

func TestEngineNoSplitOversizedOrder(t *testing.T) {
	in := baseInput()
	// A single order larger than any vehicle: must be deferred, never split.
	in.Orders = []Order{
		order("BIG", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 99999, 999, domain.TempAmbient, domain.ParkingNormal),
	}
	res := New().Plan(in)
	if len(res.Trips) != 0 {
		t.Fatal("an oversized order must not create a trip")
	}
	if len(res.Deferred) != 1 || res.Deferred[0].OrderID != "BIG" {
		t.Fatalf("expected BIG deferred, got %+v", res.Deferred)
	}
}

func TestEngineTripCap(t *testing.T) {
	in := baseInput()
	// One tiny van (van-only not required) at the depot; give many orders so more
	// than two trips would be needed. With only one vehicle the third batch must
	// be deferred with TRIP_LIMIT_EXCEEDED or a budget reason.
	in.Vehicles = []Vehicle{
		{VehicleID: "VEH001", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient,
			WeightCapKg: 5000, VolumeCapM3: 1000, KmPerL: 5, DepotID: "d-peli", Available: true},
	}
	// The single vehicle can hold everything, so force many orders that exceed
	// one trip's 270 Fresh minutes but not two.
	in.Orders = nil
	for i := 0; i < 20; i++ {
		in.Orders = append(in.Orders, order("O"+itoa(i), "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal))
	}
	res := New().Plan(in)
	if len(res.Trips) > 2 {
		t.Fatalf("a vehicle may run at most 2 trips, got %d", len(res.Trips))
	}
}

func TestEngineDeterministic(t *testing.T) {
	in := engineInput()
	first := New().Plan(in)
	for i := 0; i < 20; i++ {
		again := New().Plan(in)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("plan is not deterministic:\n first=%+v\n again=%+v", first, again)
		}
	}
}

func TestEngineIneligibleNonOperatingDay(t *testing.T) {
	in := engineInput()
	in.Calendar = CalendarDay{Date: date(2026, 9, 26), IsOperating: false}
	res := New().Plan(in)
	if len(res.Trips) != 0 {
		t.Fatal("no trips on a non-operating day")
	}
	for _, d := range res.Deferred {
		if d.Constraint != domain.ConstraintNonOperatingDay {
			t.Fatalf("expected NON_OPERATING_DAY, got %s", d.Constraint)
		}
	}
}

func TestEngineDifferentDatesNotPlanned(t *testing.T) {
	in := baseInput()
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	o.RequestedDeliveryDate = date(2026, 9, 27) // tomorrow
	in.Orders = []Order{o}
	res := New().Plan(in)
	if len(res.Trips) != 0 || len(res.Deferred) != 0 {
		t.Fatalf("an order for another date is not part of this run: trips=%d deferred=%d", len(res.Trips), len(res.Deferred))
	}
}

// budgetTotalsByVehicle returns, per vehicle, the Fresh and Style+Tech minutes
// a result would commit. It is the invariant check used by the regression tests:
// no vehicle may exceed its pool.
func budgetTotalsByVehicle(res Result) map[string]VehicleDayBudget {
	out := map[string]VehicleDayBudget{}
	for _, tr := range res.Trips {
		b := out[tr.VehicleID]
		if tr.Brand == domain.BrandFresh {
			b.FreshMinutesUsed += tr.TotalTripMin
		} else {
			b.StyleTechMinutesUsed += tr.TotalTripMin
		}
		out[tr.VehicleID] = b
	}
	return out
}

// oneVehicleWorld returns a single available vehicle of the given id and a
// Fresh/Colombo allowance deliberately chosen so a trip can be extended several
// times before it saturates: with outbound 24 and inter-stop 8, an allowance of
// 45 makes a 1-order Colombo trip 69 minutes and each extra stop add 53, so a
// trip absorbs several stops while the vehicle's two trips together would breach
// the Fresh 270 budget if the second trip ignored the first's minutes.
func oneVehicleWorld(vehicleID string, allowanceKey string, allowance int) Input {
	in := baseInput()
	in.ServiceAllowances[allowanceKey] = allowance
	in.Vehicles = []Vehicle{
		{VehicleID: vehicleID, Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient,
			WeightCapKg: 100000, VolumeCapM3: 100000, KmPerL: 5, DepotID: "d-peli", Available: true},
	}
	return in
}

// TestEngineExtensionAccumulatesFreshBudget is the deterministic regression for
// the observed VEH006/VEH009 over-budget behaviour. Before the fix, extending an
// open trip never updated the vehicle's day budget, so trip 2 was checked against
// a budget that under-counted trip 1 and a Fresh vehicle could exceed 270.
//
// The fixture is built around a single Colombo vehicle with a 45-minute handling
// allowance. The arithmetic (outbound 24, inter-stop 8) is:
//
//	trip a is 24 + 8*(n-1) + 45n
//	  n=1 → 69, n=2 → 122, n=3 → 175, n=4 → 228, n=5 → 281
//
// So each trip individually looks viable (69−228 < 270), but once trip 1 has
// grown to 228 the vehicle has only 42 Fresh minutes left and a second trip
// (69 minimum) must be refused. The regression is that a bug which ignores trip
// 1's final minutes would open trip 2 and total 297 > 270. We assert the engine
// never emits such a plan and that it defers the order it cannot serve.
func TestEngineExtensionAccumulatesFreshBudget(t *testing.T) {
	in := oneVehicleWorld("VEH009", "FRESH|STREET", 45)
	in.Orders = nil
	// Five orders: enough to grow trip 1 to its ceiling and then demand a second
	// trip that cannot fit once trip 1's real minutes are counted.
	for i := 0; i < 5; i++ {
		in.Orders = append(in.Orders, order("O"+itoa(i), "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal))
	}

	res := New().Plan(in)

	// Precondition for the fixture: trip 1 must actually be viable on its own and
	// trip 2 must individually appear viable, otherwise the test would pass for
	// the wrong reason (a trip that simply cannot open). 69 and 228 are both
	// inside the 270 budget.
	single := order("PROBE", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal)
	probe := CheckTrip(in, Candidate{Vehicle: in.Vehicles[0], TripNo: 2, Orders: []Order{single}})
	if !probe.OK {
		t.Fatalf("fixture precondition: a lone trip must be individually viable, got %v", probe.Violations)
	}

	// The invariant: no vehicle may exceed the Fresh budget across its trips.
	totals := budgetTotalsByVehicle(res)
	for vehicleID, b := range totals {
		if b.FreshMinutesUsed > FreshTimeBudgetMin {
			t.Fatalf("vehicle %s committed %d Fresh minutes, over the %d budget", vehicleID, b.FreshMinutesUsed, FreshTimeBudgetMin)
		}
	}
	for _, tr := range res.Trips {
		if tr.Brand == domain.BrandFresh && tr.TotalTripMin > FreshTimeBudgetMin {
			t.Fatalf("trip %s/%d is %d Fresh minutes, over the %d budget", tr.VehicleID, tr.TripNo, tr.TotalTripMin, FreshTimeBudgetMin)
		}
	}

	// The engine must have deferred exactly the order it could not fit — proving
	// it did not over-serve by ignoring trip 1's accumulated minutes.
	served := 0
	for _, tr := range res.Trips {
		served += len(tr.OrderIDs)
	}
	if served >= 5 {
		t.Fatalf("served %d of 5 orders; with trip 1 at its ceiling the engine must defer the remainder", served)
	}
	if len(res.Deferred) == 0 {
		t.Fatal("expected at least one deferred order once the Fresh budget was exhausted")
	}
}

// TestEngineExtensionAccumulatesStyleTechBudget applies the same deterministic
// regression to the shared Style + Tech 480-minute pool. Style/Colombo with a
// 90-minute allowance gives:
//
//	trip = 24 + 8*(n-1) + 90n → n=1 → 114, n=2 → 212, n=3 → 310, n=4 → 408, n=5 → 506
//
// Trip 1 can reach 408 (< 480), leaving only 72 minutes, so trip 2 (114 minimum)
// must be refused; a bug ignoring trip 1 would open trip 2 and total 522 > 480.
func TestEngineExtensionAccumulatesStyleTechBudget(t *testing.T) {
	in := oneVehicleWorld("VEH010", "STYLE|STREET", 90)
	in.Orders = nil
	for i := 0; i < 5; i++ {
		o := order("T"+itoa(i), "OUT001", domain.BrandStyle, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal)
		o.WindowOpen, o.WindowClose = "09:00", "17:00"
		in.Orders = append(in.Orders, o)
	}

	res := New().Plan(in)

	// Precondition: a lone Style trip is individually viable under 480.
	single := order("PROBE", "OUT001", domain.BrandStyle, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal)
	single.WindowOpen, single.WindowClose = "09:00", "17:00"
	probe := CheckTrip(in, Candidate{Vehicle: in.Vehicles[0], TripNo: 2, Orders: []Order{single}})
	if !probe.OK {
		t.Fatalf("fixture precondition: a lone Style trip must be individually viable, got %v", probe.Violations)
	}

	for vehicleID, b := range budgetTotalsByVehicle(res) {
		if b.StyleTechMinutesUsed > StyleTechTimeBudgetMin {
			t.Fatalf("vehicle %s committed %d Style+Tech minutes, over the shared %d budget", vehicleID, b.StyleTechMinutesUsed, StyleTechTimeBudgetMin)
		}
	}
	served := 0
	for _, tr := range res.Trips {
		served += len(tr.OrderIDs)
		if tr.TotalTripMin > StyleTechTimeBudgetMin {
			t.Fatalf("trip %s/%d is %d Style+Tech minutes, over the shared %d budget", tr.VehicleID, tr.TripNo, tr.TotalTripMin, StyleTechTimeBudgetMin)
		}
	}
	if served >= 5 || len(res.Deferred) == 0 {
		t.Fatalf("served %d of 5 orders with %d deferrals; trip 1 exhausting the shared pool must defer the rest", served, len(res.Deferred))
	}
}

// TestCheckTripBudgetIsCumulative locks the exact boundary of the authoritative
// validator: a second trip that is fine alone must fail once the vehicle's day
// budget already holds a first trip's minutes, and exactly at budget must pass.
func TestCheckTripBudgetIsCumulative(t *testing.T) {
	in := oneVehicleWorld("VEH011", "FRESH|STREET", 45)
	v := in.Vehicles[0]
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal)

	// Trip 1 alone: 69 minutes. A second 69-minute trip against that budget is
	// 138 ≤ 270 and must pass.
	first := CheckTrip(in, Candidate{Vehicle: v, TripNo: 1, Orders: []Order{o}})
	if !first.OK {
		t.Fatalf("trip 1 must be viable: %v", first.Violations)
	}
	secondBudget := VehicleDayBudget{FreshMinutesUsed: first.Trip.TotalTripMin}
	second := CheckTrip(in, Candidate{Vehicle: v, TripNo: 2, Orders: []Order{o}, DayBudget: secondBudget})
	if !second.OK {
		t.Fatalf("a second trip that fits the remaining budget must pass: %v", second.Violations)
	}

	// Now pretend trip 1 already consumed 250 minutes. A 69-minute trip would
	// reach 319 > 270 and must fail on FRESH_TIME_BUDGET, not on anything else.
	overBudget := VehicleDayBudget{FreshMinutesUsed: FreshTimeBudgetMin - 20} // 250
	failed := CheckTrip(in, Candidate{Vehicle: v, TripNo: 2, Orders: []Order{o}, DayBudget: overBudget})
	if failed.OK {
		t.Fatal("a trip that pushes the day past 270 must fail")
	}
	found := false
	for _, code := range failed.Violations {
		if code == domain.ConstraintFreshTimeBudget {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected FRESH_TIME_BUDGET, got %v", failed.Violations)
	}
}

// TestEngineEveryTripRespectsItsPool is a broad invariant across a mixed world:
// no emitted trip may exceed its brand's daily pool, and no vehicle's summed
// trips may exceed it either.
func TestEngineEveryTripRespectsItsPool(t *testing.T) {
	in := baseInput()
	in.ServiceAllowances["STYLE|STREET"] = 16
	in.Orders = nil
	for i := 0; i < 25; i++ {
		in.Orders = append(in.Orders, order("F"+itoa(i), "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal))
	}
	for i := 0; i < 25; i++ {
		o := order("S"+itoa(i), "OUT001", domain.BrandStyle, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal)
		o.WindowOpen, o.WindowClose = "09:00", "17:00"
		in.Orders = append(in.Orders, o)
	}
	res := New().Plan(in)
	for vehicleID, b := range budgetTotalsByVehicle(res) {
		if b.FreshMinutesUsed > FreshTimeBudgetMin {
			t.Fatalf("vehicle %s Fresh %d > %d", vehicleID, b.FreshMinutesUsed, FreshTimeBudgetMin)
		}
		if b.StyleTechMinutesUsed > StyleTechTimeBudgetMin {
			t.Fatalf("vehicle %s Style+Tech %d > %d", vehicleID, b.StyleTechMinutesUsed, StyleTechTimeBudgetMin)
		}
	}
}

func TestEngineDeferralHasReason(t *testing.T) {
	in := baseInput()
	// Chilled order but only ambient vehicles available: must defer with a reason.
	in.Vehicles = []Vehicle{
		{VehicleID: "VEH002", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient,
			WeightCapKg: 4000, VolumeCapM3: 18, KmPerL: 5, DepotID: "d-peli", Available: true},
	}
	in.Orders = []Order{order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempChilled, domain.ParkingNormal)}
	res := New().Plan(in)
	if len(res.Trips) != 0 {
		t.Fatal("no feasible vehicle")
	}
	if len(res.Deferred) != 1 || res.Deferred[0].Reason == "" || res.Deferred[0].Constraint == "" {
		t.Fatalf("deferral must carry a constraint and a reason: %+v", res.Deferred)
	}
	if res.Deferred[0].Constraint != domain.ConstraintReeferRequired {
		t.Fatalf("expected REEFER_REQUIRED, got %s", res.Deferred[0].Constraint)
	}
}
