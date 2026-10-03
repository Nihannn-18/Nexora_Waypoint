package authapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

type fakeVerifier struct {
	identity auth.Identity
	err      error
}

func (f fakeVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return f.identity, f.err
}

func newTestHandler(t *testing.T, store Store, verifier auth.RequestVerifier, user auth.Identity) (*Handler, *auth.Middleware) {
	t.Helper()
	loader := auth.NewIdentityLoader(verifier, auth.NewStaticUserStore(user))
	mw := auth.NewMiddleware(loader, auth.NewAuthorizer())
	return NewHandler(NewService(store, time.Hour), mw), mw
}

func doRequest(h *Handler, mw *auth.Middleware, method, path, body, token string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestLoginHandler(t *testing.T) {
	identity := auth.Identity{UserID: "seed-dispatcher", Name: "Priyantha W.", Email: "priyantha.w@waypoint.lk", Role: domain.RoleDispatcher, DepotID: "d-peli"}

	t.Run("valid credentials return a token and identity", func(t *testing.T) {
		store := &fakeStore{identity: identity}
		h, mw := newTestHandler(t, store, fakeVerifier{}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/login", `{"email":"priyantha.w@waypoint.lk","password":"waypoint2026"}`, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var body loginResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Token == "" || body.User.UserID != "seed-dispatcher" || body.User.Role != string(domain.RoleDispatcher) {
			t.Fatalf("body = %+v", body)
		}
		if body.User.DepotID == nil || *body.User.DepotID != "d-peli" {
			t.Fatalf("depot scope = %v", body.User.DepotID)
		}
		if body.User.OutletID != nil {
			t.Fatalf("dispatcher outlet should be null, got %v", *body.User.OutletID)
		}
	})

	t.Run("invalid credentials are a generic 401", func(t *testing.T) {
		store := &fakeStore{authErr: auth.ErrUnauthenticated}
		h, mw := newTestHandler(t, store, fakeVerifier{}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/login", `{"email":"x@y.z","password":"nope"}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if strings.Contains(strings.ToLower(rec.Body.String()), "email exists") {
			t.Fatal("error leaks account existence")
		}
	})

	t.Run("inactive account is also a generic 401", func(t *testing.T) {
		store := &fakeStore{authErr: auth.ErrForbidden}
		h, mw := newTestHandler(t, store, fakeVerifier{}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/login", `{"email":"x@y.z","password":"p"}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("malformed body is 400", func(t *testing.T) {
		h, mw := newTestHandler(t, &fakeStore{}, fakeVerifier{}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/login", `not-json`, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		h, mw := newTestHandler(t, &fakeStore{}, fakeVerifier{}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/login", `{"email":"a@b.c","password":"p","role":"DISPATCHER"}`, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("missing fields are 400 with field errors", func(t *testing.T) {
		h, mw := newTestHandler(t, &fakeStore{}, fakeVerifier{}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/login", `{"email":"","password":""}`, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestMeHandler(t *testing.T) {
	user := auth.Identity{UserID: "seed-store-manager", Name: "Ishara S.", Email: "ishara.s@waypoint.lk", Role: domain.RoleStoreManager, OutletID: "OUT014"}

	t.Run("unauthenticated is 401", func(t *testing.T) {
		h, mw := newTestHandler(t, &fakeStore{}, fakeVerifier{err: auth.ErrUnauthenticated}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodGet, "/api/v1/me", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("authenticated returns the effective identity", func(t *testing.T) {
		h, mw := newTestHandler(t, &fakeStore{}, fakeVerifier{identity: auth.Identity{UserID: user.UserID}}, user)
		rec := doRequest(h, mw, http.MethodGet, "/api/v1/me", "", "any-token")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var body userResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Role != string(domain.RoleStoreManager) || body.OutletID == nil || *body.OutletID != "OUT014" {
			t.Fatalf("body = %+v", body)
		}
		if body.DepotID != nil {
			t.Fatalf("store manager should have null depot, got %v", *body.DepotID)
		}
	})
}

func TestLogoutHandler(t *testing.T) {
	user := auth.Identity{UserID: "seed-driver", Role: domain.RoleDriver, DepotID: "d-peli"}

	t.Run("unauthenticated is 401", func(t *testing.T) {
		h, mw := newTestHandler(t, &fakeStore{}, fakeVerifier{err: auth.ErrUnauthenticated}, auth.Identity{})
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/logout", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("revokes the caller's current session", func(t *testing.T) {
		store := &fakeStore{}
		h, mw := newTestHandler(t, store, fakeVerifier{identity: auth.Identity{UserID: user.UserID}}, user)
		rec := doRequest(h, mw, http.MethodPost, "/api/v1/auth/logout", "", "raw-session-token")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
		}
		if store.deletedHex != auth.HashSessionToken("raw-session-token") {
			t.Fatalf("deleted %q, want the hash of the presented token", store.deletedHex)
		}
	})
}
