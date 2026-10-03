package auth

import (
	"context"
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// dispatcher is a depot-scoped account (plans both depots).
var dispatcher = Identity{
	UserID: "seed-dispatcher", Email: "priyantha.w@waypoint.lk",
	Role: domain.RoleDispatcher, DepotID: "d-peli",
}

// storeManager is outlet-scoped.
var storeManager = Identity{
	UserID: "seed-store-manager", Email: "ishara.s@waypoint.lk",
	Role: domain.RoleStoreManager, OutletID: "OUT014",
}

// loader is depot-scoped.
var loader = Identity{
	UserID: "seed-loader", Role: domain.RoleLoader, DepotID: "d-peli",
}

func TestIdentityAuthenticated(t *testing.T) {
	tests := []struct {
		name string
		id   Identity
		want bool
	}{
		{"complete dispatcher", dispatcher, true},
		{"complete store manager", storeManager, true},
		{"missing user id", Identity{Role: domain.RoleLoader}, false},
		{"missing role", Identity{UserID: "u1"}, false},
		{"unknown role", Identity{UserID: "u1", Role: domain.Role("ADMIN")}, false},
		{"empty", Identity{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Authenticated(); got != tt.want {
				t.Fatalf("Authenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdentityHasRole(t *testing.T) {
	if !loader.HasRole(domain.RoleLoader) {
		t.Fatal("loader should have LOADER")
	}
	if loader.HasRole(domain.RoleDriver) {
		t.Fatal("loader must not have DRIVER")
	}
	if !loader.HasAnyRole(domain.RoleDriver, domain.RoleLoader) {
		t.Fatal("loader should match one-of {DRIVER, LOADER}")
	}
	if loader.HasAnyRole(domain.RoleDriver, domain.RoleStoreManager) {
		t.Fatal("loader must not match {DRIVER, STORE_MANAGER}")
	}
}

func TestScope(t *testing.T) {
	if s := dispatcher.Scope(); !s.AllDepots || s.DepotID != "d-peli" {
		t.Fatalf("dispatcher scope = %+v", s)
	}
	if s := loader.Scope(); s.AllDepots {
		t.Fatalf("loader must not be AllDepots: %+v", s)
	}
	if s := storeManager.Scope(); s.OutletID != "OUT014" || s.AllDepots {
		t.Fatalf("store manager scope = %+v", s)
	}
}

func TestCanAccessDepot(t *testing.T) {
	tests := []struct {
		name    string
		id      Identity
		depotID string
		want    bool
	}{
		{"dispatcher sees own depot", dispatcher, "d-peli", true},
		{"dispatcher sees the other depot", dispatcher, "d-kandy", true},
		{"loader sees own depot", loader, "d-peli", true},
		{"loader cannot see the other depot", loader, "d-kandy", false},
		{"outlet-scoped account matches no depot", storeManager, "d-peli", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.CanAccessDepot(tt.depotID); got != tt.want {
				t.Fatalf("CanAccessDepot(%q) = %v, want %v", tt.depotID, got, tt.want)
			}
		})
	}
}

func TestCanAccessOutlet(t *testing.T) {
	tests := []struct {
		name     string
		id       Identity
		outletID string
		want     bool
	}{
		{"store manager own outlet", storeManager, "OUT014", true},
		{"store manager another outlet", storeManager, "OUT015", false},
		{"dispatcher any outlet", dispatcher, "OUT099", true},
		{"loader is depot-scoped not outlet-pinned", loader, "OUT099", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.CanAccessOutlet(tt.outletID); got != tt.want {
				t.Fatalf("CanAccessOutlet(%q) = %v, want %v", tt.outletID, got, tt.want)
			}
		})
	}
}

// fakeVerifier lets the loader/context/middleware tests run without the real,
// still-TBD Better Auth bridge.
type fakeVerifier struct {
	identity Identity
	err      error
}

func (f fakeVerifier) Verify(context.Context, *RequestHeader) (Identity, error) {
	return f.identity, f.err
}

func TestSessionTokenVerifierIsFailClosed(t *testing.T) {
	// The shipped placeholder must never authenticate anyone, and must be
	// distinguishable from a rejected session.
	_, err := (SessionTokenVerifier{}).Verify(context.Background(), &RequestHeader{Authorization: "Bearer anything"})
	if !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("SessionTokenVerifier err = %v, want ErrVerificationUnavailable", err)
	}
}

func TestIdentityLoader(t *testing.T) {
	store := NewStaticUserStore(dispatcher, storeManager, loader)

	t.Run("verifier establishes user, store supplies scope", func(t *testing.T) {
		// A real verifier returns only the user id; scope must come from the store.
		l := NewIdentityLoader(fakeVerifier{identity: Identity{UserID: storeManager.UserID, Email: storeManager.Email}}, store)
		got, err := l.Load(context.Background(), &RequestHeader{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Role != domain.RoleStoreManager || got.OutletID != "OUT014" {
			t.Fatalf("loaded = %+v", got)
		}
		if got.Email != storeManager.Email {
			t.Fatalf("email not carried through: %q", got.Email)
		}
	})

	t.Run("verifier error propagates", func(t *testing.T) {
		l := NewIdentityLoader(fakeVerifier{err: ErrInvalidSession}, store)
		if _, err := l.Load(context.Background(), &RequestHeader{}); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("unknown user is forbidden", func(t *testing.T) {
		l := NewIdentityLoader(fakeVerifier{identity: Identity{UserID: "nobody"}}, store)
		if _, err := l.Load(context.Background(), &RequestHeader{}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("verifier returning no user id is invalid", func(t *testing.T) {
		l := NewIdentityLoader(fakeVerifier{identity: Identity{}}, store)
		if _, err := l.Load(context.Background(), &RequestHeader{}); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("err = %v, want ErrInvalidSession", err)
		}
	})

	t.Run("nil loader fails closed", func(t *testing.T) {
		var l *IdentityLoader
		if _, err := l.Load(context.Background(), &RequestHeader{}); !errors.Is(err, ErrVerificationUnavailable) {
			t.Fatalf("err = %v, want ErrVerificationUnavailable", err)
		}
	})
}

func TestStaticUserStore(t *testing.T) {
	store := NewStaticUserStore(loader)
	got, err := store.LoadIdentity(context.Background(), loader.UserID)
	if err != nil || got.Role != domain.RoleLoader {
		t.Fatalf("LoadIdentity = %+v, %v", got, err)
	}
	if _, err := store.LoadIdentity(context.Background(), "missing"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("missing user err = %v, want ErrForbidden", err)
	}
	var nilStore *StaticUserStore
	if _, err := nilStore.LoadIdentity(context.Background(), loader.UserID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nil store err = %v, want ErrForbidden", err)
	}
}

func TestContextIdentity(t *testing.T) {
	t.Run("missing identity", func(t *testing.T) {
		if _, ok := IdentityFrom(context.Background()); ok {
			t.Fatal("expected no identity in a bare context")
		}
		if _, err := MustIdentity(context.Background()); !errors.Is(err, ErrNoIdentity) {
			t.Fatalf("MustIdentity err = %v, want ErrNoIdentity", err)
		}
	})

	t.Run("round-trip", func(t *testing.T) {
		ctx := WithIdentity(context.Background(), dispatcher)
		got, ok := IdentityFrom(ctx)
		if !ok || got.UserID != dispatcher.UserID {
			t.Fatalf("IdentityFrom = %+v, ok=%v", got, ok)
		}
		if _, err := MustIdentity(ctx); err != nil {
			t.Fatalf("MustIdentity = %v", err)
		}
	})

	t.Run("context key cannot be spoofed by another package", func(t *testing.T) {
		// A value stored under a different key must not be read back.
		type otherKey struct{}
		ctx := context.WithValue(context.Background(), otherKey{}, dispatcher)
		if _, ok := IdentityFrom(ctx); ok {
			t.Fatal("identity resolved from a foreign context key")
		}
	})
}

func TestLocalAuthorizer(t *testing.T) {
	authz := NewAuthorizer()
	ctxFor := func(id Identity) context.Context { return WithIdentity(context.Background(), id) }
	empty := context.Background()

	t.Run("unauthenticated context", func(t *testing.T) {
		if err := authz.Authenticated(empty); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("authenticated context", func(t *testing.T) {
		if err := authz.Authenticated(ctxFor(loader)); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("required role accepted", func(t *testing.T) {
		if err := authz.RequireRole(ctxFor(loader), domain.RoleLoader); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("incorrect role rejected", func(t *testing.T) {
		if err := authz.RequireRole(ctxFor(loader), domain.RoleDriver); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("one-of-role accepted", func(t *testing.T) {
		if err := authz.RequireAnyRole(ctxFor(loader), domain.RoleDriver, domain.RoleLoader); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("one-of-role rejected", func(t *testing.T) {
		if err := authz.RequireAnyRole(ctxFor(loader), domain.RoleDriver); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("scope allowed", func(t *testing.T) {
		if err := authz.RequireScope(ctxFor(loader), "d-peli"); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("scope denied", func(t *testing.T) {
		if err := authz.RequireScope(ctxFor(loader), "d-kandy"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("empty scope only requires authentication", func(t *testing.T) {
		if err := authz.RequireScope(ctxFor(loader), ""); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if err := authz.RequireScope(empty, ""); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("outlet scope allowed/denied", func(t *testing.T) {
		if err := authz.RequireOutlet(ctxFor(storeManager), "OUT014"); err != nil {
			t.Fatalf("own outlet err = %v, want nil", err)
		}
		if err := authz.RequireOutlet(ctxFor(storeManager), "OUT015"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("other outlet err = %v, want ErrForbidden", err)
		}
	})

	t.Run("dispatcher scope is any depot", func(t *testing.T) {
		if err := authz.RequireScope(ctxFor(dispatcher), "d-kandy"); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})
}
