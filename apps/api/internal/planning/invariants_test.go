package planning

import (
	"testing"

	"waypoint.lk/api/internal/domain"
)

// TestEngineInvariants runs a larger scenario and asserts the business
// invariants that must hold for every plan. These are properties, not golden
// values, so they stay true as the engine evolves.
func TestEngineInvariants(t *testing.T) {
	in := invariantInput()
	res := New().Plan(in)

	byVehicle := map[string]int{}
	served := map[string]int{}

	for _, tr := range res.Trips {
		// 4. one brand, one district per trip
		brand, district := "", ""
		var weight, volume float64
		reefer := false
		vanOnly := false
		for i, id := range tr.OrderIDs {
			o := findOrder(in.Orders, id)
			if i == 0 {
				brand, district = string(o.Brand), o.District
			} else if string(o.Brand) != brand || o.District != district {
				t.Fatalf("trip %s/%d mixes brand or district", tr.VehicleID, tr.TripNo)
			}
			weight += o.TotalWeightKg
			volume += o.TotalVolumeM3
			if o.TempRequirement.RequiresReefer() {
				reefer = true
			}
			if o.ParkingConstraint == domain.ParkingVanOnly {
				vanOnly = true
			}
			served[id]++
		}

		v := findVehicle(in.Vehicles, tr.VehicleID)
		// 6. capacity never exceeded
		if weight > v.WeightCapKg+1e-9 {
			t.Fatalf("trip %s/%d weight %.2f > cap %.2f", tr.VehicleID, tr.TripNo, weight, v.WeightCapKg)
		}
		if volume > v.VolumeCapM3+1e-9 {
			t.Fatalf("trip %s/%d volume %.3f > cap %.3f", tr.VehicleID, tr.TripNo, volume, v.VolumeCapM3)
		}
		// 7. refrigeration satisfied
		if reefer && v.TempClass != domain.VehicleTempReefer {
			t.Fatalf("trip %s/%d carries chilled/frozen without a reefer", tr.VehicleID, tr.TripNo)
		}
		// 8. van-only satisfied
		if vanOnly && v.Type != domain.VehicleVan {
			t.Fatalf("trip %s/%d serves a van-only outlet without a van", tr.VehicleID, tr.TripNo)
		}
		byVehicle[tr.VehicleID]++
	}

	// 1 + 2. no order split or duplicated
	for id, n := range served {
		if n != 1 {
			t.Fatalf("order %s served %d times", id, n)
		}
	}
	// 3. at most two trips per vehicle
	for id, n := range byVehicle {
		if n > MaxTripsPerVehiclePerDay {
			t.Fatalf("vehicle %s has %d trips, over the cap", id, n)
		}
	}
	// 12. every unserved order has a reason
	for _, d := range res.Deferred {
		if d.Constraint == "" || d.Reason == "" {
			t.Fatalf("deferral for %s lacks a constraint/reason", d.OrderID)
		}
	}
}

func invariantInput() Input {
	in := baseInput()
	// Two depots, two districts, all three brands, mixed temperatures and a
	// van-only outlet, across a handful of vehicles.
	in.Vehicles = []Vehicle{
		{VehicleID: "VEH001", Type: domain.VehicleTruck, TempClass: domain.VehicleTempReefer, WeightCapKg: 3000, VolumeCapM3: 12, KmPerL: 5, DepotID: "d-peli", Available: true},
		{VehicleID: "VEH002", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient, WeightCapKg: 2500, VolumeCapM3: 10, KmPerL: 5, DepotID: "d-peli", Available: true},
		{VehicleID: "VEH003", Type: domain.VehicleVan, TempClass: domain.VehicleTempAmbient, WeightCapKg: 1200, VolumeCapM3: 6, KmPerL: 8, DepotID: "d-peli", Available: true},
		{VehicleID: "VEH004", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient, WeightCapKg: 2500, VolumeCapM3: 10, KmPerL: 5, DepotID: "d-peli", Available: false},
	}
	in.Travel["d-peli|Gampaha"] = Travel{District: "Gampaha", DepotID: "d-peli", DepotToDistrictKm: 28, DepotToDistrictMin: 37, InterStopKm: 7, InterStopMin: 9}
	in.ServiceAllowances["STYLE|STREET"] = 16

	in.Orders = nil
	add := func(id, outlet string, brand domain.Brand, district string, w, v float64, temp domain.TempRequirement, parking domain.ParkingConstraint) {
		o := order(id, outlet, brand, district, "d-peli", w, v, temp, parking)
		o.WindowOpen, o.WindowClose = "05:00", "07:30"
		if brand != domain.BrandFresh {
			o.WindowOpen, o.WindowClose = "09:00", "17:00"
		}
		in.Orders = append(in.Orders, o)
	}
	add("A", "OUT001", domain.BrandFresh, "Colombo", 200, 2, domain.TempAmbient, domain.ParkingNormal)
	add("B", "OUT001", domain.BrandFresh, "Colombo", 300, 3, domain.TempChilled, domain.ParkingNormal)
	add("C", "OUT002", domain.BrandFresh, "Colombo", 100, 1, domain.TempAmbient, domain.ParkingVanOnly)
	add("D", "OUT001", domain.BrandFresh, "Gampaha", 500, 5, domain.TempAmbient, domain.ParkingNormal)
	add("E", "OUT001", domain.BrandStyle, "Colombo", 400, 8, domain.TempAmbient, domain.ParkingNormal)
	add("F", "OUTMALL", domain.BrandTech, "Colombo", 800, 3, domain.TempAmbient, domain.ParkingMallDock)
	return in
}

func findOrder(orders []Order, id string) Order {
	for _, o := range orders {
		if o.OrderID == id {
			return o
		}
	}
	return Order{}
}

func findVehicle(vehicles []Vehicle, id string) Vehicle {
	for _, v := range vehicles {
		if v.VehicleID == id {
			return v
		}
	}
	return Vehicle{}
}
