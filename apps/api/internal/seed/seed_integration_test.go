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

	var users int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM app_user WHERE email LIKE '%@waypoint.lk'`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != second.Users {
		t.Fatalf("app_user has %d seeded rows after two runs, want %d", users, second.Users)
	}
}
