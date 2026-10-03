package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// fakeVerifier authenticates as a fixed identity so handler tests exercise the
// real auth middleware without a database.
type fakeVerifier struct{ id auth.Identity }

func (f fakeVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return f.id, nil
}

func testMiddleware() *auth.Middleware {
	identity := auth.Identity{UserID: "seed-store-manager", Role: domain.RoleStoreManager, OutletID: "OUT014"}
	return auth.NewMiddleware(
		auth.NewIdentityLoader(fakeVerifier{id: identity}, auth.NewStaticUserStore(identity)),
		auth.NewAuthorizer(),
	)
}

func testHandler(repo Repository) http.Handler {
	mux := http.NewServeMux()
	NewHandler(NewService(repo), testMiddleware()).RegisterRoutes(mux)
	return mux
}

func doGet(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	// The fake verifier ignores headers, so a request reaches the handler as the
	// seeded store manager.
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decodeItems(t *testing.T, rec *httptest.ResponseRecorder) listItemsResponse {
	t.Helper()
	var body listItemsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode items: %v (body=%q)", err, rec.Body.String())
	}
	return body
}

func TestHandlerList(t *testing.T) {
	repo := newFakeRepo(
		item("i1", "FRESH-1", domain.BrandFresh, domain.TempAmbient),
		item("i2", "STYLE-1", domain.BrandStyle, domain.TempAmbient),
	)
	h := testHandler(repo)

	t.Run("lists all items", func(t *testing.T) {
		rec := doGet(h, "/api/v1/items")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := decodeItems(t, rec)
		if len(body.Items) != 2 {
			t.Fatalf("items = %d, want 2", len(body.Items))
		}
	})

	t.Run("brand filter narrows the list", func(t *testing.T) {
		rec := doGet(h, "/api/v1/items?brand=FRESH")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := decodeItems(t, rec)
		if len(body.Items) != 1 || body.Items[0].Brand != "FRESH" {
			t.Fatalf("unexpected items: %+v", body.Items)
		}
	})

	t.Run("invalid brand is a 400 validation error", func(t *testing.T) {
		rec := doGet(h, "/api/v1/items?brand=GROCERY")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		var body httpx.ErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != httpx.CodeValidationFailed || len(body.FieldErrors) != 1 {
			t.Fatalf("body = %+v", body)
		}
	})
}

func TestHandlerGet(t *testing.T) {
	repo := newFakeRepo(item("11111111-1111-1111-1111-111111111111", "FRESH-1", domain.BrandFresh, domain.TempChilled))
	h := testHandler(repo)

	t.Run("by id", func(t *testing.T) {
		rec := doGet(h, "/api/v1/items/11111111-1111-1111-1111-111111111111")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body itemResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.SKU != "FRESH-1" || body.TemperatureRequirement != "CHILLED" {
			t.Fatalf("body = %+v", body)
		}
	})

	t.Run("by sku query", func(t *testing.T) {
		rec := doGet(h, "/api/v1/items/x?sku=FRESH-1")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("unknown id is 404", func(t *testing.T) {
		rec := doGet(h, "/api/v1/items/does-not-exist")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		var body httpx.ErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != httpx.CodeNotFound {
			t.Fatalf("code = %q, want %q", body.Code, httpx.CodeNotFound)
		}
	})
}

// TestHandlerRequiresAuthentication proves the routes are mounted behind the
// auth middleware: with the fail-closed verifier (the real TBD wiring) the
// request is refused rather than served anonymously.
func TestHandlerRequiresAuthentication(t *testing.T) {
	mux := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(NewService(newFakeRepo()), failClosed).RegisterRoutes(mux)

	rec := doGet(mux, "/api/v1/items")
	if rec.Code == http.StatusOK {
		t.Fatal("catalogue served an unverifiable request")
	}
}
