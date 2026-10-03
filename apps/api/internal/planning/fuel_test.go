package planning

import (
	"testing"

	"waypoint.lk/api/internal/domain"
)

// TestEngineFuelAccumulatesAcrossTrips proves the fuel rule accumulates across a
// vehicle's trips in one run: trip 2 is checked against weekly ledger usage plus
// trip 1's fuel. A single vehicle with a quota that fits one trip but not two
// must place trip 1 and defer the rest.
func TestEngineFuelAccumulatesAcrossTrips(t *testing.T) {
	in := baseInput()
	// One ambient truck; plenty of capacity and time budget so only fuel binds.
	in.Vehicles = []Vehicle{
		{VehicleID: "VEH001", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient,
			WeightCapKg: 100000, VolumeCapM3: 1000, KmPerL: 5, WeeklyFuelQuotaL: 3.0,
			DepotID: "d-peli", Available: true},
	}
	in.FuelQuotaL = map[string]float64{"VEH001": 3.0}
	// Each order: 12 km (outbound) + inter-stop as stops grow; fuel = dist/5.
	// trip 1 (one order) = 12/5 = 2.4 L, leaving 0.6 L for trip 2, which cannot
	// take even one more order (2.4 L). So exactly one trip, the rest deferred.
	in.Orders = []Order{
		order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal),
		order("O2", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal),
	}
	res := New().Plan(in)

	if len(res.Trips) != 1 {
		t.Fatalf("expected exactly one trip under the quota, got %d", len(res.Trips))
	}
	if len(res.Deferred) != 1 {
		t.Fatalf("expected one deferred order, got %d", len(res.Deferred))
	}
	if res.Deferred[0].Constraint != domain.ConstraintFuelQuotaExceeded {
		t.Fatalf("expected FUEL_QUOTA_EXCEEDED, got %s (%s)", res.Deferred[0].Constraint, res.Deferred[0].Reason)
	}
}

// TestEngineFuelDeterministic proves fuel decisions are reproducible.
func TestEngineFuelDeterministic(t *testing.T) {
	in := baseInput()
	in.FuelQuotaL = map[string]float64{"VEH002": 3.0}
	in.Orders = []Order{
		order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal),
		order("O2", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal),
		order("O3", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal),
	}
	first := New().Plan(in)
	for i := 0; i < 10; i++ {
		again := New().Plan(in)
		if len(again.Trips) != len(first.Trips) || len(again.Deferred) != len(first.Deferred) {
			t.Fatalf("fuel plan not deterministic: %+v vs %+v", first, again)
		}
		for j := range first.Deferred {
			if again.Deferred[j].Constraint != first.Deferred[j].Constraint {
				t.Fatalf("deferral reason drift at %d", j)
			}
		}
	}
}
