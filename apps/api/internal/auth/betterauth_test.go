package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// fakeSessions records the token it was asked to verify and returns a canned
// result, so the verifier can be tested without a database.
type fakeSessions struct {
	userID string
	email  string
	name   string
	err    error
	seen   string
}

func (f *fakeSessions) VerifySession(_ context.Context, token string) (string, string, string, error) {
	f.seen = token
	return f.userID, f.email, f.name, f.err
}

func TestBetterAuthSessionVerifier(t *testing.T) {
	t.Run("valid bearer token resolves identity", func(t *testing.T) {
		sessions := &fakeSessions{userID: "seed-driver", email: "kasun.p@waypoint.lk", name: "Kasun P."}
		v := NewBetterAuthSessionVerifier(sessions)
		id, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer raw-session-token"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if id.UserID != "seed-driver" || id.Email != "kasun.p@waypoint.lk" || id.Name != "Kasun P." {
			t.Fatalf("identity = %+v", id)
		}
		if sessions.seen != "raw-session-token" {
			t.Fatalf("token passed = %q, want raw-session-token", sessions.seen)
		}
	})

	t.Run("signed bearer token is reduced to the session token", func(t *testing.T) {
		// Better Auth's bearer plugin uses token.split(".")[0] as session.token.
		sessions := &fakeSessions{userID: "u1", email: "e@x"}
		v := NewBetterAuthSessionVerifier(sessions)
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer raw-token.signature-part"}); err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if sessions.seen != "raw-token" {
			t.Fatalf("token passed = %q, want raw-token", sessions.seen)
		}
	})

	t.Run("url-encoded signed token is decoded then reduced", func(t *testing.T) {
		sessions := &fakeSessions{userID: "u1", email: "e@x"}
		v := NewBetterAuthSessionVerifier(sessions)
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer raw%2Dtoken.sig%3D"}); err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if sessions.seen != "raw-token" {
			t.Fatalf("token passed = %q, want raw-token", sessions.seen)
		}
	})

	t.Run("session cookie fallback", func(t *testing.T) {
		sessions := &fakeSessions{userID: "u1", email: "e@x"}
		v := NewBetterAuthSessionVerifier(sessions)
		if _, err := v.Verify(context.Background(), &RequestHeader{Cookie: "other=1; better-auth.session_token=cookie-token.sig; x=2"}); err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if sessions.seen != "cookie-token" {
			t.Fatalf("token passed = %q, want cookie-token", sessions.seen)
		}
	})

	t.Run("missing credentials is unauthenticated", func(t *testing.T) {
		v := NewBetterAuthSessionVerifier(&fakeSessions{})
		if _, err := v.Verify(context.Background(), &RequestHeader{}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("non-bearer scheme is unauthenticated", func(t *testing.T) {
		v := NewBetterAuthSessionVerifier(&fakeSessions{})
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Basic dXNlcjpwYXNz"}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("empty token is unauthenticated", func(t *testing.T) {
		v := NewBetterAuthSessionVerifier(&fakeSessions{})
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer    "}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("invalid session propagates", func(t *testing.T) {
		v := NewBetterAuthSessionVerifier(&fakeSessions{err: ErrInvalidSession})
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("authenticated but unmapped user is forbidden", func(t *testing.T) {
		v := NewBetterAuthSessionVerifier(&fakeSessions{err: ErrForbidden})
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("nil verifier and nil store fail closed", func(t *testing.T) {
		var v *BetterAuthSessionVerifier
		if _, err := v.Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrVerificationUnavailable) {
			t.Fatalf("nil verifier err = %v, want ErrVerificationUnavailable", err)
		}
		empty := NewBetterAuthSessionVerifier(nil)
		if _, err := empty.Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrVerificationUnavailable) {
			t.Fatalf("nil store err = %v, want ErrVerificationUnavailable", err)
		}
	})
}

// TestVerifierIgnoresIdentityHeaders proves a client cannot spoof identity with
// trusted-looking headers: only the verified session is consulted.
func TestVerifierIgnoresIdentityHeaders(t *testing.T) {
	sessions := &fakeSessions{userID: "real-user", email: "real@waypoint.lk"}
	v := NewBetterAuthSessionVerifier(sessions)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer real-token")
	req.Header.Set("X-User-ID", "admin")
	req.Header.Set("X-Role", "DISPATCHER")
	req.Header.Set("X-Depot-ID", "d-kandy")
	req.Header.Set("X-Outlet-ID", "OUT999")

	id, err := v.Verify(context.Background(), HTTPRequestHeader(req))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.UserID != "real-user" {
		t.Fatalf("user id = %q, want real-user (headers must be ignored)", id.UserID)
	}
}

// TestAllRolesResolveThroughLoader runs each role through the full verifier +
// store seam, proving the four-role boundary.
func TestAllRolesResolveThroughLoader(t *testing.T) {
	tests := []struct {
		role   domain.Role
		depot  string
		outlet string
		emails string
		userID string
	}{
		{domain.RoleDispatcher, "d-peli", "", "priyantha.w@waypoint.lk", "seed-dispatcher"},
		{domain.RoleLoader, "d-peli", "", "nadeesha.p@waypoint.lk", "seed-loader"},
		{domain.RoleDriver, "d-peli", "", "kasun.p@waypoint.lk", "seed-driver"},
		{domain.RoleStoreManager, "", "OUT014", "ishara.s@waypoint.lk", "seed-store-manager"},
	}
	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			store := NewStaticUserStore(Identity{
				UserID: tc.userID, Email: tc.emails, Role: tc.role, DepotID: tc.depot, OutletID: tc.outlet,
			})
			verifier := NewBetterAuthSessionVerifier(&fakeSessions{userID: tc.userID, email: tc.emails})
			loader := NewIdentityLoader(verifier, store)
			id, err := loader.Load(context.Background(), &RequestHeader{Authorization: "Bearer token"})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if id.Role != tc.role || id.DepotID != tc.depot || id.OutletID != tc.outlet {
				t.Fatalf("identity = %+v", id)
			}
		})
	}
}

func TestNormalizeSessionToken(t *testing.T) {
	cases := map[string]string{
		"raw":            "raw",
		"raw.sig":        "raw",
		"raw.a.b":        "raw",
		".sig":           "",
		"":               "",
		"raw%2Dtoken.si": "raw-token",
	}
	for in, want := range cases {
		if got := normalizeSessionToken(in); got != want {
			t.Errorf("normalizeSessionToken(%q) = %q, want %q", in, got, want)
		}
	}
}
