package authapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

type fakeStore struct {
	identity   auth.Identity
	authErr    error
	createErr  error
	deleteErr  error
	createdFor string
	createdHex string
	expiresAt  time.Time
	deletedHex string
}

func (f *fakeStore) Authenticate(context.Context, string, string) (auth.Identity, error) {
	return f.identity, f.authErr
}

func (f *fakeStore) CreateSession(_ context.Context, userID, tokenHash string, expiresAt time.Time) error {
	f.createdFor, f.createdHex, f.expiresAt = userID, tokenHash, expiresAt
	return f.createErr
}

func (f *fakeStore) DeleteSession(_ context.Context, tokenHash string) error {
	f.deletedHex = tokenHash
	return f.deleteErr
}

func TestServiceLogin(t *testing.T) {
	store := &fakeStore{identity: auth.Identity{
		UserID: "seed-driver", Email: "kasun.p@waypoint.lk", Role: domain.RoleDriver, DepotID: "d-peli",
	}}
	svc := NewService(store, time.Hour)
	before := time.Now()

	result, err := svc.Login(context.Background(), "kasun.p@waypoint.lk", "waypoint2026")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if result.Token == "" {
		t.Fatal("no token returned")
	}
	if result.User.UserID != "seed-driver" || result.User.Role != domain.RoleDriver {
		t.Fatalf("user = %+v", result.User)
	}
	if store.createdFor != "seed-driver" {
		t.Fatalf("session created for %q", store.createdFor)
	}
	if store.createdHex != auth.HashSessionToken(result.Token) {
		t.Fatal("stored hash is not HashSessionToken(token); raw token would be persisted")
	}
	if store.createdHex == result.Token {
		t.Fatal("raw token was stored instead of its hash")
	}
	if store.expiresAt.Before(before.Add(time.Hour)) || store.expiresAt.After(time.Now().Add(time.Hour+time.Minute)) {
		t.Fatalf("expiry %v is not ~1h ahead", store.expiresAt)
	}
}

func TestServiceLoginErrors(t *testing.T) {
	t.Run("bad credentials propagate", func(t *testing.T) {
		svc := NewService(&fakeStore{authErr: auth.ErrUnauthenticated}, time.Hour)
		if _, err := svc.Login(context.Background(), "x@y", "bad"); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("create session failure is surfaced", func(t *testing.T) {
		store := &fakeStore{identity: auth.Identity{UserID: "u", Role: domain.RoleLoader}, createErr: errors.New("boom")}
		svc := NewService(store, time.Hour)
		if _, err := svc.Login(context.Background(), "x@y", "ok"); err == nil {
			t.Fatal("expected an error when session creation fails")
		}
	})
}

func TestServiceLogout(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, time.Hour)

	if err := svc.Logout(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if store.deletedHex != auth.HashSessionToken("raw-token") {
		t.Fatalf("deleted %q, want the token hash", store.deletedHex)
	}

	// An empty token is a no-op, not a panic.
	store.deletedHex = ""
	if err := svc.Logout(context.Background(), ""); err != nil {
		t.Fatalf("Logout empty: %v", err)
	}
	if store.deletedHex != "" {
		t.Fatalf("empty token triggered a delete: %q", store.deletedHex)
	}
}

func TestNewServiceDefaultTTL(t *testing.T) {
	store := &fakeStore{identity: auth.Identity{UserID: "u", Role: domain.RoleLoader}}
	svc := NewService(store, 0)
	if _, err := svc.Login(context.Background(), "x@y", "p"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got := time.Until(store.expiresAt); got < 11*time.Hour || got > 13*time.Hour {
		t.Fatalf("default ttl gave expiry in %v, want ~12h", got)
	}
}
