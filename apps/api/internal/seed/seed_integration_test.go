package seed

import (
	"context"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/store"
)

// This test runs the real migration + seed against PostgreSQL when a database
// is reachable, and skips otherwise so the unit suite stays green on a machine
// without one. Point WAYPOINT_TEST_DATABASE_URL at a disposable database (for
// example the compose postgres) to exercise it.
//
// It proves the two properties the specification requires of seeding:
// idempotency (running twice changes nothing) and foreign-key ordering (the
// seed inserts depots before the rows that reference them).

func TestSeedIntegration(t *testing.T) {
	dsn := os.Getenv("WAYPOINT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	first, err := Run(ctx, db.Pool())
	if err != nil {
		t.Fatalf("first seed run: %v", err)
	}

	second, err := Run(ctx, db.Pool())
	if err != nil {
		t.Fatalf("second seed run: %v", err)
	}
	if first != second {
		t.Fatalf("seed is not idempotent: first %+v, second %+v", first, second)
	}

	if second.Depots != 2 {
		t.Errorf("depots = %d, want 2", second.Depots)
	}
	if second.Outlets != 120 {
		t.Errorf("outlets = %d, want 120", second.Outlets)
	}
	if second.Vehicles != 60 {
		t.Errorf("vehicles = %d, want 60", second.Vehicles)
	}
	if second.DistrictTravel != 12 {
		t.Errorf("district_travel = %d, want 12", second.DistrictTravel)
	}
	if second.ServiceAllowance != 9 {
		t.Errorf("service_allowance = %d, want 9", second.ServiceAllowance)
	}

	if second.Users != 4 {
		t.Errorf("users = %d, want 4", second.Users)
	}

	// Row counts in the database must match the reported counts, proving the
	// upserts did not duplicate on the second run.
	var outlets int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM outlet`).Scan(&outlets); err != nil {
		t.Fatalf("count outlets: %v", err)
	}
	if outlets != second.Outlets {
		t.Fatalf("outlet table has %d rows after two runs, want %d", outlets, second.Outlets)
	}

	if second.DemoOrders != 85 {
		t.Errorf("demo orders = %d, want 85", second.DemoOrders)
	}
	if second.DemoAvailability != 60 {
		t.Errorf("demo availability rows = %d, want 60", second.DemoAvailability)
	}

	// Each order's lines must sum to the CSV totals exactly, with one order
	// row per order_ref even after two runs.
	var orders, mismatched int
	if err := db.Pool().QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE o.total_weight_kg <> l.w OR o.total_volume_m3 <> l.v OR o.total_units <> l.q)
		FROM customer_order o
		JOIN (SELECT order_id, sum(total_weight_kg) w, sum(total_volume_m3) v, sum(quantity) q
		      FROM order_item GROUP BY order_id) l USING (order_id)
		WHERE o.order_number LIKE 'S1-%'`).Scan(&orders, &mismatched); err != nil {
		t.Fatalf("check order totals: %v", err)
	}
	if orders != second.DemoOrders || mismatched != 0 {
		t.Fatalf("orders with lines = %d (want %d), total mismatches = %d (want 0)", orders, second.DemoOrders, mismatched)
	}

	var inWorkshop int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM vehicle_daily_availability WHERE date = '2026-09-26' AND NOT available`).Scan(&inWorkshop); err != nil {
		t.Fatalf("count unavailable vehicles: %v", err)
	}
	if inWorkshop != 10 {
		t.Errorf("unavailable vehicles = %d, want 10", inWorkshop)
	}

	// Re-seeding must not rewind an order that has moved on.
	if _, err := db.Pool().Exec(ctx, `UPDATE customer_order SET status = 'ALLOCATED' WHERE order_number = 'S1-000'`); err != nil {
		t.Fatalf("advance order: %v", err)
	}
	if _, err := Run(ctx, db.Pool()); err != nil {
		t.Fatalf("third seed run: %v", err)
	}
	var status string
	if err := db.Pool().QueryRow(ctx, `SELECT status FROM customer_order WHERE order_number = 'S1-000'`).Scan(&status); err != nil {
		t.Fatalf("read order status: %v", err)
	}
	if status != "ALLOCATED" {
		t.Fatalf("re-seed rewound S1-000 to %s", status)
	}

	var users int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM app_user WHERE email LIKE '%@waypoint.lk'`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != second.Users {
		t.Fatalf("app_user has %d seeded rows after two runs, want %d", users, second.Users)
	}

	// The supplied calendar.csv ends before the demo delivery day, so the seed
	// extension must have filled the window: Sat 26 Sep 2026 is operating and
	// Sun 27 Sep 2026 is not. This is what makes planning run on the demo day.
	var operating bool
	if err := db.Pool().QueryRow(ctx, `SELECT is_operating FROM calendar_day WHERE date = '2026-09-26'`).Scan(&operating); err != nil {
		t.Fatalf("read demo delivery day calendar row: %v", err)
	}
	if !operating {
		t.Fatal("demo delivery day 2026-09-26 must be an operating day for planning to run")
	}
	var sundayOperating bool
	if err := db.Pool().QueryRow(ctx, `SELECT is_operating FROM calendar_day WHERE date = '2026-09-27'`).Scan(&sundayOperating); err != nil {
		t.Fatalf("read demo Sunday calendar row: %v", err)
	}
	if sundayOperating {
		t.Fatal("Sunday 2026-09-27 must not be operating")
	}
	// The order day is also present so a next-operating-day roll finds a row.
	var orderDayRows int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM calendar_day WHERE date >= '2026-09-25' AND date <= '2026-10-03'`).Scan(&orderDayRows); err != nil {
		t.Fatalf("count demo calendar window: %v", err)
	}
	if orderDayRows < 9 {
		t.Fatalf("demo calendar window has %d rows, want at least 9", orderDayRows)
	}
}
