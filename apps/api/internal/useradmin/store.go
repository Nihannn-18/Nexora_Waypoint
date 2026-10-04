package useradmin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"waypoint.lk/api/internal/domain"
)

// AuditSink records one audit event in the caller's transaction. It is a narrow
// interface so this package does not import internal/audit.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error
}

// pgPool is the slice of pgxpool.Pool the store uses. Narrowing it keeps the
// store testable and free of the full pool type.
type pgPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PGStore is the PostgreSQL-backed account-management store. Every write runs
// in a transaction that also writes its audit row, so a rolled-back mutation
// leaves no audit trail.
type PGStore struct {
	pool  pgPool
	audit AuditSink
}

// NewPGStore builds a store over a pool. audit may be nil in narrow tests, in
// which case no audit row is written.
func NewPGStore(p pgPool, audit AuditSink) *PGStore {
	return &PGStore{pool: p, audit: audit}
}

const accountSelect = `
	SELECT user_id, email, COALESCE(display_name, ''), role,
	       COALESCE(depot_id::text, ''), COALESCE(outlet_id, ''), is_active,
	       to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
	FROM app_user`

// accountColumns are the scan destinations for accountSelect, in a helper so
// each call site stays readable.
func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.UserID, &a.Email, &a.DisplayName, &a.Role,
		&a.DepotID, &a.OutletID, &a.Active, &a.CreatedAt)
	return a, err
}

// ListAccounts returns accounts, optionally filtered, newest first.
func (s *PGStore) ListAccounts(ctx context.Context, role domain.Role, active *bool) ([]Account, error) {
	var (
		where []string
		args  []any
	)
	if role != "" {
		args = append(args, string(role))
		where = append(where, fmt.Sprintf("role = $%d", len(args)))
	}
	if active != nil {
		args = append(args, *active)
		where = append(where, fmt.Sprintf("is_active = $%d", len(args)))
	}
	query := accountSelect
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC, user_id"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	out := make([]Account, 0)
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return out, nil
}

// AccountByID loads one account without its credential.
func (s *PGStore) AccountByID(ctx context.Context, userID string) (Account, error) {
	a, err := scanAccount(s.pool.QueryRow(ctx, accountSelect+" WHERE user_id = $1", userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, fmt.Errorf("%w: %s", ErrNotFound, userID)
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

// EmailExists reports whether an email is already in use.
func (s *PGStore) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user WHERE email = $1)`, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("check email: %w", err)
	}
	return exists, nil
}

// DepotExists reports whether a depot exists and is active.
func (s *PGStore) DepotExists(ctx context.Context, depotID string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM depot WHERE depot_id::text = $1 AND is_active)`, depotID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check depot: %w", err)
	}
	return exists, nil
}

// OutletDepot returns the depot id of an existing outlet, or ErrNotFound.
func (s *PGStore) OutletDepot(ctx context.Context, outletID string) (string, error) {
	var depotID string
	err := s.pool.QueryRow(ctx, `SELECT depot_id::text FROM outlet WHERE outlet_id = $1`, outletID).Scan(&depotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ValidationError{Field: "outletId", Message: "is not a known outlet"}
	}
	if err != nil {
		return "", fmt.Errorf("get outlet depot: %w", err)
	}
	return depotID, nil
}

