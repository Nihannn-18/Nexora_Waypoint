package useradmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

// staticVerifier authenticates every request as a fixed identity, so the tests
// exercise RBAC without a session store.
type staticVerifier struct{ identity auth.Identity }

func (v staticVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.identity, nil
}

func newHandler(t *testing.T, store Store, identity auth.Identity, devLog bool) (*Handler, *auth.Middleware) {
	t.Helper()
	loader := auth.NewIdentityLoader(staticVerifier{identity: identity}, auth.NewStaticUserStore(identity))
	mw := auth.NewMiddleware(loader, auth.NewAuthorizer())
	return NewHandler(NewService(store, nil), mw, devLog), mw
}

func do(h *Handler, mw *auth.Middleware, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

var dispatcher = auth.Identity{UserID: "seed-dispatcher", Role: domain.RoleDispatcher, DepotID: "d-peli"}

func TestCreateUserHandler(t *testing.T) {
	t.Run("dispatcher creates a driver", func(t *testing.T) {
		h, mw := newHandler(t, newFakeStore(), dispatcher, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/users",
			`{"email":"d@waypoint.lk","displayName":"Dee","role":"DRIVER","depotId":"d-peli","initialPassword":"secret-pass"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
		}
		var body userResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Role != "DRIVER" || body.UserID == "" {
			t.Fatalf("body = %+v", body)
		}
		// The response must never contain credential material.
		if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "Hash") {
			t.Fatalf("response leaked credential fields: %s", rec.Body.String())
		}
	})

	t.Run("dispatcher cannot create a dispatcher", func(t *testing.T) {
		h, mw := newHandler(t, newFakeStore(), dispatcher, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/users",
			`{"email":"x@waypoint.lk","displayName":"X","role":"DISPATCHER","depotId":"d-peli","initialPassword":"secret-pass"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown role is rejected", func(t *testing.T) {
		h, mw := newHandler(t, newFakeStore(), dispatcher, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/users",
			`{"email":"x@waypoint.lk","displayName":"X","role":"ADMIN","depotId":"d-peli","initialPassword":"secret-pass"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("duplicate email is a 409", func(t *testing.T) {
		store := newFakeStore()
		store.emailExists = true
		h, mw := newHandler(t, store, dispatcher, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/users",
			`{"email":"dup@waypoint.lk","displayName":"D","role":"LOADER","depotId":"d-peli","initialPassword":"secret-pass"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", rec.Code)
		}
	})

	t.Run("unknown field is rejected by strict decode", func(t *testing.T) {
		h, mw := newHandler(t, newFakeStore(), dispatcher, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/users",
			`{"email":"d@waypoint.lk","displayName":"D","role":"DRIVER","depotId":"d-peli","initialPassword":"secret-pass","isAdmin":true}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestUserRoutesRBAC(t *testing.T) {
	roles := []domain.Role{domain.RoleDriver, domain.RoleLoader, domain.RoleStoreManager}
	for _, role := range roles {
		t.Run(string(role)+" is forbidden from account management", func(t *testing.T) {
			id := auth.Identity{UserID: "u", Role: role, DepotID: "d-peli", OutletID: "OUT014"}
			h, mw := newHandler(t, newFakeStore(), id, false)

			if rec := do(h, mw, http.MethodGet, "/api/v1/users", ""); rec.Code != http.StatusForbidden {
				t.Fatalf("GET /users status = %d, want 403", rec.Code)
			}
			if rec := do(h, mw, http.MethodPost, "/api/v1/users",
				`{"email":"d@waypoint.lk","displayName":"D","role":"DRIVER","depotId":"d-peli","initialPassword":"secret-pass"}`); rec.Code != http.StatusForbidden {
				t.Fatalf("POST /users status = %d, want 403", rec.Code)
			}
			if rec := do(h, mw, http.MethodPost, "/api/v1/users/u1/deactivate", ""); rec.Code != http.StatusForbidden {
				t.Fatalf("deactivate status = %d, want 403", rec.Code)
			}
		})
	}
}

func TestUserRoutesUnauthenticated(t *testing.T) {
	// A verifier that returns no identity: RequireRole must answer 401.
	loader := auth.NewIdentityLoader(noIdentityVerifier{}, nil)
	mw := auth.NewMiddleware(loader, auth.NewAuthorizer())
	h := NewHandler(NewService(newFakeStore(), nil), mw, false)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

type noIdentityVerifier struct{}

func (noIdentityVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return auth.Identity{}, auth.ErrUnauthenticated
}

func TestForgotPasswordHandlerGeneric(t *testing.T) {
	t.Run("unknown email returns the same generic message", func(t *testing.T) {
		store := newFakeStore() // userIDByEmail empty
		h, mw := newHandler(t, store, auth.Identity{}, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/auth/forgot-password", `{"email":"nobody@waypoint.lk"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["message"] != genericForgotMessage {
			t.Fatalf("message = %q, want the generic reply", body["message"])
		}
		if strings.Contains(rec.Body.String(), "token") {
			t.Fatalf("response leaked a token: %s", rec.Body.String())
		}
	})

	t.Run("known email returns the same generic message", func(t *testing.T) {
		store := newFakeStore()
		store.userIDByEmail = "u1"
		h, mw := newHandler(t, store, auth.Identity{}, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/auth/forgot-password", `{"email":"kasun.p@waypoint.lk"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["message"] != genericForgotMessage {
			t.Fatalf("message = %q", body["message"])
		}
		if strings.Contains(rec.Body.String(), "token") {
			t.Fatalf("response leaked a token: %s", rec.Body.String())
		}
	})
}

func TestResetPasswordHandler(t *testing.T) {
	t.Run("invalid token is a single field error", func(t *testing.T) {
		store := newFakeStore()
		store.consumeErr = ErrInvalidResetToken
		h, mw := newHandler(t, store, auth.Identity{}, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/auth/reset-password",
			`{"token":"bad","newPassword":"new-secret","confirmPassword":"new-secret"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "token") {
			t.Fatalf("response should name the token field: %s", rec.Body.String())
		}
	})

	t.Run("valid token succeeds", func(t *testing.T) {
		store := newFakeStore()
		store.consumeUserID = "u1"
		h, mw := newHandler(t, store, auth.Identity{}, false)
		rec := do(h, mw, http.MethodPost, "/api/v1/auth/reset-password",
			`{"token":"good","newPassword":"new-secret","confirmPassword":"new-secret"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeactivateHandlerRevokes(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleDriver, Active: true}
	h, mw := newHandler(t, store, dispatcher, false)
	rec := do(h, mw, http.MethodPost, "/api/v1/users/u1/deactivate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body userResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Active {
		t.Fatal("response should show the account deactivated")
	}
}
