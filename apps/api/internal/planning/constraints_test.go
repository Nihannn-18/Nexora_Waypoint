package planning

import (
	"errors"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func day() CalendarDay {
	return CalendarDay{Date: date(2026, 9, 26), IsOperating: true}
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// baseInput builds a minimal, valid world: one Fresh/Colombo outlet, a small
// reefer and a dry van, travel and allowance reference.
func baseInput() Input {
	return Input{
		PlanningDate: date(2026, 9, 26),
		DepotID:      "d-peli",
		Calendar:     day(),
		Outlets: map[string]Outlet{
			"OUT001": {OutletID: "OUT001", Brand: domain.BrandFresh, District: "Colombo", DepotID: "d-peli",
				DockType: domain.DockStreet, ParkingConstraint: domain.ParkingNormal, WindowOpen: "05:00", WindowClose: "07:30"},
			"OUT002": {OutletID: "OUT002", Brand: domain.BrandFresh, District: "Colombo", DepotID: "d-peli",
				DockType: domain.DockStreet, ParkingConstraint: domain.ParkingVanOnly, WindowOpen: "05:00", WindowClose: "07:30"},
			"OUTMALL": {OutletID: "OUTMALL", Brand: domain.BrandTech, District: "Colombo", DepotID: "d-peli",
				DockType: domain.DockMallBay, ParkingConstraint: domain.ParkingMallDock,
				WindowOpen: "09:00", WindowClose: "17:00", MallWindowOpen: "10:00", MallWindowClose: "12:00"},
		},
		Vehicles: []Vehicle{
			{VehicleID: "VEH001", Type: domain.VehicleTruck, TempClass: domain.VehicleTempReefer,
				WeightCapKg: 5000, VolumeCapM3: 20, KmPerL: 5, DepotID: "d-peli", Available: true},
			{VehicleID: "VEH002", Type: domain.VehicleTruck, TempClass: domain.VehicleTempAmbient,
				WeightCapKg: 4000, VolumeCapM3: 18, KmPerL: 5, DepotID: "d-peli", Available: true},
			{VehicleID: "VEH003", Type: domain.VehicleVan, TempClass: domain.VehicleTempAmbient,
				WeightCapKg: 1500, VolumeCapM3: 8, KmPerL: 8, DepotID: "d-peli", Available: true},
		},
		Travel: map[string]Travel{
			"d-peli|Colombo": {District: "Colombo", DepotID: "d-peli",
				DepotToDistrictKm: 12, DepotToDistrictMin: 24, InterStopKm: 4, InterStopMin: 8},
		},
		ServiceAllowances: map[string]int{
			"FRESH|STREET": 16, "FRESH|REAR_DOCK": 15, "FRESH|MALL_BAY": 20,
			"TECH|MALL_BAY": 20, "TECH|STREET": 16,
		},
	}
}

func order(id, outlet string, brand domain.Brand, district, depot string, w, v float64, temp domain.TempRequirement, parking domain.ParkingConstraint) Order {
	return Order{
		OrderID: id, OrderNumber: "ORD-" + id, OutletID: outlet, Brand: brand,
		District: district, DepotID: depot, RequestedDeliveryDate: date(2026, 9, 26),
		TotalWeightKg: w, TotalVolumeM3: v, TempRequirement: temp, ParkingConstraint: parking,
		WindowOpen: "05:00", WindowClose: "07:30",
	}
}

// ---------------------------------------------------------------------------
// Capacity boundaries
// ---------------------------------------------------------------------------

func TestCheckTripCapacity(t *testing.T) {
	in := baseInput()
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 4000, 18, domain.TempAmbient, domain.ParkingNormal)

	tests := []struct {
		name string
		w, v float64
		ok   bool
	}{
		{"fits", 4000, 18, true},
		{"exact capacity passes", 4000, 18, true},
		{"weight over", 4001, 18, false},
		{"volume over", 4000, 18.01, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oo := o
			oo.TotalWeightKg, oo.TotalVolumeM3 = tt.w, tt.v
			// VEH002: 4000kg / 18m3.
			cand := Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{oo}}
			got := CheckTrip(in, cand)
			if got.OK != tt.ok {
				t.Fatalf("OK = %v, want %v (violations=%v)", got.OK, tt.ok, got.Violations)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Temperature / reefer
// ---------------------------------------------------------------------------

func TestCheckTripTemperature(t *testing.T) {
	in := baseInput()
	chilled := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 100, 1, domain.TempChilled, domain.ParkingNormal)

	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[0], TripNo: 1, Orders: []Order{chilled}}); !v.OK {
		t.Fatalf("chilled on a reefer should pass: %v", v.Violations)
	}
	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{chilled}}); v.OK {
		t.Fatal("chilled on an ambient truck must fail")
	}
	if !hasViolation(CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{chilled}}), domain.ConstraintReeferRequired) {
		t.Fatal("expected REEFER_REQUIRED")
	}
}

// ---------------------------------------------------------------------------
// Van-only
// ---------------------------------------------------------------------------

