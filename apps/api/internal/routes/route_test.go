package routes

import (
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

func validRoute() Route {
	return Route{
		VehicleID: "VEH014",
		DepotID:   "d-peli",
		RouteDate: "2026-09-26",
		TripNo:    1,
		Brand:     domain.BrandFresh,
		District:  "Colombo",
		Status:    RouteConfirmed,
		Legs: []RouteLeg{
			{OrderID: "O1", Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUT001"},
			{OrderID: "O2", Seq: 1, FromPoint: "OUT001", ToOutlet: "OUT002"},
		},
	}
}

func TestRouteValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Route)
		wantErr bool
		field   string
	}{
		{"valid", func(*Route) {}, false, ""},
		{"missing vehicle", func(r *Route) { r.VehicleID = "" }, true, "vehicleId"},
		{"missing depot", func(r *Route) { r.DepotID = "" }, true, "depotId"},
		{"missing date", func(r *Route) { r.RouteDate = "" }, true, "routeDate"},
		{"trip 0", func(r *Route) { r.TripNo = 0 }, true, "tripNo"},
		{"trip 3", func(r *Route) { r.TripNo = 3 }, true, "tripNo"},
		{"bad brand", func(r *Route) { r.Brand = "GROCERY" }, true, "brand"},
		{"missing district", func(r *Route) { r.District = "" }, true, "district"},
		{"bad status", func(r *Route) { r.Status = "PLANNED" }, true, "status"},
		{"no legs", func(r *Route) { r.Legs = nil }, true, "legs"},
		{"non-contiguous seq", func(r *Route) { r.Legs[1].Seq = 5 }, true, "legs[1].seq"},
		{"missing order", func(r *Route) { r.Legs[0].OrderID = "" }, true, "legs[0].orderId"},
		{"missing outlet", func(r *Route) { r.Legs[0].ToOutlet = "" }, true, "legs[0].toOutlet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRoute()
			tt.mutate(&r)
			err := r.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("error %v does not wrap ErrInvalid", err)
				}
				var ve ValidationError
				if !errors.As(err, &ve) || ve.Field != tt.field {
					t.Fatalf("field = %q, want %q (err=%v)", ve.Field, tt.field, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateLegOrder(t *testing.T) {
	good := []RouteLeg{
		{Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUT001"},
		{Seq: 1, FromPoint: "OUT001", ToOutlet: "OUT002"},
		{Seq: 2, FromPoint: "OUT002", ToOutlet: "OUT003"},
	}
	if err := ValidateLegOrder(good); err != nil {
		t.Fatalf("valid order rejected: %v", err)
	}

	badSeq := []RouteLeg{{Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUT001"}, {Seq: 2, FromPoint: "OUT001", ToOutlet: "OUT002"}}
	if err := ValidateLegOrder(badSeq); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-contiguous seq accepted: %v", err)
	}

	badFrom := []RouteLeg{{Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUT001"}, {Seq: 1, FromPoint: "DEPOT", ToOutlet: "OUT002"}}
	if err := ValidateLegOrder(badFrom); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong fromPoint accepted: %v", err)
	}
}

func TestValidStatusAndTripNo(t *testing.T) {
	for _, s := range []string{RouteDraft, RouteConfirmed, RouteDispatched, RouteInTransit, RouteCompleted, RouteCancelled} {
		if !ValidRouteStatus(s) {
			t.Errorf("%s should be valid", s)
		}
	}
	for _, s := range []string{"", "PLANNED", "draft"} {
		if ValidRouteStatus(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
	if !ValidTripNo(1) || !ValidTripNo(2) || ValidTripNo(0) || ValidTripNo(3) {
		t.Fatal("trip number validity is wrong")
	}
}
