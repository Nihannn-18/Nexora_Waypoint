package demo

import (
	"context"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/seed"
	"waypoint.lk/api/internal/store"
)

// TestResetIntegration runs the real reset against PostgreSQL. A reset empties
// every operational table, so besides WAYPOINT_TEST_DATABASE_URL it needs an
// explicit WAYPOINT_TEST_DEMO_RESET=1: it must never wipe a database by
// accident, and it must not run while other packages' integration tests (which
// `go test ./...` runs in parallel) hold rows in the same database.
func TestResetIntegration(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("WAYPOINT_TEST_DEMO_RESET") != "1" {
		t.Skip("set WAYPOINT_TEST_DATABASE_URL and WAYPOINT_TEST_DEMO_RESET=1 to run the destructive demo reset test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := seed.Run(ctx, db.Pool()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pool := db.Pool()

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}

	// Move the demo day away from its seeded state the way a walkthrough does.
	if _, err := pool.Exec(ctx, `UPDATE customer_order SET status = 'DEFERRED' WHERE order_number = (SELECT min(order_number) FROM customer_order WHERE requested_delivery_date = '2026-09-26')`); err != nil {
		t.Fatalf("defer an order: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notification (user_id, type, title, message, reference) VALUES ('seed-dispatcher', 'SHORTFALL', 't', 'm', 'demo-reset-test')`); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	// A dispatcher moves the demo driver to another vehicle for the demo day.
	// The re-seed alone cannot undo this (its insert only tolerates a vehicle
	// conflict), so the reset must clear assignments first.
	if _, err := pool.Exec(ctx, `UPDATE driver_vehicle_assignment SET vehicle_id = 'VEH020' WHERE driver_id = 'seed-driver' AND assignment_date = '2026-09-26'`); err != nil {
		t.Fatalf("reassign demo driver: %v", err)
	}
	usersBefore := count(`SELECT count(*) FROM app_user`)
	outletsBefore := count(`SELECT count(*) FROM outlet`)
	auditBefore := count(`SELECT count(*) FROM audit_log`)

	repo := NewPGRepository(pool)
	start := time.Date(2026, 9, 25, 15, 40, 0, 0, time.FixedZone("+0530", 5*3600+1800))
	sum, err := repo.Reset(ctx, "seed-dispatcher", "", start)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if sum.DemoOrders != 85 {
		t.Errorf("demo orders = %d, want 85", sum.DemoOrders)
	}
	if sum.Cleared["notification"] < 1 {
		t.Errorf("cleared notifications = %d, want at least 1", sum.Cleared["notification"])
	}

	if n := count(`SELECT count(*) FROM customer_order`); n != 85 {
		t.Errorf("orders after reset = %d, want exactly the 85 seeded", n)
	}
	if n := count(`SELECT count(*) FROM customer_order WHERE status <> 'CONFIRMED'`); n != 0 {
		t.Errorf("%d orders not back to CONFIRMED", n)
	}
	if n := count(`SELECT count(*) FROM notification`); n != 0 {
		t.Errorf("notifications after reset = %d, want 0", n)
	}
	if n := count(`SELECT count(*) FROM driver_vehicle_assignment WHERE driver_id = 'seed-driver' AND vehicle_id = 'VEH014' AND assignment_date = '2026-09-26'`); n != 1 {
		t.Errorf("demo assignment seed-driver→VEH014 rows = %d, want 1 restored", n)
	}
	if n := count(`SELECT count(*) FROM driver_vehicle_assignment`); n != 1 {
		t.Errorf("assignments after reset = %d, want only the demo one", n)
	}
	if n := count(`SELECT count(*) FROM app_user`); n != usersBefore {
		t.Errorf("users = %d, want %d kept", n, usersBefore)
	}
	if n := count(`SELECT count(*) FROM outlet`); n != outletsBefore {
		t.Errorf("outlets = %d, want %d kept", n, outletsBefore)
	}
	// The audit trail is append-only: nothing removed, one DEMO_RESET added.
	if n := count(`SELECT count(*) FROM audit_log`); n != auditBefore+1 {
		t.Errorf("audit rows = %d, want %d (kept + the reset)", n, auditBefore+1)
	}
	if n := count(`SELECT count(*) FROM audit_log WHERE action = 'DEMO_RESET' AND actor = 'seed-dispatcher'`); n < 1 {
		t.Error("the reset is not on the audit trail")
	}

	// A second reset in a row is a no-op on the data and still succeeds.
	if _, err := repo.Reset(ctx, "seed-dispatcher", "", start); err != nil {
		t.Fatalf("second reset: %v", err)
	}
	if n := count(`SELECT count(*) FROM customer_order`); n != 85 {
		t.Errorf("orders after second reset = %d, want 85", n)
	}
}