// releaseOutletManagers clears every store manager currently on an outlet
// except keepUserID (which may be empty), auditing each release, so an outlet
// never holds more than one manager. It runs inside the caller's transaction,
// so a failed mutation releases nobody.
func (s *PGStore) releaseOutletManagers(ctx context.Context, tx pgx.Tx, outletID, keepUserID, actor string) error {
	if outletID == "" {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT user_id
		FROM app_user
		WHERE role = 'STORE_MANAGER' AND outlet_id = $1 AND user_id <> $2
		FOR UPDATE`, outletID, keepUserID)
	if err != nil {
		return fmt.Errorf("load outlet managers: %w", err)
	}
	var replaced []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan outlet manager: %w", err)
		}
		replaced = append(replaced, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate outlet managers: %w", err)
	}
	for _, id := range replaced {
		if _, err := tx.Exec(ctx, `
			UPDATE app_user SET outlet_id = NULL, depot_id = NULL
			WHERE user_id = $1 AND role = 'STORE_MANAGER'`, id); err != nil {
			return fmt.Errorf("release previous manager: %w", err)
		}
		if err := s.recordAudit(ctx, tx, "MANAGER_UNASSIGNED", "OUTLET_MANAGER", outletID, actor, "", outletID, map[string]any{
			"userId": id, "releasedFor": keepUserID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// CreateAccount inserts an account and audits the creation in one transaction.
// The user id is generated by the server as usr_<uuid>; a client never supplies
// it.
func (s *PGStore) CreateAccount(ctx context.Context, in CreateInput, passwordHash, actor string) (Account, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("begin create account: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var depotID *string
	if in.DepotID != "" {
		depotID = &in.DepotID
	}
	var outletID *string
	if in.OutletID != "" {
		outletID = &in.OutletID
	}

	// An outlet holds exactly one manager: creating a store manager for an
	// outlet releases whoever held it before, mirroring the assignment
	// endpoint, so the invariant cannot be bypassed through account creation.
	if in.Role == domain.RoleStoreManager {
		if err := s.releaseOutletManagers(ctx, tx, in.OutletID, "", actor); err != nil {
			return Account{}, err
		}
	}

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO app_user (user_id, email, display_name, role, depot_id, outlet_id, password_hash, is_active)
		VALUES ('usr_' || gen_random_uuid()::text, $1, $2, $3, $4, $5, $6, TRUE)
		RETURNING user_id`,
		in.Email, in.DisplayName, string(in.Role), depotID, outletID, passwordHash).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			return Account{}, ErrDuplicateEmail
		}
		return Account{}, fmt.Errorf("insert account: %w", err)
	}

	// The audit detail records the role and scope only. It never carries the
	// initial password or its hash.
	if err := s.recordAudit(ctx, tx, "USER_CREATED", "USER", userID, actor, in.DepotID, in.OutletID, map[string]any{
		"email": in.Email, "role": string(in.Role),
	}); err != nil {
		return Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit create account: %w", err)
	}
	return s.AccountByID(ctx, userID)
}

