// Package authstore is the PostgreSQL-backed authentication store.
//
// It is the persistence seam behind internal/auth: it resolves a Better Auth
// session token to an active application user, and loads that user's role and
// depot/outlet scope from app_user.
//
// Better Auth owns the `user`, `session`, `account` and `verification` tables.
// This package only reads the session and user rows; it never writes them. The
// boundary between the two systems is the email address: a Better Auth user maps
// to an app_user with the same email (see docs/data-model.md).
//
// All queries are parameterised. Database errors are wrapped and never returned
// to a client verbatim; the auth middleware maps the sentinel errors to 401/403.
package authstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
)

// Store implements auth.SessionVerifier (session -> identity) and auth.UserStore
// (user id -> role and scope) over PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a store over an existing pool. The pool is owned by the caller.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// VerifySession resolves an active Better Auth session token.
//
// It first finds a live session and its Better Auth user, then maps that user to
// an active app_user by email. An unknown or expired session is
// auth.ErrInvalidSession (401); a valid session with no provisioned, active
// app_user is auth.ErrForbidden (403).
func (s *Store) VerifySession(ctx context.Context, token string) (string, string, string, error) {
	if s == nil || s.pool == nil {
		return "", "", "", auth.ErrVerificationUnavailable
	}
	if token == "" {
		return "", "", "", auth.ErrInvalidSession
	}

	var email, name string
	err := s.pool.QueryRow(ctx, `
		SELECT u."email", u."name"
		FROM "session" s
		JOIN "user" u ON u."id" = s."userId"
		WHERE s."token" = $1 AND s."expiresAt" > now()`,
		token,
	).Scan(&email, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", auth.ErrInvalidSession
	}
	if err != nil {
		return "", "", "", fmt.Errorf("look up Better Auth session: %w", err)
	}

	var userID string
	err = s.pool.QueryRow(ctx, `
		SELECT user_id
		FROM app_user
		WHERE email = $1 AND is_active`,
		email,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Authenticated with a real Better Auth session, but not a provisioned
		// Waypoint user: authenticated but not permitted.
		return "", "", "", auth.ErrForbidden
	}
	if err != nil {
		return "", "", "", fmt.Errorf("map Better Auth user to app_user: %w", err)
	}

	return userID, email, name, nil
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
		role     string
		depotID  string
		outletID string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, email, role,
		       COALESCE(depot_id::text, ''),
		       COALESCE(outlet_id, '')
		FROM app_user
		WHERE user_id = $1 AND is_active`,
		userID,
	).Scan(&id, &email, &role, &depotID, &outletID)
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
