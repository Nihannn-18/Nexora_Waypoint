package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// These tests run against PostgreSQL when WAYPOINT_TEST_DATABASE_URL points at
// a disposable database, and skip otherwise so the unit suite stays green on a
// machine without one.

func openTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv("WAYPOINT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WAYPOINT_TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	db, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(db.Close)
	return db, ctx
}

func TestMigrateIsIdempotentAndEnforcesSchemaRules(t *testing.T) {
	db, ctx := openTestStore(t)

	// Two concurrent callers model two API instances starting together; on a
	// fresh database the advisory lock must stop them racing on the schema.
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- db.Migrate(ctx) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent migrate: %v", err)
		}
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate on an up-to-date database: %v", err)
	}

	// The database itself must enforce these, not only the services (CLAUDE.md §10,
	// docs/data-model.md).
	required := []struct {
		table, definition string
	}{
		{"route", "UNIQUE (vehicle_id, route_date, trip_no)"},
		{"route", "CHECK ((trip_no = ANY (ARRAY[1, 2])))"},
		{"route_leg", "UNIQUE (route_id, seq)"},
		{"route_leg", "UNIQUE (route_id, order_id)"},
		{"delivery_event", "UNIQUE (client_event_id)"},
	}
	for _, rc := range required {
		var found bool
		err := db.Pool().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conrelid = $1::regclass AND pg_get_constraintdef(oid) = $2
			)`, rc.table, rc.definition).Scan(&found)
		if err != nil {
			t.Fatalf("look up %s constraint on %s: %v", rc.definition, rc.table, err)
		}
		if !found {
			t.Errorf("%s is missing constraint %s", rc.table, rc.definition)
		}
	}
}

func TestInTxCommitsAndRollsBack(t *testing.T) {
	db, ctx := openTestStore(t)
	const table = "store_intx_probe"
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	exists := func() bool {
		t.Helper()
		var ok bool
		if err := db.Pool().QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatalf("check table: %v", err)
		}
		return ok
	}

	boom := errors.New("boom")
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `CREATE TABLE `+table+` (id INT)`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx error = %v, want the callback's error", err)
	}
	if exists() {
		t.Fatal("work inside a failed transaction was committed")
	}

	if err := db.InTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `CREATE TABLE `+table+` (id INT)`)
		return err
	}); err != nil {
		t.Fatalf("InTx: %v", err)
	}
	if !exists() {
		t.Fatal("work inside a successful transaction was not committed")
	}
}
