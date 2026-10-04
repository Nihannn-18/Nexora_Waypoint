package planning

import (
	"testing"

	"waypoint.lk/api/internal/domain"
)

// mkOrder builds a Fresh/Colombo order at OUT001 with sensible defaults, then
// lets a test override the fields the policy decision turns on.
func mkOrder(id string, weight float64) Order {
	return order(id, "OUT001", domain.BrandFresh, "Colombo", "d-peli", weight, 0.1, domain.TempAmbient, domain.ParkingNormal)
}

// TestLessOrderPolicy pins each step of the documented ordering, and that the
// tie-break is stable.
func TestLessOrderPolicy(t *testing.T) {
	plain := func() Order {
		o := mkOrder("A", 10)
		o.WindowOpen, o.WindowClose = "05:00", "07:30"
		return o
	}

	t.Run("rule 1: deferred yesterday first", func(t *testing.T) {
		a, b := plain(), plain()
		a.OrderID, b.OrderID = "A", "B"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		a.DeferredYesterday = true
		if !lessOrder(a, b) || lessOrder(b, a) {
			t.Fatal("a previously deferred outlet must be considered first")
		}
	})

	t.Run("rule 2: starved outlet first, then longer ignored", func(t *testing.T) {
		a, b := plain(), plain()
		a.OrderID, b.OrderID = "A", "B"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		a.DaysSinceLastServed, b.DaysSinceLastServed = 8, 3
		if !lessOrder(a, b) {
			t.Fatal("an outlet unserved >= 7 days must be considered first")
		}
		// Both starved: the longer-ignored wins.
		a.DaysSinceLastServed, b.DaysSinceLastServed = 20, 9
		if !lessOrder(a, b) {
			t.Fatal("among starved outlets, the longer ignored must be first")
		}
	})

	t.Run("rule 3: chilled before ambient", func(t *testing.T) {
		a, b := plain(), plain()
		a.OrderID, b.OrderID = "A", "B"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		a.TempRequirement = domain.TempChilled
		if !lessOrder(a, b) {
			t.Fatal("chilled must be considered before ambient")
		}
	})

	t.Run("rule 4: narrower window first", func(t *testing.T) {
		a, b := plain(), plain()
		a.OrderID, b.OrderID = "A", "B"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		a.WindowOpen, a.WindowClose = "05:00", "06:00" // 60 min
		b.WindowOpen, b.WindowClose = "05:00", "07:30" // 150 min
		if !lessOrder(a, b) {
			t.Fatal("the narrower delivery window must be considered first")
		}
	})

	t.Run("rule 5: larger order first among equals", func(t *testing.T) {
		a, b := plain(), plain()
		a.OrderID, b.OrderID = "A", "B"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		a.TotalWeightKg, b.TotalWeightKg = 50, 20
		if !lessOrder(a, b) {
			t.Fatal("the larger order must be considered first among equals")
		}
	})

	t.Run("rule 6: Fresh before Style and Tech", func(t *testing.T) {
		a := plain()
		b := order("B", "OUT001", domain.BrandStyle, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
		a.OrderID, b.OrderID = "A", "B"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		if !lessOrder(a, b) || lessOrder(b, a) {
			t.Fatal("Fresh must be considered before Style/Tech when otherwise equal")
		}
	})

	t.Run("tie-break is by outlet then order number", func(t *testing.T) {
		a, b := plain(), plain()
		a.OrderID, b.OrderID = "A", "B"
		a.OutletID, b.OutletID = "OUT001", "OUT002"
		a.OrderNumber, b.OrderNumber = "ORD-A", "ORD-B"
		if !lessOrder(a, b) || lessOrder(b, a) {
			t.Fatal("equal orders must fall back to a stable identity order")
		}
	})
}

// TestPolicyChangesAllocationUnderConstraint proves the policy is not cosmetic:
// when two same-size orders are considered, the policy decides which one the
// engine places first. With a vehicle that holds one order per trip, that is
// directly observable as the order on the earliest trip; on a capacity-blocked
// day it is the difference between served and deferred.
func TestPolicyChangesAllocationUnderConstraint(t *testing.T) {
	competitor := func() Order {
		// Same outlet/district/brand and same size as the winner, so only the
		// policy fields can separate them.
		return order("COMP", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 20, domain.TempAmbient, domain.ParkingNormal)
	}
	winner := func() Order {
		return order("WIN", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 20, domain.TempAmbient, domain.ParkingNormal)
	}

	// One order per trip: the vehicle holds 100 kg, each order is 100 kg.
	oneOrderTruck := func(temp domain.VehicleTempClass) []Vehicle {
		return []Vehicle{
			{VehicleID: "VEH001", Type: domain.VehicleTruck, TempClass: temp,
				WeightCapKg: 100, VolumeCapM3: 20, KmPerL: 5, DepotID: "d-peli", Available: true},
		}
	}

	// trip1Order returns the order on the earliest trip - the one the engine
	// considered first.
	trip1Order := func(res Result) string {
		for _, tr := range res.Trips {
			if tr.TripNo == 1 && len(tr.OrderIDs) > 0 {
				return tr.OrderIDs[0]
			}
		}
		return ""
	}

	t.Run("previously deferred outlet is considered first", func(t *testing.T) {
		in := baseInput()
		in.Vehicles = oneOrderTruck(domain.VehicleTempAmbient)
		w, c := winner(), competitor()
		w.DeferredYesterday = true
		in.Orders = []Order{c, w} // competitor is first in input order
		res := New().Plan(in)

		if got := trip1Order(res); got != "WIN" {
			t.Fatalf("first-placed order = %q, want WIN (the previously deferred outlet)", got)
		}
		for i := 0; i < 10; i++ {
			if got := trip1Order(New().Plan(in)); got != "WIN" {
				t.Fatal("policy ordering must be deterministic across runs")
			}
		}
	})

	t.Run("starved outlet is considered first", func(t *testing.T) {
		in := baseInput()
		in.Vehicles = oneOrderTruck(domain.VehicleTempAmbient)
		w, c := winner(), competitor()
		w.DaysSinceLastServed = 30
		c.DaysSinceLastServed = 1
		in.Orders = []Order{c, w}
		res := New().Plan(in)

		if got := trip1Order(res); got != "WIN" {
			t.Fatalf("first-placed order = %q, want WIN (the starved outlet)", got)
		}
	})

	t.Run("chilled is considered before a same-size ambient order", func(t *testing.T) {
		in := baseInput()
		in.Vehicles = oneOrderTruck(domain.VehicleTempReefer)
		cold := order("COLD", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 20, domain.TempChilled, domain.ParkingNormal)
		amb := order("AMB", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 20, domain.TempAmbient, domain.ParkingNormal)
		in.Orders = []Order{amb, cold}
		res := New().Plan(in)

		if got := trip1Order(res); got != "COLD" {
			t.Fatalf("first-placed order = %q, want COLD (chilled before ambient)", got)
		}
	})

	t.Run("Fresh is considered before a same-size Style order", func(t *testing.T) {
		in := baseInput()
		in.Vehicles = oneOrderTruck(domain.VehicleTempAmbient)
		fresh := order("FRESH", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 20, domain.TempAmbient, domain.ParkingNormal)
		style := order("STYLE", "OUT001", domain.BrandStyle, "Colombo", "d-peli", 100, 20, domain.TempAmbient, domain.ParkingNormal)
		in.Orders = []Order{style, fresh}
		res := New().Plan(in)

		if got := trip1Order(res); got != "FRESH" {
			t.Fatalf("first-placed order = %q, want FRESH (Fresh before Style/Tech)", got)
		}
	})
}

// TestPolicyChangesAllocationWhenOnlyOneFits proves the stronger property: when
// exactly one order can be served at all, the policy decides which one it is.
func TestPolicyChangesAllocationWhenOnlyOneFits(t *testing.T) {
	in := baseInput()
	// One trip only: a Colombo trip is 140 + 16 = 156 Fresh minutes, so a second
	// trip would take the vehicle to 312, over the 270-minute Fresh budget. The
	// vehicle can therefore make exactly one trip, and only one order is served.
	in.Travel["d-peli|Colombo"] = Travel{
		District: "Colombo", DepotID: "d-peli",
		DepotToDistrictKm: 12, DepotToDistrictMin: 140, InterStopKm: 4, InterStopMin: 8,
	}
	in.Vehicles = []Vehicle{
		{VehicleID: "VEH001", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient,
			WeightCapKg: 100, VolumeCapM3: 20, KmPerL: 5, DepotID: "d-peli", Available: true},
	}
	w := order("WIN", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 60, 0.1, domain.TempAmbient, domain.ParkingNormal)
	c := order("COMP", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 60, 0.1, domain.TempAmbient, domain.ParkingNormal)
	w.DeferredYesterday = true
	in.Orders = []Order{c, w}

	res := New().Plan(in)
	if !servedOrder(res, "WIN") {
		t.Fatal("the prioritised order must be the one served when only one fits")
	}
	if servedOrder(res, "COMP") {
		t.Fatal("the lower-priority order must be deferred when only one fits")
	}
}

// servedOrder reports whether an order appears on any proposed trip.
func servedOrder(res Result, orderID string) bool {
	for _, tr := range res.Trips {
		for _, id := range tr.OrderIDs {
			if id == orderID {
				return true
			}
		}
	}
	return false
}
