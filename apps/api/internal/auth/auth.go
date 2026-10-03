// Package auth is the authentication and authorization boundary for the Go API.
//
// Better Auth, in the Next.js app, owns authentication, sessions and identity.
// The Go API does not issue or store credentials. It verifies an authenticated
// request, loads the caller's role and scope, and enforces RBAC and
// resource-level scope on every request.
//
// The exact Better Auth -> Go verification mechanism is TBD (see
// docs/api.md, "Authentication"). This package therefore models the boundary
// explicitly rather than guessing:
//
//   - RequestVerifier is the seam the real Better Auth bridge implements. The
//     only shipped implementation, SessionTokenVerifier, is an explicit stub
//     that returns ErrVerificationUnavailable — it never accepts a request and
//     never invents a JWT/bearer fallback.
//   - Identity is what every downstream handler consumes. It is deliberately
//     independent of how the caller was verified, so swapping the verifier
//     changes no handler.
//
// Nothing here logs or serialises a token, session secret or credential.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Identity is an authenticated caller, flattened to what authorization needs.
//
// Role is exactly one of the four canonical domain.Role values; there are no
// additional roles. Scope is derived from the app_user record, never from a
// client-supplied parameter and never trusted from a token claim alone.
type Identity struct {
	// UserID matches app_user.user_id.
	UserID string
	// Email is the account address (app_user.email). Optional in the value;
	// present when the verifier loaded it.
	Email string
	// Role is one of the four domain roles.
	Role domain.Role
	// DepotID is the caller's home depot (app_user.depot_id), empty when the
	// account is not depot-scoped. A dispatcher is depot-scoped but plans both
	// depots, so DepotID is a filter hint, not a hard restriction, for that role.
	DepotID string
	// OutletID is the caller's outlet (app_user.outlet_id), empty unless the
	// account is outlet-scoped (a store manager).
	OutletID string
}

// Authenticated reports whether the identity is usable. An Identity is valid
// only when it names a user and carries a known role.
func (id Identity) Authenticated() bool {
	return id.UserID != "" && id.Role.Valid()
}

// HasRole reports whether the caller's role is exactly role.
func (id Identity) HasRole(role domain.Role) bool {
	return id.Role == role
}

// HasAnyRole reports whether the caller's role is one of roles.
func (id Identity) HasAnyRole(roles ...domain.Role) bool {
	for _, r := range roles {
		if id.Role == r {
			return true
		}
	}
	return false
}

// Scope describes the depot/outlet bounds an identity may touch.
type Scope struct {
	// DepotID is the caller's depot, if any.
	DepotID string
	// OutletID is the caller's outlet, if any.
	OutletID string
	// AllDepots is true for a dispatcher, who plans both depots and is
	// therefore filtered by a requested depotId rather than pinned to one.
	AllDepots bool
}

// Scope returns the identity's scope bounds.
func (id Identity) Scope() Scope {
	return Scope{
		DepotID:   id.DepotID,
		OutletID:  id.OutletID,
		AllDepots: id.Role == domain.RoleDispatcher,
	}
}

// CanAccessDepot reports whether the identity may read a record in depotID.
// A dispatcher sees both depots; any other account is limited to its own.
func (id Identity) CanAccessDepot(depotID string) bool {
	if id.Role == domain.RoleDispatcher {
		return true
	}
	return id.DepotID != "" && id.DepotID == depotID
}

// CanAccessOutlet reports whether the identity may read a record for outletID.
// A store manager is pinned to its own outlet; a dispatcher sees every outlet;
// a loader or driver is bounded by depot, not by a specific outlet, so outlet
// checks for those roles fall back to the depot check the caller supplies.
func (id Identity) CanAccessOutlet(outletID string) bool {
	switch id.Role {
	case domain.RoleDispatcher:
		return true
	case domain.RoleStoreManager:
		return id.OutletID != "" && id.OutletID == outletID
	// LOADER and DRIVER are depot-scoped, not outlet-scoped; outlet-level access
	// is decided by the depot check plus the route they are assigned.
	default:
		return true
	}
}

// Verifier error vocabulary. These are internal signals, mapped to the standard
// HTTP error contract at the auth middleware, never returned to a client as-is.
var (
	// ErrUnauthenticated means no usable credentials were presented.
	ErrUnauthenticated = errors.New("auth: unauthenticated")
	// ErrInvalidSession means credentials were presented but are not valid.
	ErrInvalidSession = errors.New("auth: invalid session")
	// ErrVerificationUnavailable means the verification mechanism is not yet
	// configured (the Better Auth -> Go bridge is TBD). It is distinct from a
	// rejected session so an operator can tell "not wired" from "wrong token".
	ErrVerificationUnavailable = errors.New("auth: verification mechanism TBD")
	// ErrForbidden means the caller is authenticated but not permitted.
	ErrForbidden = errors.New("auth: forbidden")
	// ErrNoIdentity means a downstream helper expected an authenticated request
	// context but none was present (a wiring bug, not a client error).
	ErrNoIdentity = errors.New("auth: no identity in request context")
)

