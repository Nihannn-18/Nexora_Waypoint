package loading

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

func loader() auth.Identity {
	return auth.Identity{UserID: "u-loader", Role: domain.RoleLoader, DepotID: "d-peli"}
}
func dispatcher() auth.Identity {
	return auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher, DepotID: "d-peli"}
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

func TestHandlerPickingList(t *testing.T) {
	svc, _ := serviceFixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux)

	rec := getReq(mux, "/api/v1/routes/R1/loading")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body)
	}
	var body routeLoadingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RouteID != "R1" || len(body.Lines) != 1 {
		t.Fatalf("body = %+v", body)
	}
}

func TestHandlerRecordShortfalls(t *testing.T) {
	svc, _ := serviceFixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/routes/R1/shortfalls", map[string]any{
		"items": []map[string]any{{"orderItemId": "OI1", "loadedQty": 8, "damagedQty": 0, "missingQty": 2}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body)
	}
}

func TestHandlerRejectsBadSubmission(t *testing.T) {
	svc, _ := serviceFixture()
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(loader())).RegisterRoutes(mux)

	rec := postJSON(t, mux, "/api/v1/routes/R1/shortfalls", map[string]any{"items": []map[string]any{}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty items status = %d, want 400", rec.Code)
	}
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != httpx.CodeValidationFailed {
		t.Fatalf("code = %q, want %q", body.Code, httpx.CodeValidationFailed)
	}
}

func TestHandlerRequiresLoader(t *testing.T) {
	svc, _ := serviceFixture()

	// A dispatcher (not a loader) is forbidden.
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(dispatcher())).RegisterRoutes(mux)
	if rec := getReq(mux, "/api/v1/routes/R1/loading"); rec.Code != http.StatusForbidden {
		t.Fatalf("dispatcher status = %d, want 403", rec.Code)
	}

	// Fail-closed verifier refuses.
	mux2 := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(svc, failClosed).RegisterRoutes(mux2)
	if rec := getReq(mux2, "/api/v1/routes/R1/loading"); rec.Code == http.StatusOK {
		t.Fatal("loading served an unverifiable request")
	}
}