func TestCheckTripVanOnly(t *testing.T) {
	in := baseInput()
	o := order("O1", "OUT002", domain.BrandFresh, "Colombo", "d-peli", 100, 1, domain.TempAmbient, domain.ParkingVanOnly)

	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[2], TripNo: 1, Orders: []Order{o}}); !v.OK {
		t.Fatalf("van should serve a van-only outlet: %v", v.Violations)
	}
	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{o}}); v.OK {
		t.Fatal("a truck must not serve a van-only outlet")
	}
}

// ---------------------------------------------------------------------------
// Depot / brand / district
// ---------------------------------------------------------------------------

func TestCheckTripGrouping(t *testing.T) {
	in := baseInput()
	fresh := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	style := order("O2", "OUT001", domain.BrandStyle, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)

	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{fresh, style}}); v.OK {
		t.Fatal("mixing brands must fail")
	}
	if !hasViolation(CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{fresh, style}}), domain.ConstraintBrandDistrictMix) {
		t.Fatal("expected BRAND_DISTRICT_MIX")
	}

	oth := order("O3", "OUT001", domain.BrandFresh, "Gampaha", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{fresh, oth}}); v.OK {
		t.Fatal("mixing districts must fail")
	}

	// Depot mismatch requires a vehicle whose depot differs.
	foreign := in.Vehicles[1]
	foreign.DepotID = "d-kandy"
	if v := CheckTrip(in, Candidate{Vehicle: foreign, TripNo: 1, Orders: []Order{fresh}}); v.OK {
		t.Fatal("a vehicle from another depot must fail")
	}
}

// ---------------------------------------------------------------------------
// Trip number
// ---------------------------------------------------------------------------

func TestCheckTripNumber(t *testing.T) {
	in := baseInput()
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	for _, n := range []int{1, 2} {
		if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: n, Orders: []Order{o}}); !v.OK {
			t.Fatalf("trip %d should be valid: %v", n, v.Violations)
		}
	}
	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 3, Orders: []Order{o}}); v.OK {
		t.Fatal("trip 3 must be invalid")
	}
}

// ---------------------------------------------------------------------------
// Time budgets incl. shared Style+Tech
// ---------------------------------------------------------------------------

func TestCheckTripTimeBudgets(t *testing.T) {
	in := baseInput()
	// A trip's handling scales with stop count; use many orders to exceed 270.
	many := func(n int) []Order {
		out := make([]Order, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, order("O"+itoa(i), "OUT001", domain.BrandFresh, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal))
		}
		return out
	}

	// Fresh 270: outbound 24 + inter-stop 8*(n-1) + handling 16*n. For n=17:
	// 24 + 128 + 272 = 424 > 270, so a large Fresh trip fails; a small one passes.
	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: many(2)}); !v.OK {
		t.Fatalf("small Fresh trip should pass: %v", v.Violations)
	}
	if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: many(17)}); v.OK {
		t.Fatal("large Fresh trip should exceed 270")
	}

	// Style+Tech share 480. A Style trip that alone passes; two such trips on the
	// same vehicle must not both fit.
	styleOrder := order("S1", "OUT001", domain.BrandStyle, "Colombo", "d-peli", 1, 0.001, domain.TempAmbient, domain.ParkingNormal)
	// Style departs at 09:00, so give the outlet a Style-appropriate window.
	styleOrder.WindowOpen, styleOrder.WindowClose = "09:00", "17:00"
	in.ServiceAllowances["STYLE|STREET"] = 16
	// A trip near but under 480: outbound 24 + inter-stop 8*(n-1) + 16*n.
	// n=22: 24 + 168 + 352 = 544 (>480). n=18: 24 + 136 + 288 = 448 (<480).
	medium := make([]Order, 0, 18)
	for i := 0; i < 18; i++ {
		medium = append(medium, styleOrder)
	}
	first := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: medium})
	if !first.OK {
		t.Fatalf("first Style trip should fit under 480: %v", first.Violations)
	}
	used := VehicleDayBudget{StyleTechMinutesUsed: first.Trip.TotalTripMin}
	second := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 2, Orders: medium, DayBudget: used})
	if second.OK {
		t.Fatal("second Style trip must exceed the shared 480 budget")
	}
	if !hasViolation(second, domain.ConstraintStyleTechBudget) {
		t.Fatal("expected STYLE_TECH_TIME_BUDGET")
	}
}

// ---------------------------------------------------------------------------
// Availability
// ---------------------------------------------------------------------------

func TestCheckTripAvailability(t *testing.T) {
	in := baseInput()
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	off := in.Vehicles[1]
	off.Available = false
	if v := CheckTrip(in, Candidate{Vehicle: off, TripNo: 1, Orders: []Order{o}}); v.OK {
		t.Fatal("an unavailable vehicle must fail")
	}
}

// ---------------------------------------------------------------------------
// Fuel
// ---------------------------------------------------------------------------

// fuelInput returns baseInput with a known travel distance and a vehicle whose
// km/l and quota can be set per test. VEH002 (ambient truck) is the subject.
func fuelInput() Input {
	in := baseInput()
	// Colombo: depot_to_district_km = 12, inter_stop_km = 4 (from baseInput).
	// VEH002 km/l = 5, quota 400 by default.
	return in
}

