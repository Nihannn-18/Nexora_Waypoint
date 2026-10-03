// Package authstore is the PostgreSQL-backed authentication store.
//
// It owns the three database operations behind Go authentication:
//
//   - Authenticate: verify an email/password against app_user.password_hash.
//   - CreateSession / ValidateToken / DeleteSession: opaque session lifecycle.
//   - LoadIdentity: role and depot/outlet scope for an authenticated user.
//
// It also resolves media owners to their depot/outlet so media authorization
// can enforce scope without the media package importing SQL.
//
// Passwords are verified in constant time by internal/password (Argon2id).
// Sessions are stored only as SHA-256 token hashes. All queries are
// parameterised; database errors are wrapped and never returned verbatim.
package authstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/password"
)

// Store implements auth.SessionValidator, auth.UserStore and the login/logout
// store contract consumed by internal/authapi.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a store over an existing pool. The pool is owned by the caller.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Authenticate verifies email/password and returns the caller's identity.
//
// A wrong password, an unknown email and a password-less account all return
// auth.ErrUnauthenticated (the HTTP layer maps it to a generic 401). An
// inactive account returns auth.ErrForbidden. An unknown email still pays the
// Argon2id cost so timing does not reveal whether the account exists.
func (s *Store) Authenticate(ctx context.Context, email, plainPassword string) (auth.Identity, error) {
	if s == nil || s.pool == nil {
		return auth.Identity{}, auth.ErrVerificationUnavailable
	}

	var (
		id       string
		mail     string
		hash     string
		role     string
		depotID  string
		outletID string
		name     string
		active   bool
	)
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, password_hash, role,
		       COALESCE(depot_id::text, ''), COALESCE(outlet_id, ''),
		       COALESCE(display_name, ''), is_active
		FROM app_user
		WHERE email = $1`,
		email,
	).Scan(&id, &mail, &hash, &role, &depotID, &outletID, &name, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = password.Verify(plainPassword, password.DummyHash())
		return auth.Identity{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return auth.Identity{}, fmt.Errorf("load credentials: %w", err)
	}

	if hash == "" || password.Verify(plainPassword, hash) != nil {
		return auth.Identity{}, auth.ErrUnauthenticated
	}
	if !active {
		return auth.Identity{}, auth.ErrForbidden
	}

	domainRole := domain.Role(role)
	if !domainRole.Valid() {
		return auth.Identity{}, fmt.Errorf("%w: unknown role", auth.ErrForbidden)
	}

	return auth.Identity{
		UserID:   id,
		Email:    mail,
		Name:     name,
		Role:     domainRole,
		DepotID:  depotID,
		OutletID: outletID,
	}, nil
}

// CreateSession persists a new session for userID. tokenHash is the SHA-256
// hash of the raw token, which is never stored.
func (s *Store) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if s == nil || s.pool == nil {
		return auth.ErrVerificationUnavailable
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO session (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// ValidateToken resolves a raw session token to an active user's id. An unknown
// or expired session, or a deleted/inactive user, is auth.ErrInvalidSession.
func (s *Store) ValidateToken(ctx context.Context, rawToken string) (string, error) {
	if s == nil || s.pool == nil {
		return "", auth.ErrVerificationUnavailable
	}
	if rawToken == "" {
		return "", auth.ErrInvalidSession
	}

	var userID string
	err := s.pool.QueryRow(ctx, `
		SELECT s.user_id
		FROM session s
		JOIN app_user u ON u.user_id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > now()
		  AND u.is_active`,
		auth.HashSessionToken(rawToken),
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrInvalidSession
	}
	if err != nil {
		return "", fmt.Errorf("validate session: %w", err)
	}
	return userID, nil
}

// DeleteSession removes the session for tokenHash. Removing a missing session
// is not an error, so a repeated logout is safe.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if s == nil || s.pool == nil {
		return auth.ErrVerificationUnavailable
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM session WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// LoadIdentity loads role and depot/outlet scope for an app_user id. Unknown or
// inactive users are auth.ErrForbidden, matching StaticUserStore.
func (s *Store) LoadIdentity(ctx context.Context, userID string) (auth.Identity, error) {
	if s == nil || s.pool == nil {
		return auth.Identity{}, auth.ErrVerificationUnavailable
	}
	if userID == "" {
		return auth.Identity{}, fmt.Errorf("%w: empty user id", auth.ErrForbidden)
	}

	var (
		id       string
		email    string
		name     string
		role     string
		depotID  string
		outletID string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, COALESCE(display_name, ''), role,
		       COALESCE(depot_id::text, ''), COALESCE(outlet_id, '')
		FROM app_user
		WHERE user_id = $1 AND is_active`,
		userID,
	).Scan(&id, &email, &name, &role, &depotID, &outletID)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Identity{}, fmt.Errorf("%w: unknown or inactive user", auth.ErrForbidden)
	}
	if err != nil {
		return auth.Identity{}, fmt.Errorf("load app_user identity: %w", err)
	}

	domainRole := domain.Role(role)
	if !domainRole.Valid() {
		return auth.Identity{}, fmt.Errorf("%w: unknown role", auth.ErrForbidden)
	}

	return auth.Identity{
		UserID:   id,
		Email:    email,
		Name:     name,
		Role:     domainRole,
		DepotID:  depotID,
		OutletID: outletID,
	}, nil
}

// OrderItemScope resolves the depot and outlet of a shortfall's order line. It
// backs media scope checks for loader shortfall uploads. found is false when the
// order line does not exist.
func (s *Store) OrderItemScope(ctx context.Context, orderItemID string) (depotID, outletID string, found bool, err error) {
	if s == nil || s.pool == nil {
		return "", "", false, auth.ErrVerificationUnavailable
	}
	err = s.pool.QueryRow(ctx, `
		SELECT o.depot_id::text, o.outlet_id
		FROM order_item oi
		JOIN customer_order co ON co.order_id = oi.order_id
		JOIN outlet o ON o.outlet_id = co.outlet_id
		WHERE oi.order_item_id::text = $1`,
		orderItemID,
	).Scan(&depotID, &outletID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("resolve order-item scope: %w", err)
	}
	return depotID, outletID, true, nil
}

// LegScope resolves the depot and outlet of a route leg. It backs media scope
// checks for driver POD uploads. found is false when the leg does not exist.
func (s *Store) LegScope(ctx context.Context, legID string) (depotID, outletID string, found bool, err error) {
	if s == nil || s.pool == nil {
		return "", "", false, auth.ErrVerificationUnavailable
	}
	err = s.pool.QueryRow(ctx, `
		SELECT o.depot_id::text, o.outlet_id
		FROM route_leg l
		JOIN outlet o ON o.outlet_id = l.to_outlet
		WHERE l.leg_id::text = $1`,
		legID,
	).Scan(&depotID, &outletID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("resolve leg scope: %w", err)
	}
	return depotID, outletID, true, nil
}
