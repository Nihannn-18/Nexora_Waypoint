package orders

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/catalog"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// verifier authenticates as a fixed identity so the real auth middleware runs
// without a database.
type verifier struct{ id auth.Identity }

func (v verifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

func middlewareFor(id auth.Identity) *auth.Middleware {
	return auth.NewMiddleware(
		auth.NewIdentityLoader(verifier{id: id}, auth.NewStaticUserStore(id)),
		auth.NewAuthorizer(),
	)
}

// handlerFor builds a handler whose service uses the same fakes as the service
// tests, plus the given caller identity.
func handlerFor(t *testing.T, id auth.Identity, now time.Time) (http.Handler, *fakeRepo) {
	t.Helper()
	svc, repo, _ := fixture(now)
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(id)).RegisterRoutes(mux)
	return mux, repo
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf)))
	return rec
}

func getReq(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func storeManager() auth.Identity {
	return auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT001"}
}

func dispatcher() auth.Identity {
	return auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher, DepotID: "d1"}
}

func TestHandlerCreate(t *testing.T) {
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)

	t.Run("creates an order for the caller's outlet", func(t *testing.T) {
		h, _ := handlerFor(t, storeManager(), before)
		rec := postJSON(t, h, "/api/v1/orders", map[string]any{
			"outletId":              "OUT001",
			"requestedDeliveryDate": "2026-09-26",
			"items":                 []map[string]any{{"itemId": "i-amb", "quantity": 3}},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body)
		}
		var body orderResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != "PLACED" || body.TotalUnits != 3 || body.OutletID != "OUT001" {
			t.Fatalf("body = %+v", body)
		}
		if body.OrderNumber == "" {
			t.Fatal("order number not set")
		}
	})

	t.Run("store manager cannot order for another outlet", func(t *testing.T) {
		h, _ := handlerFor(t, storeManager(), before)
		rec := postJSON(t, h, "/api/v1/orders", map[string]any{
			"outletId":              "OUT050",
			"requestedDeliveryDate": "2026-09-26",
			"items":                 []map[string]any{{"itemId": "i-style", "quantity": 1}},
		})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("dispatcher may order for any outlet", func(t *testing.T) {
		h, _ := handlerFor(t, dispatcher(), before)
		rec := postJSON(t, h, "/api/v1/orders", map[string]any{
			"outletId":              "OUT050",
			"requestedDeliveryDate": "2026-09-26",
			"items":                 []map[string]any{{"itemId": "i-style", "quantity": 1}},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body)
		}
	})

	t.Run("bad delivery date is a 400", func(t *testing.T) {
		h, _ := handlerFor(t, storeManager(), before)
		rec := postJSON(t, h, "/api/v1/orders", map[string]any{
			"outletId":              "OUT001",
			"requestedDeliveryDate": "26-09-2026",
			"items":                 []map[string]any{{"itemId": "i-amb", "quantity": 1}},
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown item is a 400 validation error", func(t *testing.T) {
		h, _ := handlerFor(t, storeManager(), before)
		rec := postJSON(t, h, "/api/v1/orders", map[string]any{
			"outletId":              "OUT001",
			"requestedDeliveryDate": "2026-09-26",
			"items":                 []map[string]any{{"itemId": "ghost", "quantity": 1}},
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("malformed JSON is a 400", func(t *testing.T) {
		h, _ := handlerFor(t, storeManager(), before)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader([]byte("{not json"))))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestHandlerGetAndConfirm(t *testing.T) {
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	h, repo := handlerFor(t, storeManager(), before)

	rec := postJSON(t, h, "/api/v1/orders", map[string]any{
		"outletId":              "OUT001",
		"requestedDeliveryDate": "2026-09-26",
		"items":                 []map[string]any{{"itemId": "i-amb", "quantity": 1}},
	})
	var created orderResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	t.Run("get returns the order", func(t *testing.T) {
		got := getReq(h, "/api/v1/orders/"+created.OrderID)
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", got.Code, got.Body)
		}
	})

	t.Run("unknown order is 404", func(t *testing.T) {
		got := getReq(h, "/api/v1/orders/missing")
		if got.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got.Code)
		}
		var body httpx.ErrorBody
		if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != httpx.CodeNotFound {
			t.Fatalf("code = %q, want %q", body.Code, httpx.CodeNotFound)
		}
	})

	t.Run("confirm transitions to CONFIRMED", func(t *testing.T) {
		got := postJSON(t, h, "/api/v1/orders/"+created.OrderID+"/confirm", nil)
		if got.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", got.Code, got.Body)
		}
		var body orderResponse
		if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != "CONFIRMED" {
			t.Fatalf("status = %s, want CONFIRMED", body.Status)
		}
	})

	t.Run("confirming again is 409", func(t *testing.T) {
		got := postJSON(t, h, "/api/v1/orders/"+created.OrderID+"/confirm", nil)
		if got.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", got.Code)
		}
		var body httpx.ErrorBody
		if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != httpx.CodeConflict {
			t.Fatalf("code = %q, want %q", body.Code, httpx.CodeConflict)
		}
	})
	_ = repo
}

// TestHandlerRequiresAuthentication proves the routes sit behind the auth
// middleware: with the fail-closed verifier the request is refused.
func TestHandlerRequiresAuthentication(t *testing.T) {
	svc := NewService(newFakeRepo(), &fakeCatalogue{items: map[string]catalog.Item{}}, fakeOutlets{}, fixedClock{})
	mux := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(svc, failClosed).RegisterRoutes(mux)

	rec := getReq(mux, "/api/v1/orders/anything")
	if rec.Code == http.StatusOK {
		t.Fatal("orders served an unverifiable request")
	}
}
