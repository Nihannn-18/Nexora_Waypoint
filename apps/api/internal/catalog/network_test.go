package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

type fakeNetwork struct {
	gotDepot string
	gotDate  time.Time
}

func (f *fakeNetwork) Depots(context.Context) ([]Depot, error) {
	return []Depot{{DepotID: "d-kandy", Code: "KANDY", Name: "Kandy"}, {DepotID: "d-pel", Code: "PELIYAGODA", Name: "Peliyagoda"}}, nil
}

func (f *fakeNetwork) Outlets(_ context.Context, depotID string) ([]Outlet, error) {
	f.gotDepot = depotID
	return []Outlet{
		{OutletID: "OUT001", Name: "Fresh Maharagama", Brand: "FRESH", District: "Colombo", DepotID: "d-pel", DockType: "REAR_DOCK", ParkingConstraint: "NORMAL", WindowOpenTime: "05:00", WindowCloseTime: "08:00"},
		{OutletID: "OUT090", Name: "Style Mall", Brand: "STYLE", District: "Colombo", DepotID: "d-pel", DockType: "MALL_BAY", ParkingConstraint: "MALL_DOCK", MallWindowOpen: "09:00", MallWindowClose: "11:30", WindowOpenTime: "09:00", WindowCloseTime: "12:00"},
	}, nil
}

func (f *fakeNetwork) Vehicles(_ context.Context, depotID string, date time.Time) ([]Vehicle, error) {
	f.gotDepot, f.gotDate = depotID, date
	return []Vehicle{{VehicleID: "VEH014", Type: "TRUCK", TempClass: "REEFER", WeightCapKg: 2500, VolumeCapM3: 18, KmPerL: 6, WeeklyFuelQuotaL: 400, DepotID: "d-pel", Status: "IN_WORKSHOP"}}, nil
}

type fixedNow struct{ t time.Time }

func (c fixedNow) Now() time.Time { return c.t }

type stubVerifier struct{ id auth.Identity }

func (v stubVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

func networkMux(role domain.Role, reader NetworkReader, now time.Time) http.Handler {
	id := auth.Identity{UserID: "u1", Role: role}
	mw := auth.NewMiddleware(auth.NewIdentityLoader(stubVerifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
	mux := http.NewServeMux()
	NewNetworkHandler(reader, fixedNow{t: now}, mw).RegisterRoutes(mux)
	return mux
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestNetworkHandler(t *testing.T) {
	demoNow := time.Date(2026, time.September, 25, 15, 40, 0, 0, time.FixedZone("LKT", 5*3600+1800))

	t.Run("depots are listed with their internal ids", func(t *testing.T) {
		rec := get(networkMux(domain.RoleDispatcher, &fakeNetwork{}, demoNow), "/api/v1/depots")
		var body struct {
			Depots []depotResponse `json:"depots"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("status %d, err %v", rec.Code, err)
		}
		if len(body.Depots) != 2 || body.Depots[1].DepotID != "d-pel" || body.Depots[1].Code != "PELIYAGODA" {
			t.Fatalf("depots = %+v", body.Depots)
		}
	})

	t.Run("outlets carry the mall window only for mall outlets", func(t *testing.T) {
		reader := &fakeNetwork{}
		rec := get(networkMux(domain.RoleDispatcher, reader, demoNow), "/api/v1/outlets?depotId=d-pel")
		var body struct {
			Outlets []outletResponse `json:"outlets"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if reader.gotDepot != "d-pel" {
			t.Fatalf("depot filter = %q", reader.gotDepot)
		}
		if body.Outlets[0].MallWindow != nil {
			t.Fatal("a street outlet should have no mall window")
		}
		if mw := body.Outlets[1].MallWindow; mw == nil || mw.Open != "09:00" || mw.Close != "11:30" {
			t.Fatalf("mall window = %+v", mw)
		}
	})

	t.Run("vehicle status defaults to the API clock's date, not the wall clock", func(t *testing.T) {
		reader := &fakeNetwork{}
		rec := get(networkMux(domain.RoleDispatcher, reader, demoNow), "/api/v1/vehicles")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if want := time.Date(2026, time.September, 25, 0, 0, 0, 0, time.UTC); !reader.gotDate.Equal(want) {
			t.Fatalf("date = %v, want %v", reader.gotDate, want)
		}
		var body struct {
			Vehicles []vehicleResponse `json:"vehicles"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Vehicles[0].Status != "IN_WORKSHOP" || body.Vehicles[0].TempClass != "REEFER" {
			t.Fatalf("vehicle = %+v", body.Vehicles[0])
		}
	})

	t.Run("an explicit date is honoured and a bad one is 400", func(t *testing.T) {
		reader := &fakeNetwork{}
		h := networkMux(domain.RoleDispatcher, reader, demoNow)
		get(h, "/api/v1/vehicles?date=2026-09-26&depotId=d-pel")
		if reader.gotDate.Day() != 26 || reader.gotDepot != "d-pel" {
			t.Fatalf("date %v depot %q", reader.gotDate, reader.gotDepot)
		}
		if rec := get(h, "/api/v1/vehicles?date=26/09/2026"); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("other roles are refused", func(t *testing.T) {
		h := networkMux(domain.RoleDriver, &fakeNetwork{}, demoNow)
		for _, path := range []string{"/api/v1/depots", "/api/v1/outlets", "/api/v1/vehicles"} {
			if rec := get(h, path); rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status = %d, want 403", path, rec.Code)
			}
		}
	})
}
