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
