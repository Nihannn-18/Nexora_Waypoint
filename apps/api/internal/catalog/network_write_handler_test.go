package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

// fakeWriter is an in-memory MasterDataWriter that records what it was asked to
// persist, so the handler tests can assert RBAC and validation without a
// database.
type fakeWriter struct {
	vehicle    VehicleDetail
	outlet     OutletDetail
	created    []VehicleWrite
	createdOut []OutletWrite
	updated    []VehicleWrite
	updatedOut []OutletWrite
	depotOK    bool
	nextVeh    string
	nextOut    string
}

func (f *fakeWriter) GetVehicle(context.Context, string) (VehicleDetail, error) {
	return f.vehicle, nil
}
func (f *fakeWriter) CreateVehicle(_ context.Context, v VehicleWrite, _ string) (VehicleDetail, error) {
	f.created = append(f.created, v)
	return VehicleDetail{VehicleWrite: v, Status: "AVAILABLE"}, nil
}
func (f *fakeWriter) UpdateVehicle(_ context.Context, v VehicleWrite, _ string) (VehicleDetail, error) {
	f.updated = append(f.updated, v)
	return VehicleDetail{VehicleWrite: v, Status: "AVAILABLE"}, nil
}
func (f *fakeWriter) NextVehicleID(context.Context) (string, error) {
	if f.nextVeh == "" {
		return "VEH061", nil
	}
	return f.nextVeh, nil
}
func (f *fakeWriter) DepotExists(context.Context, string) (bool, error) { return f.depotOK, nil }

func (f *fakeWriter) GetOutlet(context.Context, string) (OutletDetail, error) {
	return f.outlet, nil
}
func (f *fakeWriter) CreateOutlet(_ context.Context, o OutletWrite, _ string) (OutletDetail, error) {
	f.createdOut = append(f.createdOut, o)
	return OutletDetail{OutletWrite: o}, nil
}
func (f *fakeWriter) UpdateOutlet(_ context.Context, o OutletWrite, _ string) (OutletDetail, error) {
	f.updatedOut = append(f.updatedOut, o)
	return OutletDetail{OutletWrite: o}, nil
}
func (f *fakeWriter) NextOutletID(context.Context) (string, error) {
	if f.nextOut == "" {
		return "OUT121", nil
	}
	return f.nextOut, nil
}

