// Package authapi serves the Go-owned authentication endpoints:
//
//	POST /api/v1/auth/login   -> verify credentials, mint an opaque session
//	POST /api/v1/auth/logout  -> revoke the caller's current session
//	GET  /api/v1/me           -> the authenticated caller's identity and scope
//
// The service holds the login/logout rules; the handler only decodes, calls the
// service and maps errors to the shared HTTP error contract. Sessions are
// opaque and server-side: the raw token is returned once at login and only its
// hash is persisted.
package authapi

import (
	"context"
	"fmt"
	"time"

	"waypoint.lk/api/internal/auth"
)

// Store is the persistence the service needs. It is implemented by
// internal/authstore over PostgreSQL.
type Store interface {
	// Authenticate verifies credentials and returns the caller's identity.
	Authenticate(ctx context.Context, email, password string) (auth.Identity, error)
	// CreateSession persists a session for userID keyed by tokenHash.
	CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	// DeleteSession removes the session for tokenHash (idempotent).
	DeleteSession(ctx context.Context, tokenHash string) error
}

// Service implements login and logout.
type Service struct {
	store Store
	ttl   time.Duration
	now   func() time.Time
}

// NewService builds the service. A non-positive ttl falls back to 12 hours so a
// misconfiguration never mints an already-expired session.
func NewService(store Store, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Service{store: store, ttl: ttl, now: time.Now}
}

// LoginResult is a successful login: the raw session token (returned once) and
// the caller's identity.
type LoginResult struct {
	Token string
	User  auth.Identity
}

// Login verifies credentials and creates a session. On failure it returns
// auth.ErrUnauthenticated (bad credentials) or auth.ErrForbidden (inactive),
// which the handler maps to a generic 401 so account state is not revealed.
func (s *Service) Login(ctx context.Context, email, plainPassword string) (LoginResult, error) {
	identity, err := s.store.Authenticate(ctx, email, plainPassword)
	if err != nil {
		return LoginResult{}, err
	}

	raw, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("mint session token: %w", err)
	}
	if err := s.store.CreateSession(ctx, identity.UserID, tokenHash, s.now().Add(s.ttl)); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{Token: raw, User: identity}, nil
}

// Logout revokes the session identified by rawToken. An empty token is a no-op;
// the handler only calls this behind RequireAuthenticated, so a valid token is
// expected.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, auth.HashSessionToken(rawToken))
}
