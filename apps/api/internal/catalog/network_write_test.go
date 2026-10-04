package catalog

import (
	"errors"
	"strings"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// TestValidateVehicle pins the server-side vehicle rules to the schema's CHECK
// constraints and the domain enums. Invalid data must be rejected here, before
// it can reach PostgreSQL.
func TestValidateVehicle(t *testing.T) {
	valid := func() VehicleWrite {
		return VehicleWrite{
			VehicleID: "VEH061", Type: domain.VehicleTruck, TempClass: domain.VehicleTempReefer,
			WeightCapKg: 3000, VolumeCapM3: 18, FuelType: "diesel", KmPerL: 6,
			WeeklyFuelQuotaL: 400, DepotID: "d-pel",
		}
	}

	t.Run("a complete vehicle passes", func(t *testing.T) {
		if err := ValidateVehicle(valid(), true); err != nil {
			t.Fatalf("valid vehicle rejected: %v", err)
		}
	})

	t.Run("equality at the capacity bounds passes", func(t *testing.T) {
		v := valid()
		v.WeightCapKg, v.VolumeCapM3 = 0.01, 0.01
		if err := ValidateVehicle(v, true); err != nil {
			t.Fatalf("minimum positive capacities rejected: %v", err)
		}
		v = valid()
		v.WeeklyFuelQuotaL = 0 // quota >= 0, zero allowed
		if err := ValidateVehicle(v, true); err != nil {
			t.Fatalf("zero fuel quota rejected: %v", err)
		}
	})

	cases := []struct {
		name  string
		mut   func(*VehicleWrite)
		field string
	}{
		{"bad id shape", func(v *VehicleWrite) { v.VehicleID = "VEH61" }, "vehicleId"},
		{"bad type", func(v *VehicleWrite) { v.Type = "BIKE" }, "type"},
		{"bad temp", func(v *VehicleWrite) { v.TempClass = "COLD" }, "tempClass"},
		{"zero weight", func(v *VehicleWrite) { v.WeightCapKg = 0 }, "weightCapKg"},
		{"negative weight", func(v *VehicleWrite) { v.WeightCapKg = -1 }, "weightCapKg"},
		{"zero volume", func(v *VehicleWrite) { v.VolumeCapM3 = 0 }, "volumeCapM3"},
		{"empty fuel type", func(v *VehicleWrite) { v.FuelType = "  " }, "fuelType"},
		{"zero km/l", func(v *VehicleWrite) { v.KmPerL = 0 }, "kmPerL"},
		{"negative fuel quota", func(v *VehicleWrite) { v.WeeklyFuelQuotaL = -1 }, "weeklyFuelQuotaL"},
		{"empty depot", func(v *VehicleWrite) { v.DepotID = "" }, "depotId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := valid()
			tc.mut(&v)
			err := ValidateVehicle(v, true)
			var ve ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %v, want ValidationError on %q", err, tc.field)
			}
		})
	}

	t.Run("update does not require an id in the payload", func(t *testing.T) {
		v := valid()
		v.VehicleID = "" // the path supplies it; ValidateVehicle(false) skips the shape check
		if err := ValidateVehicle(v, false); err != nil {
			t.Fatalf("update payload rejected: %v", err)
		}
	})
}

// TestValidateOutlet pins the outlet rules, including the window and mall-window
// pairing that the seed convention requires.
func TestValidateOutlet(t *testing.T) {
	valid := func() OutletWrite {
		return OutletWrite{
			OutletID: "OUT121", Name: "Fresh New Town", Brand: domain.BrandFresh,
			District: "Colombo", DepotID: "d-pel", DockType: domain.DockStreet,
			ParkingConstraint: domain.ParkingNormal,
			WindowOpenTime:    "05:00", WindowCloseTime: "08:00",
		}
	}

	t.Run("a complete outlet passes", func(t *testing.T) {
		if err := ValidateOutlet(valid(), true); err != nil {
			t.Fatalf("valid outlet rejected: %v", err)
		}
	})

	t.Run("a mall dock outlet with a mall window passes", func(t *testing.T) {
		o := valid()
		o.DockType, o.ParkingConstraint = domain.DockMallBay, domain.ParkingMallDock
		o.MallWindowOpen, o.MallWindowClose = "10:00", "12:00"
		if err := ValidateOutlet(o, true); err != nil {
			t.Fatalf("valid mall outlet rejected: %v", err)
		}
	})

	t.Run("window close exactly after open passes", func(t *testing.T) {
		o := valid()
		o.WindowOpenTime, o.WindowCloseTime = "05:00", "05:01"
		if err := ValidateOutlet(o, true); err != nil {
			t.Fatalf("one-minute window rejected: %v", err)
		}
	})

	cases := []struct {
		name  string
		mut   func(*OutletWrite)
		field string
	}{
		{"bad id shape", func(o *OutletWrite) { o.OutletID = "OUT12" }, "outletId"},
		{"empty name", func(o *OutletWrite) { o.Name = " " }, "name"},
		{"bad brand", func(o *OutletWrite) { o.Brand = "GROCERY" }, "brand"},
		{"empty district", func(o *OutletWrite) { o.District = "" }, "district"},
		{"empty depot", func(o *OutletWrite) { o.DepotID = "" }, "depotId"},
		{"bad dock", func(o *OutletWrite) { o.DockType = "LOADING" }, "dockType"},
		{"bad parking", func(o *OutletWrite) { o.ParkingConstraint = "TRUCK_ONLY" }, "parkingConstraint"},
		{"bad open time", func(o *OutletWrite) { o.WindowOpenTime = "5:00" }, "windowOpenTime"},
		{"bad close time", func(o *OutletWrite) { o.WindowCloseTime = "25:00" }, "windowCloseTime"},
		{"close not after open", func(o *OutletWrite) { o.WindowCloseTime = "05:00" }, "windowCloseTime"},
		{"mall dock without window", func(o *OutletWrite) {
			o.ParkingConstraint = domain.ParkingMallDock
		}, "mallWindow"},
		{"non-mall with mall window", func(o *OutletWrite) {
			o.MallWindowOpen, o.MallWindowClose = "10:00", "12:00"
		}, "mallWindow"},
		{"mall window half set", func(o *OutletWrite) {
			o.ParkingConstraint = domain.ParkingMallDock
			o.MallWindowOpen = "10:00"
		}, "mallWindow"},
		{"bad mall close time", func(o *OutletWrite) {
			o.ParkingConstraint = domain.ParkingMallDock
			o.MallWindowOpen, o.MallWindowClose = "10:00", "bad"
		}, "mallWindowClose"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := valid()
			tc.mut(&o)
			err := ValidateOutlet(o, true)
			var ve ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("err = %v, want ValidationError on %q", err, tc.field)
			}
		})
	}
}

// TestIdentityPatterns guards the fixed identifier conventions the whole system
// relies on.
func TestIdentityPatterns(t *testing.T) {
	if !validVehicleID("VEH001") || !validVehicleID("VEH999") || validVehicleID("VEH1") || validVehicleID("veh001") {
		t.Fatal("vehicle id pattern is wrong")
	}
	if !validOutletID("OUT001") || !validOutletID("OUT120") || validOutletID("OUT12") || validOutletID("outlet1") {
		t.Fatal("outlet id pattern is wrong")
	}
	if strings.TrimSpace("") != "" {
		t.Fatal("sanity")
	}
}