// RequestVerifier turns an HTTP request into an Identity.
//
// Better Auth owns the session; an implementation of this interface is where
// the (TBD) verification happens. Implementations must be safe for concurrent
// use and must not return any credential material in the Identity.
type RequestVerifier interface {
	// Verify returns the caller identity, or one of ErrUnauthenticated,
	// ErrInvalidSession or ErrVerificationUnavailable.
	Verify(ctx context.Context, r *RequestHeader) (Identity, error)
}

// RequestHeader is the minimal, transport-neutral view of a request the
// verifier needs. Passing a small value rather than *http.Request keeps the
// interface testable and free of net/http in its signature.
type RequestHeader struct {
	// Authorization is the raw Authorization header value, if present. It is
	// read only to hand to the (TBD) verification mechanism; it is never logged.
	Authorization string
	// Cookie is the raw Cookie header value, if present — Better Auth sessions
	// are typically cookie-based. Never logged.
	Cookie string
}

// SessionTokenVerifier is the stub wired in until the Better Auth -> Go
// verification mechanism is decided and implemented. It deliberately verifies
// nothing and returns ErrVerificationUnavailable for every request.
//
// This is not a fallback: it never authenticates anyone. It exists so the API
// can be composed with a real verifier later without any handler changing, and
// so a mis-deployment fails closed (401) rather than open.
type SessionTokenVerifier struct{}

// Verify always reports that verification is not yet configured.
func (SessionTokenVerifier) Verify(context.Context, *RequestHeader) (Identity, error) {
	return Identity{}, ErrVerificationUnavailable
}

// UserStore loads the role and scope for an authenticated identity.
//
// Better Auth owns credentials and sessions; app_user holds role and depot/
// outlet scope (see docs/data-model.md). After a verifier establishes who the
// caller is, the store supplies what that caller may do.
type UserStore interface {
	// LoadIdentity returns the role/scope row for userID, or ErrForbidden when
	// the user is unknown or inactive.
	LoadIdentity(ctx context.Context, userID string) (Identity, error)
}

// StaticUserStore is an in-memory UserStore for tests and for composition before
// a database-backed store exists. Production wiring should use the app_user
// table; a StaticUserStore must never be populated from client input.
type StaticUserStore struct {
	Users map[string]Identity
}

// NewStaticUserStore builds a store from a set of identities keyed by UserID.
func NewStaticUserStore(users ...Identity) *StaticUserStore {
	m := make(map[string]Identity, len(users))
	for _, u := range users {
		m[u.UserID] = u
	}
	return &StaticUserStore{Users: m}
}

// LoadIdentity returns the stored identity for userID.
func (s *StaticUserStore) LoadIdentity(_ context.Context, userID string) (Identity, error) {
	if s == nil {
		return Identity{}, ErrForbidden
	}
	id, ok := s.Users[userID]
	if !ok {
		return Identity{}, fmt.Errorf("%w: unknown user", ErrForbidden)
	}
	return id, nil
}

// IdentityLoader combines verification and loading into the Identity that
// downstream middleware store in the request context. It is the seam Agent 3+
// dependencies hang off: replace the verifier, keep the middleware.
type IdentityLoader struct {
	verifier RequestVerifier
	users    UserStore
}

// NewIdentityLoader builds a loader from a verifier and a user store.
func NewIdentityLoader(v RequestVerifier, users UserStore) *IdentityLoader {
	return &IdentityLoader{verifier: v, users: users}
}

// Load verifies the request and resolves the full identity. The verifier
// establishes the user id; the store supplies role and scope. A verifier that
// returns an identity without scope is completed from the store.
func (l *IdentityLoader) Load(ctx context.Context, h *RequestHeader) (Identity, error) {
	if l == nil || l.verifier == nil {
		return Identity{}, ErrVerificationUnavailable
	}
	partial, err := l.verifier.Verify(ctx, h)
	if err != nil {
		return Identity{}, err
	}
	if partial.UserID == "" {
		return Identity{}, ErrInvalidSession
	}
	if l.users == nil {
		// No store: trust only a fully-formed identity (used in tests).
		if !partial.Authenticated() {
			return Identity{}, ErrInvalidSession
		}
		return partial, nil
	}
	full, err := l.users.LoadIdentity(ctx, partial.UserID)
	if err != nil {
		return Identity{}, err
	}
	// Prefer store scope; preserve any email the verifier supplied.
	if full.Email == "" {
		full.Email = partial.Email
	}
	return full, nil
}

// DemoMode reports whether the seeded demo accounts may be used. It is here so
// callers can gate demo-only behaviour (such as the demo clock) on an explicit
// flag rather than on APP_ENV guesses. The demo accounts carry role and scope
// only; credentials remain Better Auth's.
func DemoMode(env string) bool { return env == "development" }

// nowFunc is a seam for tests that need deterministic time. Business logic that
// needs the wall clock goes through the injected Clock in domain packages, not
// through auth; this is only for internal bookkeeping.
var nowFunc = time.Now
