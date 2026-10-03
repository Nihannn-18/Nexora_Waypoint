package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

// stubVerifier stands in for the Better Auth session verifier so /me can be
// tested without a database.
type stubVerifier struct {
	id  auth.Identity
	err error
}

func (s stubVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return s.id, s.err
}

func meMiddleware(v auth.RequestVerifier, user auth.Identity) *auth.Middleware {
	store := auth.NewStaticUserStore(user)
	return auth.NewMiddleware(auth.NewIdentityLoader(v, store), auth.NewAuthorizer())
}

func TestMeEndpointRejectsUnauthenticated(t *testing.T) {
	loader := meMiddleware(stubVerifier{err: auth.ErrUnauthenticated}, auth.Identity{})
	h := meHandler{auth: loader}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMeEndpointReturnsEffectiveIdentity(t *testing.T) {
	user := auth.Identity{
		UserID: "seed-store-manager", Email: "ishara.s@waypoint.lk",
		Role: domain.RoleStoreManager, OutletID: "OUT014",
	}
	v := stubVerifier{id: auth.Identity{UserID: user.UserID, Email: user.Email, Name: "Ishara S."}}
	h := meHandler{auth: meMiddleware(v, user)}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Role != string(domain.RoleStoreManager) || body.OutletID == nil || *body.OutletID != "OUT014" {
		t.Fatalf("response = %+v", body)
	}
	if body.DepotID != nil {
		t.Fatalf("store manager should have null depot, got %v", *body.DepotID)
	}
	if body.Name != "Ishara S." || body.Email != user.Email {
		t.Fatalf("response = %+v", body)
	}
}
