package useradmin

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"waypoint.lk/api/internal/audit"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/password"
	"waypoint.lk/api/internal/store"
)

// testAudit adapts audit.RecordTx to the useradmin AuditSink for the
// integration test.
type testAudit struct{}

func (testAudit) RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error {
	return audit.RecordTx(ctx, tx, action, entityType, entityID, actor, depotID, outletID, result, detail)
}

// TestUserAdminIntegration exercises the store against a real PostgreSQL when
// one is reachable, and skips otherwise. It proves the migration applies, the
// password is hashed, the reset token is stored hashed and is single-use, and a
// password reset revokes sessions.
func TestUserAdminIntegration(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool := db.Pool()

	// A depot and outlet for scope resolution. The outlet is upserted (not
	// DO NOTHING) so a stale row from an earlier run under a recreated depot
	// cannot leave the test pointing at a depot that no longer exists.
	var depotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('UADEPOT', 'User Admin Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type,
		                    parking_constraint, window_open_time, window_close_time)
		VALUES ('OUT991','IT Outlet','FRESH','IT', $1, 'REAR_DOCK','NORMAL','08:00','12:00')
		ON CONFLICT (outlet_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, depotID); err != nil {
		t.Fatalf("outlet: %v", err)
	}

	// Clean up any prior run so the test is repeatable.
	_, _ = pool.Exec(ctx, `DELETE FROM app_user WHERE email LIKE 'it-%@waypoint.lk'`)

	// A demo-mode clock: an instant in the seeded (past) day, exactly as
	// DEMO_MODE runs. Both minting and consuming must agree on this domain, or
	// every token would look expired against the database's real-time now().
	demoNow := time.Date(2026, 9, 25, 15, 40, 0, 0, time.FixedZone("+0530", 5*3600+30*60))
	svc := NewService(NewPGStore(pool, testAudit{}), fixedClock{demoNow})
	actor := "it-actor"

	// Create a driver: password is hashed, depot is set, audit row written.
	driver, err := svc.CreateAccount(ctx, CreateInput{
		Email:           "it-driver@waypoint.lk",
		DisplayName:     "IT Driver",
		Role:            domain.RoleDriver,
		DepotID:         depotID,
		InitialPassword: "initial-pass",
	}, actor)
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM app_user WHERE user_id = $1`, driver.UserID).Scan(&hash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if hash == "initial-pass" || hash == "" {
		t.Fatal("password was not hashed")
	}
	if err := password.Verify("initial-pass", hash); err != nil {
		t.Fatalf("stored hash does not verify: %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'USER_CREATED' AND entity_id = $1`, driver.UserID).Scan(&auditCount); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("USER_CREATED audit rows = %d, want 1", auditCount)
	}

	// Duplicate email is a conflict.
	if _, err := svc.CreateAccount(ctx, CreateInput{
		Email: "it-driver@waypoint.lk", DisplayName: "D2", Role: domain.RoleLoader,
		DepotID: depotID, InitialPassword: "initial-pass",
	}, actor); !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("duplicate create err = %v, want ErrDuplicateEmail", err)
	}

	// Create a store manager: depot derives from the outlet.
	sm, err := svc.CreateAccount(ctx, CreateInput{
		Email: "it-store@waypoint.lk", DisplayName: "IT Store",
		Role: domain.RoleStoreManager, OutletID: "OUT991", InitialPassword: "initial-pass",
	}, actor)
	if err != nil {
		t.Fatalf("create store manager: %v", err)
	}
	if sm.DepotID != depotID {
		t.Fatalf("store manager depot = %q, want %q", sm.DepotID, depotID)
	}

	// A session for the driver, which a reset must revoke.
	var sessionCount int
	if _, err := pool.Exec(ctx, `INSERT INTO session (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour')`,
		driver.UserID, HashResetToken("session-token")); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Forgot: mint a token, stored hashed.
	raw, matched, err := svc.ForgotPassword(ctx, "it-driver@waypoint.lk")
	if err != nil || !matched {
		t.Fatalf("forgot password: matched=%v err=%v", matched, err)
	}
	var storedHash string
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM password_reset_token WHERE user_id = $1`, driver.UserID).Scan(&storedHash); err != nil {
		t.Fatalf("read token hash: %v", err)
	}
	if storedHash != HashResetToken(raw) {
		t.Fatal("stored token hash does not match HashResetToken(raw)")
	}
	if storedHash == raw {
		t.Fatal("raw token was stored")
	}

	// Reset: new password hashes, token is consumed, sessions are revoked.
	if err := svc.ResetPassword(ctx, ResetInput{Token: raw, NewPassword: "brand-new-pass", ConfirmPassword: "brand-new-pass"}); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	var newHash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM app_user WHERE user_id = $1`, driver.UserID).Scan(&newHash); err != nil {
		t.Fatalf("read new hash: %v", err)
	}
	if err := password.Verify("brand-new-pass", newHash); err != nil {
		t.Fatalf("new password does not verify: %v", err)
	}
	if err := password.Verify("initial-pass", newHash); err == nil {
		t.Fatal("old password still verifies after reset")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session WHERE user_id = $1`, driver.UserID).Scan(&sessionCount); err != nil {
		t.Fatalf("count session: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("sessions after reset = %d, want 0 (revoked)", sessionCount)
	}
	var used *time.Time
	if err := pool.QueryRow(ctx, `SELECT used_at FROM password_reset_token WHERE user_id = $1`, driver.UserID).Scan(&used); err != nil {
		t.Fatalf("read used_at: %v", err)
	}
	if used == nil {
		t.Fatal("reset token was not marked used")
	}

	// Reusing the token is refused.
	if err := svc.ResetPassword(ctx, ResetInput{Token: raw, NewPassword: "another-pass", ConfirmPassword: "another-pass"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("token reuse err = %v, want ErrInvalidResetToken", err)
	}

	// Deactivation revokes sessions and blocks a further reset.
	if _, err := pool.Exec(ctx, `INSERT INTO session (user_id, token_hash, expires_at) VALUES ($1, $2, now() + interval '1 hour')`,
		driver.UserID, HashResetToken("session-token-2")); err != nil {
		t.Fatalf("re-insert session: %v", err)
	}
	if _, err := svc.SetActive(ctx, driver.UserID, false, actor); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session WHERE user_id = $1`, driver.UserID).Scan(&sessionCount); err != nil {
		t.Fatalf("count session: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("sessions after deactivate = %d, want 0", sessionCount)
	}
	if _, matched, err := svc.ForgotPassword(ctx, "it-driver@waypoint.lk"); err != nil || matched {
		t.Fatalf("forgot for inactive account: matched=%v err=%v; want no token", matched, err)
	}
}

// fixedClock is a deterministic Clock pinned to one demo instant.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }
