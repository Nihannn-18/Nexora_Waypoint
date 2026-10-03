package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// okHandler records that it ran and echoes the caller's user id so a test can
// prove the identity reached the handler. It never touches credentials.
func okHandler() (http.Handler, *string) {
	var seen string
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := IdentityFrom(r.Context()); ok {
			seen = id.UserID
		}
		w.WriteHeader(http.StatusOK)
	}), &seen
}

func middlewareFor(v RequestVerifier, users UserStore) *Middleware {
	return NewMiddleware(NewIdentityLoader(v, users), LocalAuthorizer{})
}

func do(t *testing.T, m *Middleware, wrap func(http.Handler) http.Handler, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	next, _ := okHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	wrap(next).ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) httpx.ErrorBody {
	t.Helper()
	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (body=%q)", err, rec.Body.String())
	}
	return body
}

func TestRequireAuthenticated(t *testing.T) {
	store := NewStaticUserStore(loader)

	t.Run("unauthenticated request is rejected", func(t *testing.T) {
		m := middlewareFor(SessionTokenVerifier{}, store)
		rec := do(t, m, m.RequireAuthenticated, nil)
		// The stub returns ErrVerificationUnavailable, which is a 500 so a
		// misconfigured bridge is loud rather than a silent 401.
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
		if got := decodeBody(t, rec).Code; got != httpx.CodeInternal {
			t.Fatalf("code = %q, want %q", got, httpx.CodeInternal)
		}
	})

	t.Run("invalid session is 401", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{err: ErrInvalidSession}, store)
		rec := do(t, m, m.RequireAuthenticated, map[string]string{"Authorization": "Bearer nope"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := decodeBody(t, rec).Code; got != httpx.CodeUnauthenticated {
			t.Fatalf("code = %q, want %q", got, httpx.CodeUnauthenticated)
		}
	})

	t.Run("authenticated request is accepted and carries identity", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		next, seen := okHandler()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
		m.RequireAuthenticated(next).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if *seen != loader.UserID {
			t.Fatalf("handler saw user %q, want %q", *seen, loader.UserID)
		}
	})
}

func TestRequireRoleMiddleware(t *testing.T) {
	store := NewStaticUserStore(loader, dispatcher)

	t.Run("required role accepted", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		rec := do(t, m, func(next http.Handler) http.Handler { return m.RequireRole(domain.RoleLoader, next) }, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("incorrect role rejected with 403", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		rec := do(t, m, func(next http.Handler) http.Handler { return m.RequireRole(domain.RoleDriver, next) }, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if got := decodeBody(t, rec).Code; got != httpx.CodeForbidden {
			t.Fatalf("code = %q, want %q", got, httpx.CodeForbidden)
		}
	})

	t.Run("one-of roles accepted", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		rec := do(t, m, func(next http.Handler) http.Handler {
			return m.RequireAnyRole([]domain.Role{domain.RoleDriver, domain.RoleLoader}, next)
		}, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("one-of roles rejected", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		rec := do(t, m, func(next http.Handler) http.Handler {
			return m.RequireAnyRole([]domain.Role{domain.RoleDriver, domain.RoleStoreManager}, next)
		}, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
}

func TestRequireScopeMiddleware(t *testing.T) {
	store := NewStaticUserStore(loader, dispatcher)

	t.Run("scope allowed", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		rec := do(t, m, func(next http.Handler) http.Handler { return m.RequireScope("d-peli", next) }, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("scope denied", func(t *testing.T) {
		m := middlewareFor(fakeVerifier{identity: Identity{UserID: loader.UserID}}, store)
		rec := do(t, m, func(next http.Handler) http.Handler { return m.RequireScope("d-kandy", next) }, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
}

// TestNoCredentialLeak asserts that neither the response body nor the error
// message echoes the Authorization header or any token-like value, for every
// rejection path.
func TestNoCredentialLeak(t *testing.T) {
	secret := "eyJhbGciOiJIUzI1NiJ9.SUPER-SECRET-SESSION-TOKEN.signature"
	store := NewStaticUserStore(loader)

	cases := []struct {
		name  string
		v     RequestVerifier
		store UserStore
		wrap  func(m *Middleware, next http.Handler) http.Handler
	}{
		{"unauthenticated", SessionTokenVerifier{}, store, func(m *Middleware, n http.Handler) http.Handler { return m.RequireAuthenticated(n) }},
		{"invalid", fakeVerifier{err: ErrInvalidSession}, store, func(m *Middleware, n http.Handler) http.Handler { return m.RequireAuthenticated(n) }},
		{"forbidden", fakeVerifier{identity: Identity{UserID: loader.UserID}}, store, func(m *Middleware, n http.Handler) http.Handler {
			return m.RequireRole(domain.RoleDriver, n)
		}},
		{"unknown user", fakeVerifier{identity: Identity{UserID: "ghost"}}, store, func(m *Middleware, n http.Handler) http.Handler { return m.RequireAuthenticated(n) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := middlewareFor(tc.v, tc.store)
			next, _ := okHandler()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
			req.Header.Set("Authorization", "Bearer "+secret)
			req.Header.Set("Cookie", "session="+secret)
			tc.wrap(m, next).ServeHTTP(rec, req)

			blob := rec.Body.String() + " " + rec.Header().Get("WWW-Authenticate")
			if strings.Contains(blob, secret) || strings.Contains(blob, "SUPER-SECRET") {
				t.Fatalf("credential leaked in response: %q", blob)
			}
			// The message must also be generic, not a stack of internals.
			if body := decodeBody(t, rec); body.Message == "" {
				t.Fatal("error body missing a message")
			}
		})
	}
}

// TestWriteAuthErrorMapping locks the error -> HTTP contract.
func TestWriteAuthErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"unauthenticated", ErrUnauthenticated, http.StatusUnauthorized, httpx.CodeUnauthenticated},
		{"invalid session", ErrInvalidSession, http.StatusUnauthorized, httpx.CodeUnauthenticated},
		{"forbidden", ErrForbidden, http.StatusForbidden, httpx.CodeForbidden},
		{"verification unavailable", ErrVerificationUnavailable, http.StatusInternalServerError, httpx.CodeInternal},
		{"wrapped forbidden", errors.Join(ErrForbidden, errors.New("depot out of scope")), http.StatusForbidden, httpx.CodeForbidden},
		{"unknown", context.Canceled, http.StatusInternalServerError, httpx.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeAuthError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if body := decodeBody(t, rec); body.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", body.Code, tc.wantCode)
			}
		})
	}
}
