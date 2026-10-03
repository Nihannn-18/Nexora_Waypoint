package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

type verifier struct{ id auth.Identity }

func (v verifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

func middlewareFor(id auth.Identity) *auth.Middleware {
	return auth.NewMiddleware(auth.NewIdentityLoader(verifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
}

func driver() auth.Identity {
	return auth.Identity{UserID: "u-driver", Role: domain.RoleDriver, DepotID: "d-peli"}
}
func loader() auth.Identity {
	return auth.Identity{UserID: "u-loader", Role: domain.RoleLoader, DepotID: "d-peli"}
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf)))
	return rec
}

func TestHandlerRecordEvent(t *testing.T) {
	svc, _ := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{
		"clientEventId": "EV1", "outcome": "DELIVERED",
		"occurredAt": "2026-09-26T07:42:00+05:30", "createdOffline": true,
		"proofOfDelivery": map[string]any{"type": "PHOTO", "receiverName": "Nimal", "fileRef": "pod/LEG1/abc"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body)
	}
	var body eventResultResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != SyncAccepted {
		t.Fatalf("status = %q, want ACCEPTED", body.Status)
	}
}

func TestHandlerRecordEventValidation(t *testing.T) {
	svc, _ := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{
		"clientEventId": "EV1", "outcome": "DELIVERED",
		"occurredAt": "2026-09-26T07:42:00+05:30",
		// no receiver / no POD -> invalid
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != httpx.CodeValidationFailed {
		t.Fatalf("code = %q, want %q", body.Code, httpx.CodeValidationFailed)
	}
}

func TestHandlerSyncEvents(t *testing.T) {
	svc, repo := fixture()
	repo.results["EV2"] = EventResult{ClientEventID: "EV2", Status: SyncDuplicate, ServerEventID: "E2"}
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/sync/events", map[string]any{
		"events": []map[string]any{
			{"legId": "LEG1", "clientEventId": "EV1", "outcome": "DELIVERED", "occurredAt": "2026-09-26T07:42:00+05:30",
				"proofOfDelivery": map[string]any{"type": "PHOTO", "receiverName": "N", "fileRef": "pod/LEG1/a"}},
			{"legId": "LEG1", "clientEventId": "EV2", "outcome": "FAILED", "reasonCode": "OUTLET_CLOSED", "occurredAt": "2026-09-26T07:50:00+05:30"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body)
	}
	var body syncEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 2 || body.Results[0].Status != SyncAccepted || body.Results[1].Status != SyncDuplicate {
		t.Fatalf("body = %+v", body.Results)
	}
}

func TestHandlerRequiresDriver(t *testing.T) {
	svc, _ := fixture()

	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux)
	if rec := postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{}); rec.Code != http.StatusForbidden {
		t.Fatalf("loader status = %d, want 403", rec.Code)
	}

	mux2 := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(svc, failClosed).RegisterRoutes(mux2)
	if rec := postJSON(t, mux2, "/api/v1/legs/LEG1/events", map[string]any{}); rec.Code == http.StatusOK {
		t.Fatal("delivery served an unverifiable request")
	}
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerFailedReasonCode(t *testing.T) {
	svc, repo := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/sync/events", map[string]any{"events": []map[string]any{{
		"legId": "LEG1", "clientEventId": "EVF", "outcome": "FAILED", "reasonCode": "REFUSED_BY_STORE",
		"occurredAt": "2026-09-26T07:50:00+05:30", "createdOffline": true,
	}}})
	if rec.Code != http.StatusOK || repo.recorded == nil || repo.recorded.ReasonCode != "REFUSED_BY_STORE" {
		t.Fatalf("status=%d recorded=%+v", rec.Code, repo.recorded)
	}

	rec = postJSON(t, mux, "/api/v1/legs/LEG1/events", map[string]any{
		"clientEventId": "EVF0", "outcome": "FAILED", "occurredAt": "2026-09-26T07:50:00+05:30",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("failed without reason = %d, want 400", rec.Code)
	}
}

func TestHandlerGetLeg(t *testing.T) {
	svc, repo := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := get(mux, "/api/v1/legs/LEG1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body)
	}
	var body legResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Route.VehicleID != "VEH014" || body.Outlet.WindowClose != "08:00" ||
		len(body.Orders) != 1 || body.Orders[0].Lines[0].Quantity != 10 || body.PlannedArrival != "" {
		t.Fatalf("body = %+v", body)
	}

	// Another depot's driver gets 404, never the data.
	other := driver()
	other.DepotID = "d-kandy"
	mux2 := http.NewServeMux()
	NewHandler(svc, middlewareFor(other)).RegisterRoutes(mux2)
	if rec := get(mux2, "/api/v1/legs/LEG1"); rec.Code != http.StatusNotFound {
		t.Fatalf("other depot = %d, want 404", rec.Code)
	}

	mux3 := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux3)
	if rec := get(mux3, "/api/v1/legs/LEG1"); rec.Code != http.StatusForbidden {
		t.Fatalf("loader = %d, want 403", rec.Code)
	}
	mux4 := http.NewServeMux()
	NewHandler(svc, auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())).RegisterRoutes(mux4)
	if rec := get(mux4, "/api/v1/legs/LEG1"); rec.Code == http.StatusOK {
		t.Fatal("served an unverifiable request")
	}

	repo.legErr = ErrNotFound
	if rec := get(mux, "/api/v1/legs/NOPE"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing leg = %d, want 404", rec.Code)
	}
}

func TestHandlerDriverRoutes(t *testing.T) {
	svc, repo := fixture()
	repo.routes = []DriverRoute{{
		RouteSummary: RouteSummary{RouteID: "R1", RouteDate: "2026-09-26", VehicleID: "VEH014", TripNo: 1},
		Stops:        []RouteStop{{LegID: "LEG1", OutletID: "OUT014", WindowOpen: "05:00", WindowClose: "08:00", Status: "PENDING"}},
	}}
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)

	rec := get(mux, "/api/v1/driver/routes?date=2026-09-26")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body)
	}
	var body []driverRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body[0].VehicleID != "VEH014" || body[0].Stops[0].LegID != "LEG1" {
		t.Fatalf("body = %s", rec.Body)
	}
	if repo.routesArg[0] != "d-peli" {
		t.Fatalf("depot = %q, want the caller's depot", repo.routesArg[0])
	}
	if rec := get(mux, "/api/v1/driver/routes"); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing date = %d, want 400", rec.Code)
	}

	mux2 := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux2)
	if rec := get(mux2, "/api/v1/driver/routes?date=2026-09-26"); rec.Code != http.StatusForbidden {
		t.Fatalf("loader = %d, want 403", rec.Code)
	}
}

func TestHandlerSyncStatus(t *testing.T) {
	svc, repo := fixture()
	repo.status = SyncStatus{Synced: 3}
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(driver())).RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sync/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
