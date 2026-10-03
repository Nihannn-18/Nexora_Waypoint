package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSessionToken(t *testing.T) {
	raw, hash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if raw == "" || hash == "" {
		t.Fatal("empty token or hash")
	}
	if hash != HashSessionToken(raw) {
		t.Fatal("hash does not match HashSessionToken(raw)")
	}
	if len(raw) < 40 {
		t.Fatalf("raw token %q is shorter than expected", raw)
	}

	raw2, hash2, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if raw == raw2 || hash == hash2 {
		t.Fatal("two session tokens are identical; randomness is broken")
	}
}

func TestHashSessionTokenIsDeterministic(t *testing.T) {
	first := HashSessionToken("abc")
	if first != HashSessionToken("abc") {
		t.Fatal("HashSessionToken is not deterministic")
	}
	if first == HashSessionToken("abd") {
		t.Fatal("different tokens hash to the same value")
	}
}

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc.def", "abc.def", true},
		{"bearer abc", "abc", true},
		{"  Bearer   abc  ", "abc", true},
		{"Basic abc", "", false},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer    ", "", false},
		{"abc", "", false},
	}
	for _, tc := range cases {
		got, ok := BearerToken(tc.header)
		if got != tc.want || ok != tc.ok {
			t.Errorf("BearerToken(%q) = %q, %v; want %q, %v", tc.header, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeValidator struct {
	userID string
	err    error
	seen   string
}

func (f *fakeValidator) ValidateToken(_ context.Context, raw string) (string, error) {
	f.seen = raw
	return f.userID, f.err
}

func TestOpaqueSessionVerifier(t *testing.T) {
	t.Run("valid bearer token resolves a partial identity", func(t *testing.T) {
		v := &fakeValidator{userID: "seed-driver"}
		got, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), &RequestHeader{Authorization: "Bearer raw-token"})
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if got.UserID != "seed-driver" {
			t.Fatalf("UserID = %q", got.UserID)
		}
		if v.seen != "raw-token" {
			t.Fatalf("token passed = %q", v.seen)
		}
	})

	t.Run("missing credentials are unauthenticated", func(t *testing.T) {
		v := &fakeValidator{userID: "x"}
		if _, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), &RequestHeader{}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("malformed authorization is unauthenticated", func(t *testing.T) {
		v := &fakeValidator{userID: "x"}
		if _, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), &RequestHeader{Authorization: "Token abc"}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("invalid session propagates", func(t *testing.T) {
		v := &fakeValidator{err: ErrInvalidSession}
		if _, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("forbidden user propagates", func(t *testing.T) {
		v := &fakeValidator{err: ErrForbidden}
		if _, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("nil verifier or store fails closed", func(t *testing.T) {
		var nilVerifier *OpaqueSessionVerifier
		if _, err := nilVerifier.Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrVerificationUnavailable) {
			t.Fatalf("nil verifier err = %v", err)
		}
		if _, err := NewOpaqueSessionVerifier(nil).Verify(context.Background(), &RequestHeader{Authorization: "Bearer t"}); !errors.Is(err, ErrVerificationUnavailable) {
			t.Fatalf("nil store err = %v", err)
		}
	})

	t.Run("cookie alone does not authenticate", func(t *testing.T) {
		v := &fakeValidator{userID: "x"}
		if _, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), &RequestHeader{Cookie: "session=abc"}); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated (cookie transport is not enabled)", err)
		}
	})
}

// TestVerifierIgnoresIdentityHeaders proves a client cannot spoof identity with
// trusted-looking headers: only the verified bearer token is consulted.
func TestVerifierIgnoresIdentityHeaders(t *testing.T) {
	v := &fakeValidator{userID: "real-user"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer real-token")
	req.Header.Set("X-User-ID", "admin")
	req.Header.Set("X-Role", "DISPATCHER")
	req.Header.Set("X-Depot-ID", "d-kandy")
	req.Header.Set("X-Outlet-ID", "OUT999")

	id, err := NewOpaqueSessionVerifier(v).Verify(context.Background(), HTTPRequestHeader(req))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.UserID != "real-user" {
		t.Fatalf("UserID = %q, want real-user (headers must be ignored)", id.UserID)
	}
}