// UpdateAccount updates the permitted fields and audits the change. The role,
// email and password are never touched here.
func (s *PGStore) UpdateAccount(ctx context.Context, userID string, in UpdateInput, actor string) (Account, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("begin update account: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := scanAccount(tx.QueryRow(ctx, accountSelect+" WHERE user_id = $1 FOR UPDATE", userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, fmt.Errorf("%w: %s", ErrNotFound, userID)
	}
	if err != nil {
		return Account{}, fmt.Errorf("load account for update: %w", err)
	}

	// Resolve the effective value for each permitted field: the patch when
	// present, otherwise the current value.
	name := before.DisplayName
	if in.DisplayName != nil {
		name = *in.DisplayName
	}
	depotID := before.DepotID
	if in.DepotID != nil {
		depotID = *in.DepotID
	}
	outletID := before.OutletID
	if in.OutletID != nil {
		outletID = *in.OutletID
	}

	var depotArg, outletArg *string
	if depotID != "" {
		depotArg = &depotID
	}
	if outletID != "" {
		outletArg = &outletID
	}

	// A store manager's outlet is exclusive: moving one onto an occupied
	// outlet releases the previous manager, exactly as creation does.
	if before.Role == domain.RoleStoreManager && outletArg != nil {
		if err := s.releaseOutletManagers(ctx, tx, outletID, userID, actor); err != nil {
			return Account{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE app_user
		SET display_name = $2, depot_id = $3, outlet_id = $4
		WHERE user_id = $1`,
		userID, name, depotArg, outletArg); err != nil {
		return Account{}, fmt.Errorf("update account: %w", err)
	}

	if err := s.recordAudit(ctx, tx, "USER_UPDATED", "USER", userID, actor, depotID, outletID, map[string]any{
		"before": accountAuditDetail(before),
		"after":  accountAuditDetail(Account{DisplayName: name, DepotID: depotID, OutletID: outletID}),
	}); err != nil {
		return Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit update account: %w", err)
	}
	return s.AccountByID(ctx, userID)
}

// SetActive activates or deactivates an account. Deactivating revokes every
// session for the account in the same transaction, so a live token cannot
// outlive the deactivation. The audit row records the state change only.
func (s *PGStore) SetActive(ctx context.Context, userID string, active bool, actor string) (Account, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("begin set active: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := scanAccount(tx.QueryRow(ctx, accountSelect+" WHERE user_id = $1 FOR UPDATE", userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, fmt.Errorf("%w: %s", ErrNotFound, userID)
	}
	if err != nil {
		return Account{}, fmt.Errorf("load account: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE app_user SET is_active = $2 WHERE user_id = $1`, userID, active); err != nil {
		return Account{}, fmt.Errorf("update active: %w", err)
	}
	if !active {
		if _, err := tx.Exec(ctx, `DELETE FROM session WHERE user_id = $1`, userID); err != nil {
			return Account{}, fmt.Errorf("revoke sessions: %w", err)
		}
	}

	action := "USER_DEACTIVATED"
	if active {
		action = "USER_ACTIVATED"
	}
	if err := s.recordAudit(ctx, tx, action, "USER", userID, actor, before.DepotID, before.OutletID, map[string]any{
		"role": string(before.Role),
	}); err != nil {
		return Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit set active: %w", err)
	}
	return s.AccountByID(ctx, userID)
}

// UserIDByEmail returns the active user id for an email, or "" when there is no
// active account. An inactive account is treated as absent so a password reset
// cannot revive a disabled login.
func (s *PGStore) UserIDByEmail(ctx context.Context, email string) (string, error) {
	var userID string
	err := s.pool.QueryRow(ctx, `SELECT user_id FROM app_user WHERE email = $1 AND is_active`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find user by email: %w", err)
	}
	return userID, nil
}

// CreateResetToken stores a reset-token hash and invalidates any outstanding
// unused token for the same user, so only the most recent link works. It runs
// in one transaction so a race cannot leave two live tokens.
func (s *PGStore) CreateResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create reset token: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Supersede any earlier unused token: the newest request is the only live
	// link, so an old email that is still open cannot reset the password.
	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_token SET used_at = now()
		WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return fmt.Errorf("supersede old reset tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO password_reset_token (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("insert reset token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create reset token: %w", err)
	}
	return nil
}

// ConsumeResetToken validates and consumes a reset token. It requires an unused,
// unexpired token for an active user, sets the new password hash, marks the
// token used, and revokes the user's sessions, all in one transaction.
//
// Expiry is compared against the caller's `now` (the injected business clock),
// never the database's real-time now(): under DEMO_MODE the business day is the
// seeded past day, and `expires_at` was written from that same clock, so a real
// now() would reject every live token (CLAUDE.md §2 rule 8).
func (s *PGStore) ConsumeResetToken(ctx context.Context, tokenHash, newPasswordHash string, now time.Time) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin reset password: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		tokenID string
		userID  string
	)
	err = tx.QueryRow(ctx, `
		SELECT t.token_id, t.user_id
		FROM password_reset_token t
		JOIN app_user u ON u.user_id = t.user_id
		WHERE t.token_hash = $1
		  AND t.used_at IS NULL
		  AND t.expires_at > $2
		  AND u.is_active
		FOR UPDATE OF t`, tokenHash, now).Scan(&tokenID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidResetToken
	}
	if err != nil {
		return "", fmt.Errorf("load reset token: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE app_user SET password_hash = $2 WHERE user_id = $1`, userID, newPasswordHash); err != nil {
		return "", fmt.Errorf("update password: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE password_reset_token SET used_at = now() WHERE token_id = $1`, tokenID); err != nil {
		return "", fmt.Errorf("mark token used: %w", err)
	}
	// A password reset ends every existing session: the old credential must not
	// remain usable anywhere it was already presented.
	if _, err := tx.Exec(ctx, `DELETE FROM session WHERE user_id = $1`, userID); err != nil {
		return "", fmt.Errorf("revoke sessions after reset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit reset password: %w", err)
	}
	return userID, nil
}

// recordAudit writes an audit row in the caller's transaction when a sink is
// configured. The detail map it is given never contains credentials or tokens.
func (s *PGStore) recordAudit(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID string, detail map[string]any) error {
	if s.audit == nil {
		return nil
	}
	if err := s.audit.RecordTx(ctx, tx, action, entityType, entityID, actor, depotID, outletID, "SUCCESS", detail); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

// accountAuditDetail reduces an account to the non-sensitive facts worth
// auditing. It never includes email, password or token material in a way that
// could leak; the email is intentionally omitted from before/after diffs.
func accountAuditDetail(a Account) map[string]any {
	return map[string]any{
		"displayName": a.DisplayName,
		"depotId":     a.DepotID,
		"outletId":    a.OutletID,
	}
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
