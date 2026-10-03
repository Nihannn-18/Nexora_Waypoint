package routes

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

func dispatcher() auth.Identity {
	return auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher, DepotID: "d-peli"}
}
func storeManager() auth.Identity {
	return auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT001"}
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf)))
	return rec
}

func getReq(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerConfirm(t *testing.T) {
	svc, repo := fixture()
	repo.routes["r1"] = Route{RouteID: "r1", VehicleID: "VEH014", Status: RouteConfirmed}
	mux := http.NewServeMux()
	NewHandler(svc, repo, middlewareFor(dispatcher())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/allocations/confirm", map[string]any{
		"jobId":     "j1",
		"routes":    []map[string]any{{"vehicleId": "VEH014", "tripNo": 1, "orderIds": []string{"O1", "O2"}}},
		"deferrals": []map[string]any{{"orderId": "O3", "reasonType": "CONSTRAINT", "constraintCode": "FRESH_TIME_BUDGET", "reasonText": "no budget"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body)
	}
	var body confirmResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.RouteIDs) != 1 || len(body.AllocatedOrders) != 2 {
		t.Fatalf("body = %+v", body)
	}
}

func TestHandlerConfirmIncompleteIs400(t *testing.T) {
	svc, repo := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, repo, middlewareFor(dispatcher())).RegisterRoutes(mux)
	rec := postJSON(t, mux, "/api/v1/allocations/confirm", map[string]any{
		"jobId":  "j1",
		"routes": []map[string]any{{"vehicleId": "VEH014", "tripNo": 1, "orderIds": []string{"O1"}}},
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

func TestHandlerRequiresDispatcher(t *testing.T) {
	svc, repo := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, repo, middlewareFor(storeManager())).RegisterRoutes(mux)
	rec := postJSON(t, mux, "/api/v1/allocations/confirm", map[string]any{"jobId": "j1"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("store manager status = %d, want 403", rec.Code)
	}

	// Fail-closed verifier must refuse.
	mux2 := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(svc, repo, failClosed).RegisterRoutes(mux2)
	if got := getReq(mux2, "/api/v1/routes/r1"); got.Code == http.StatusOK {
		t.Fatal("routes served an unverifiable request")
	}
}

func TestHandlerGetRouteAndLegs(t *testing.T) {
	svc, repo := fixture()
	repo.routes["r1"] = Route{
		RouteID: "r1", VehicleID: "VEH014", Status: RouteConfirmed, TripNo: 1,
		Legs: []RouteLeg{{LegID: "l1", Seq: 0, FromPoint: "DEPOT", ToOutlet: "OUT001", Status: LegPending}},
	}
	mux := http.NewServeMux()
	NewHandler(svc, repo, middlewareFor(dispatcher())).RegisterRoutes(mux)

	if got := getReq(mux, "/api/v1/routes/r1"); got.Code != http.StatusOK {
		t.Fatalf("get route status = %d", got.Code)
	}
	if got := getReq(mux, "/api/v1/routes/r1/legs"); got.Code != http.StatusOK {
		t.Fatalf("get legs status = %d", got.Code)
	}
	if got := getReq(mux, "/api/v1/routes/missing"); got.Code != http.StatusNotFound {
		t.Fatalf("missing route status = %d, want 404", got.Code)
	}
}

func TestHandlerListRoutesRequiresDate(t *testing.T) {
	svc, repo := fixture()
	mux := http.NewServeMux()
	NewHandler(svc, repo, middlewareFor(dispatcher())).RegisterRoutes(mux)
	if got := getReq(mux, "/api/v1/routes"); got.Code != http.StatusBadRequest {
		t.Fatalf("no date status = %d, want 400", got.Code)
	}
}