// masterMux mounts the read + master-data routes for an identity, with a writer.
func masterMux(id auth.Identity, reader NetworkReader, writer MasterDataWriter, now time.Time) http.Handler {
	mw := auth.NewMiddleware(auth.NewIdentityLoader(stubVerifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
	mux := http.NewServeMux()
	h := NewNetworkHandler(reader, fixedNow{t: now}, mw).WithWriter(writer)
	h.RegisterRoutes(mux)
	h.RegisterMasterDataRoutes(mux)
	return mux
}

func send(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newVehicleBody() map[string]any {
	return map[string]any{
		"type": "TRUCK", "tempClass": "REEFER", "weightCapKg": 3000, "volumeCapM3": 18,
		"fuelType": "diesel", "kmPerL": 6, "weeklyFuelQuotaL": 400, "depotId": "d-pel",
	}
}

func newOutletBody() map[string]any {
	return map[string]any{
		"name": "Fresh New Town", "brand": "FRESH", "district": "Colombo", "depotId": "d-pel",
		"dockType": "STREET", "parkingConstraint": "NORMAL",
		"windowOpenTime": "05:00", "windowCloseTime": "08:00",
	}
}

func TestMasterDataRBAC(t *testing.T) {
	now := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	dispatcher := auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher}
	storeMgr := auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT001"}
	loader := auth.Identity{UserID: "u-load", Role: domain.RoleLoader, DepotID: "d-pel"}
	driver := auth.Identity{UserID: "u-driv", Role: domain.RoleDriver, DepotID: "d-pel"}

	t.Run("dispatcher may create and update", func(t *testing.T) {
		w := &fakeWriter{depotOK: true}
		h := masterMux(dispatcher, &fakeNetwork{}, w, now)
		if rec := send(t, h, http.MethodPost, "/api/v1/vehicles", newVehicleBody()); rec.Code != http.StatusCreated {
			t.Fatalf("create vehicle = %d, want 201 (%s)", rec.Code, rec.Body)
		}
		if rec := send(t, h, http.MethodPatch, "/api/v1/vehicles/VEH001", newVehicleBody()); rec.Code != http.StatusOK {
			t.Fatalf("update vehicle = %d, want 200", rec.Code)
		}
		if rec := send(t, h, http.MethodPost, "/api/v1/outlets", newOutletBody()); rec.Code != http.StatusCreated {
			t.Fatalf("create outlet = %d, want 201 (%s)", rec.Code, rec.Body)
		}
		if rec := send(t, h, http.MethodPatch, "/api/v1/outlets/OUT001", newOutletBody()); rec.Code != http.StatusOK {
			t.Fatalf("update outlet = %d, want 200", rec.Code)
		}
	})

	for _, role := range []struct {
		name string
		id   auth.Identity
	}{
		{"store manager", storeMgr}, {"loader", loader}, {"driver", driver},
	} {
		t.Run(role.name+" cannot mutate vehicles or outlets", func(t *testing.T) {
			h := masterMux(role.id, &fakeNetwork{}, &fakeWriter{depotOK: true}, now)
			if rec := send(t, h, http.MethodPost, "/api/v1/vehicles", newVehicleBody()); rec.Code != http.StatusForbidden {
				t.Fatalf("create vehicle = %d, want 403", rec.Code)
			}
			if rec := send(t, h, http.MethodPatch, "/api/v1/vehicles/VEH001", newVehicleBody()); rec.Code != http.StatusForbidden {
				t.Fatalf("update vehicle = %d, want 403", rec.Code)
			}
			if rec := send(t, h, http.MethodPost, "/api/v1/outlets", newOutletBody()); rec.Code != http.StatusForbidden {
				t.Fatalf("create outlet = %d, want 403", rec.Code)
			}
			if rec := send(t, h, http.MethodPatch, "/api/v1/outlets/OUT001", newOutletBody()); rec.Code != http.StatusForbidden {
				t.Fatalf("update outlet = %d, want 403", rec.Code)
			}
		})
	}

	t.Run("unauthenticated mutations are 401", func(t *testing.T) {
		mw := auth.NewMiddleware(
			auth.NewIdentityLoader(rejectingVerifier{}, auth.NewStaticUserStore(auth.Identity{})),
			auth.NewAuthorizer())
		mux := http.NewServeMux()
		NewNetworkHandler(&fakeNetwork{}, fixedNow{t: now}, mw).WithWriter(&fakeWriter{depotOK: true}).RegisterMasterDataRoutes(mux)
		if rec := send(t, mux, http.MethodPost, "/api/v1/vehicles", newVehicleBody()); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if rec := send(t, mux, http.MethodPost, "/api/v1/outlets", newOutletBody()); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}

func TestMasterDataValidation(t *testing.T) {
	now := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	dispatcher := auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher}
	h := masterMux(dispatcher, &fakeNetwork{}, &fakeWriter{depotOK: true}, now)

	t.Run("invalid vehicle enum is 400", func(t *testing.T) {
		body := newVehicleBody()
		body["type"] = "BIKE"
		rec := send(t, h, http.MethodPost, "/api/v1/vehicles", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("invalid capacity is 400", func(t *testing.T) {
		body := newVehicleBody()
		body["weightCapKg"] = 0
		if rec := send(t, h, http.MethodPost, "/api/v1/vehicles", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown depot is 400", func(t *testing.T) {
		h := masterMux(dispatcher, &fakeNetwork{}, &fakeWriter{depotOK: false}, now)
		if rec := send(t, h, http.MethodPost, "/api/v1/vehicles", newVehicleBody()); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("invalid outlet window is 400", func(t *testing.T) {
		body := newOutletBody()
		body["windowCloseTime"] = "04:00"
		if rec := send(t, h, http.MethodPost, "/api/v1/outlets", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("mall dock without a mall window is 400", func(t *testing.T) {
		body := newOutletBody()
		body["parkingConstraint"] = "MALL_DOCK"
		body["dockType"] = "MALL_BAY"
		if rec := send(t, h, http.MethodPost, "/api/v1/outlets", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("a client-supplied identity is rejected by strict decoding", func(t *testing.T) {
		// The create payload has no identity field: identity is server-generated,
		// so a client that tries to set one is refused, not silently ignored.
		h := masterMux(dispatcher, &fakeNetwork{}, &fakeWriter{depotOK: true, nextVeh: "VEH061"}, now)
		body := newVehicleBody()
		body["vehicleId"] = "VEH999"
		rec := send(t, h, http.MethodPost, "/api/v1/vehicles", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for a client-supplied id (%s)", rec.Code, rec.Body)
		}
	})
}

// TestGetOutletStoreManagerScope proves a store manager can read their own
// outlet but not another, and cannot mutate it.
func TestGetOutletStoreManagerScope(t *testing.T) {
	now := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	storeMgr := auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT001"}
	w := &fakeWriter{outlet: OutletDetail{OutletWrite: OutletWrite{OutletID: "OUT001", Name: "Mine"}}}
	h := masterMux(storeMgr, &fakeNetwork{}, w, now)

	if rec := get(h, "/api/v1/outlets/OUT001"); rec.Code != http.StatusOK {
		t.Fatalf("own outlet = %d, want 200 (%s)", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/v1/outlets/OUT090"); rec.Code != http.StatusNotFound {
		t.Fatalf("other outlet = %d, want 404", rec.Code)
	}
	if rec := send(t, h, http.MethodPatch, "/api/v1/outlets/OUT001", newOutletBody()); rec.Code != http.StatusForbidden {
		t.Fatalf("store manager mutation = %d, want 403", rec.Code)
	}
}
