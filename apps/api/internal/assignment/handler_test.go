package assignment

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

type stubVerifier struct{ id auth.Identity }

func (v stubVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

type rejectingVerifier struct{}

func (rejectingVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return auth.Identity{}, auth.ErrUnauthenticated
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func assignMux(id auth.Identity, store Store, now time.Time) http.Handler {
	mw := auth.NewMiddleware(auth.NewIdentityLoader(stubVerifier{id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
	mux := http.NewServeMux()
	NewHandler(NewService(store, fixedClock{now}), mw).RegisterRoutes(mux)
	return mux
}

func unauthMux(store Store, now time.Time) http.Handler {
	mw := auth.NewMiddleware(auth.NewIdentityLoader(rejectingVerifier{}, auth.NewStaticUserStore()), auth.NewAuthorizer())
	mux := http.NewServeMux()
	NewHandler(NewService(store, fixedClock{now}), mw).RegisterRoutes(mux)
	return mux
}

func sendJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
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

func doGet(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestAssignmentRBAC(t *testing.T) {
	now := time.Date(2026, time.September, 26, 5, 0, 0, 0, time.UTC)
	dispatcher := auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher}

	t.Run("dispatcher manages a driver assignment", func(t *testing.T) {
		store := newFakeStore()
		h := assignMux(dispatcher, store, now)
		if rec := doGet(h, "/api/v1/drivers"); rec.Code != http.StatusOK {
			t.Fatalf("list drivers = %d", rec.Code)
		}
		rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH014/assignment",
			map[string]any{"driverId": "u-driv", "date": "2026-09-26"})
		if rec.Code != http.StatusOK {
			t.Fatalf("assign = %d (%s)", rec.Code, rec.Body)
		}
		if rec := doGet(h, "/api/v1/vehicles/VEH014/assignment?date=2026-09-26"); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("u-driv")) {
			t.Fatalf("read assignment = %d (%s)", rec.Code, rec.Body)
		}
		if rec := sendJSON(t, h, http.MethodDelete, "/api/v1/vehicles/VEH014/assignment?date=2026-09-26", nil); rec.Code != http.StatusOK {
			t.Fatalf("unassign = %d (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("dispatcher manages an outlet manager", func(t *testing.T) {
		store := newFakeStore()
		h := assignMux(dispatcher, store, now)
		rec := sendJSON(t, h, http.MethodPut, "/api/v1/outlets/OUT014/manager", map[string]any{"userId": "u-store"})
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("u-store")) {
			t.Fatalf("assign manager = %d (%s)", rec.Code, rec.Body)
		}
		if rec := sendJSON(t, h, http.MethodDelete, "/api/v1/outlets/OUT014/manager", nil); rec.Code != http.StatusOK {
			t.Fatalf("remove manager = %d (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("dispatcher manages a loader depot", func(t *testing.T) {
		store := newFakeStore()
		h := assignMux(dispatcher, store, now)
		if rec := doGet(h, "/api/v1/loaders"); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("u-load")) {
			t.Fatalf("list loaders = %d (%s)", rec.Code, rec.Body)
		}
		rec := sendJSON(t, h, http.MethodPut, "/api/v1/loaders/u-load2/depot", map[string]any{"depotId": "d-kandy"})
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("d-kandy")) {
			t.Fatalf("assign loader depot = %d (%s)", rec.Code, rec.Body)
		}
		if rec := doGet(h, "/api/v1/loaders/u-load2/depot"); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("d-kandy")) {
			t.Fatalf("read loader depot = %d (%s)", rec.Code, rec.Body)
		}
		if rec := sendJSON(t, h, http.MethodDelete, "/api/v1/loaders/u-load2/depot", nil); rec.Code != http.StatusOK {
			t.Fatalf("remove loader depot = %d (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("dispatcher lists the day's driver assignments", func(t *testing.T) {
		store := newFakeStore()
		h := assignMux(dispatcher, store, now)
		if rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH014/assignment", map[string]any{"driverId": "u-driv", "date": "2026-09-26"}); rec.Code != http.StatusOK {
			t.Fatal(rec.Body)
		}
		rec := doGet(h, "/api/v1/driver-vehicle-assignments?date=2026-09-26")
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("VEH014")) {
			t.Fatalf("list assignments = %d (%s)", rec.Code, rec.Body)
		}
	})

	for _, role := range []struct {
		name string
		id   auth.Identity
	}{
		{"loader", auth.Identity{UserID: "u-load", Role: domain.RoleLoader, DepotID: "d-pel"}},
		{"driver", auth.Identity{UserID: "u-driv", Role: domain.RoleDriver, DepotID: "d-pel"}},
		{"store manager", auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT014"}},
	} {
		t.Run(role.name+" cannot mutate assignments", func(t *testing.T) {
			h := assignMux(role.id, newFakeStore(), now)
			if rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH014/assignment", map[string]any{"driverId": "u-driv", "date": "2026-09-26"}); rec.Code != http.StatusForbidden {
				t.Fatalf("assign driver = %d, want 403", rec.Code)
			}
			if rec := sendJSON(t, h, http.MethodPut, "/api/v1/outlets/OUT014/manager", map[string]any{"userId": "u-store"}); rec.Code != http.StatusForbidden {
				t.Fatalf("assign manager = %d, want 403", rec.Code)
			}
			if rec := doGet(h, "/api/v1/drivers"); rec.Code != http.StatusForbidden {
				t.Fatalf("list drivers = %d, want 403", rec.Code)
			}
			if rec := sendJSON(t, h, http.MethodPut, "/api/v1/loaders/u-load2/depot", map[string]any{"depotId": "d-kandy"}); rec.Code != http.StatusForbidden {
				t.Fatalf("assign loader depot = %d, want 403", rec.Code)
			}
		})
	}

	t.Run("a loader reads only their own depot", func(t *testing.T) {
		store := newFakeStore()
		h := assignMux(auth.Identity{UserID: "u-load", Role: domain.RoleLoader, DepotID: "d-pel"}, store, now)
		rec := doGet(h, "/api/v1/loader/assignment")
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("d-pel")) {
			t.Fatalf("own loader assignment = %d (%s)", rec.Code, rec.Body)
		}
		if rec := sendJSON(t, h, http.MethodPut, "/api/v1/loaders/u-load/depot", map[string]any{"depotId": "d-kandy"}); rec.Code != http.StatusForbidden {
			t.Fatalf("loader cannot mutate own depot = %d, want 403", rec.Code)
		}
	})

	t.Run("a driver reads only their own assignment", func(t *testing.T) {
		store := newFakeStore()
		store.driverAssign["u-driv|2026-09-26"] = VehicleAssignment{
			VehicleID: "VEH014", AssignmentDate: "2026-09-26", DepotID: "d-pel",
			Driver: Driver{UserID: "u-driv", Name: "Kasun P."},
		}
		driver := auth.Identity{UserID: "u-driv", Role: domain.RoleDriver, DepotID: "d-pel"}
		h := assignMux(driver, store, now)
		rec := doGet(h, "/api/v1/driver/assignment?date=2026-09-26")
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("VEH014")) {
			t.Fatalf("own assignment = %d (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("unauthenticated is 401", func(t *testing.T) {
		h := unauthMux(newFakeStore(), now)
		if rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH014/assignment", map[string]any{"driverId": "u-driv", "date": "2026-09-26"}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("assign driver = %d, want 401", rec.Code)
		}
		if rec := doGet(h, "/api/v1/driver/assignment"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("driver assignment = %d, want 401", rec.Code)
		}
	})
}

func TestAssignmentErrors(t *testing.T) {
	now := time.Date(2026, time.September, 26, 5, 0, 0, 0, time.UTC)
	dispatcher := auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher}

	t.Run("bad date is 400", func(t *testing.T) {
		h := assignMux(dispatcher, newFakeStore(), now)
		rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH014/assignment", map[string]any{"driverId": "u-driv", "date": "26 Sep"})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("a driver already on another vehicle is 409", func(t *testing.T) {
		store := newFakeStore()
		h := assignMux(dispatcher, store, now)
		if rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH014/assignment", map[string]any{"driverId": "u-driv", "date": "2026-09-26"}); rec.Code != http.StatusOK {
			t.Fatal(rec.Body)
		}
		rec := sendJSON(t, h, http.MethodPut, "/api/v1/vehicles/VEH001/assignment", map[string]any{"driverId": "u-driv", "date": "2026-09-26"})
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body)
		}
	})

	t.Run("removing a missing assignment is 404", func(t *testing.T) {
		h := assignMux(dispatcher, newFakeStore(), now)
		rec := sendJSON(t, h, http.MethodDelete, "/api/v1/vehicles/VEH014/assignment?date=2026-09-26", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("assigning a non-manager is 400", func(t *testing.T) {
		h := assignMux(dispatcher, newFakeStore(), now)
		rec := sendJSON(t, h, http.MethodPut, "/api/v1/outlets/OUT014/manager", map[string]any{"userId": "u-driv"})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}