func TestCheckTripFuelQuota(t *testing.T) {
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)

	t.Run("no quota modelled leaves the rule unapplied", func(t *testing.T) {
		in := fuelInput()
		if v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{o}}); hasRule(v, domain.ConstraintFuelQuotaExceeded) {
			t.Fatal("fuel rule should not be reported when the vehicle has no quota")
		}
	})

	t.Run("below quota passes", func(t *testing.T) {
		in := fuelInput()
		in.FuelQuotaL = map[string]float64{"VEH002": 400}
		v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{o}})
		if !v.OK {
			t.Fatalf("expected pass, got %v", v.Violations)
		}
	})

	t.Run("exactly at quota passes", func(t *testing.T) {
		in := fuelInput()
		// One order Colombo: distance 12 + 4*0 = 12 km; fuel = 12/5 = 2.4 L.
		// Set used + quota so that used + 2.4 == quota exactly.
		in.FuelUsedL = map[string]float64{"VEH002": 7.6}
		in.FuelQuotaL = map[string]float64{"VEH002": 10.0}
		v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{o}, WeeklyFuelUsedL: 7.6})
		if !v.OK {
			t.Fatalf("exact quota boundary must pass, got %v", v.Violations)
		}
	})

	t.Run("over quota fails", func(t *testing.T) {
		in := fuelInput()
		in.FuelQuotaL = map[string]float64{"VEH002": 2} // 2.4 L needed
		v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{o}})
		if v.OK || !hasViolation(v, domain.ConstraintFuelQuotaExceeded) {
			t.Fatalf("expected FUEL_QUOTA_EXCEEDED, got %v", v.Violations)
		}
	})

	t.Run("prior weekly usage is counted", func(t *testing.T) {
		in := fuelInput()
		in.FuelQuotaL = map[string]float64{"VEH002": 5}
		// used 4 + trip 2.4 = 6.4 > 5 fails.
		in.FuelUsedL = map[string]float64{"VEH002": 4}
		v := CheckTrip(in, Candidate{Vehicle: in.Vehicles[1], TripNo: 1, Orders: []Order{o}, WeeklyFuelUsedL: 4})
		if v.OK {
			t.Fatal("prior usage should push it over quota")
		}
	})
}

func TestCheckTripFuelEfficiencyInvalid(t *testing.T) {
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	for _, kmpl := range []float64{0, -3} {
		in := fuelInput()
		in.FuelQuotaL = map[string]float64{"VEH002": 400}
		v := in.Vehicles[1]
		v.KmPerL = kmpl
		got := CheckTrip(in, Candidate{Vehicle: v, TripNo: 1, Orders: []Order{o}})
		if got.OK {
			t.Fatalf("km_per_l=%v must be rejected", kmpl)
		}
		if !hasViolation(got, domain.ConstraintFuelEfficiencyInvalid) {
			t.Fatalf("km_per_l=%v expected FUEL_EFFICIENCY_INVALID, got %v", kmpl, got.Violations)
		}
		if hasViolation(got, domain.ConstraintFuelQuotaExceeded) {
			t.Fatalf("km_per_l=%v must not be reported as FUEL_QUOTA_EXCEEDED", kmpl)
		}
	}
}

func TestEstimateFuelLitresFormula(t *testing.T) {
	// 100 km / 5 km per l = 20 l.
	got, err := EstimateFuelLitres(100, 5)
	if err != nil || got != 20 {
		t.Fatalf("EstimateFuelLitres(100,5) = %v, %v; want 20", got, err)
	}
	if _, err := EstimateFuelLitres(100, 0); !errors.Is(err, ErrInvalidEfficiency) {
		t.Fatalf("zero km/l err = %v, want ErrInvalidEfficiency", err)
	}
}

func TestTripFuelDifferentEfficiency(t *testing.T) {
	in := fuelInput()
	o := order("O1", "OUT001", domain.BrandFresh, "Colombo", "d-peli", 10, 0.1, domain.TempAmbient, domain.ParkingNormal)
	cand := Candidate{TripNo: 1, Orders: []Order{o}}

	slow := in.Vehicles[1] // km/l 5
	fast := in.Vehicles[1]
	fast.KmPerL = 10

	trip, _ := computeTrip(in, cand, 10, 0.1)
	slowL, _ := tripFuelLitres(in, trip, slow)
	fastL, _ := tripFuelLitres(in, trip, fast)
	if slowL <= fastL {
		t.Fatalf("5 km/l (%v L) should use more fuel than 10 km/l (%v L)", slowL, fastL)
	}
	if fastL != slowL/2 {
		t.Fatalf("doubling efficiency should halve fuel: %v vs %v", slowL, fastL)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func hasViolation(v TripVerdict, code domain.ConstraintCode) bool {
	for _, c := range v.Violations {
		if c == code {
			return true
		}
	}
	return false
}

func hasRule(v TripVerdict, code domain.ConstraintCode) bool {
	for _, r := range v.Results {
		if r.Code == code {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
