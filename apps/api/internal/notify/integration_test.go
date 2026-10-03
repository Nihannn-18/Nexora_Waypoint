package notify

import (
	"context"
	"os"
	"testing"
	"time"

	"waypoint.lk/api/internal/store"
)

// TestNotifyIntegration exercises the notification repository against a real
// PostgreSQL when one is reachable, and skips otherwise. It proves idempotent
// creation on (user_id, type, reference), dispatcher fan-out, recipient scoping,
// read state and unread count.
func TestNotifyIntegration(t *testing.T) {
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

	var depotID string
	if err := db.Pool().QueryRow(ctx, `
		INSERT INTO depot (code, name) VALUES ('ANDEPOT', 'Audit Notify Depot')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name RETURNING depot_id`).Scan(&depotID); err != nil {
		t.Fatalf("depot: %v", err)
	}
	// Two dispatchers at the depot, one elsewhere.
	for _, u := range []struct{ id, depot string }{
		{"andisp1", depotID}, {"andisp2", depotID}, {"andisp3", ""},
	} {
		var depot any
		if u.depot != "" {
			depot = u.depot
		}
		if _, err := db.Pool().Exec(ctx, `
			INSERT INTO app_user (user_id, email, role, depot_id) VALUES ($1,$2,'DISPATCHER',$3)
			ON CONFLICT (user_id) DO UPDATE SET depot_id = EXCLUDED.depot_id`, u.id, u.id+"@x.lk", depot); err != nil {
			t.Fatalf("user %s: %v", u.id, err)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.Pool().Exec(bg, `DELETE FROM notification WHERE user_id IN ('andisp1','andisp2') OR reference LIKE 'it-%'`)
		_, _ = db.Pool().Exec(bg, `DELETE FROM app_user WHERE user_id IN ('andisp1','andisp2','andisp3')`)
	})

	repo := NewPGRepository(db.Pool())

	// Dispatcher fan-out for the depot (not andisp3, which has no depot).
	ids, err := repo.DispatcherUserIDs(ctx, depotID)
	if err != nil {
		t.Fatalf("resolve dispatchers: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("dispatchers = %v, want 2", ids)
	}

	// Create for all; idempotent on reference.
	if err := txCreateForDispatchers(ctx, db, depotID, "SHORTFALL", "ref-1"); err != nil {
		t.Fatalf("create dispatchers: %v", err)
	}
	if err := txCreateForDispatchers(ctx, db, depotID, "SHORTFALL", "ref-1"); err != nil {
		t.Fatalf("create duplicate: %v", err)
	}
	items, err := repo.List(ctx, Recipient{UserID: "andisp1"}, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("dispatcher 1 notifications = %d, want 1 (idempotent)", len(items))
	}
	// andisp3 (no depot) must not have received it.
	other, _ := repo.List(ctx, Recipient{UserID: "andisp3"}, 50, 0)
	if len(other) != 0 {
		t.Fatalf("andisp3 notifications = %d, want 0", len(other))
	}

	// Unread count and read state.
	if n, _ := repo.UnreadCount(ctx, Recipient{UserID: "andisp1"}); n != 1 {
		t.Fatalf("unread = %d, want 1", n)
	}
	if err := repo.MarkRead(ctx, Recipient{UserID: "andisp1"}, items[0].ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if n, _ := repo.UnreadCount(ctx, Recipient{UserID: "andisp1"}); n != 0 {
		t.Fatalf("unread after read = %d, want 0", n)
	}
	// Marking again is idempotent, not an error.
	if err := repo.MarkRead(ctx, Recipient{UserID: "andisp1"}, items[0].ID); err != nil {
		t.Fatalf("re-mark: %v", err)
	}
	// Another user cannot mark someone else's notification.
	if err := repo.MarkRead(ctx, Recipient{UserID: "andisp2"}, items[0].ID); err == nil {
		t.Fatal("cross-user mark read should fail")
	}
}

// txCreateForDispatchers runs the transaction-bound creator for the test.
func txCreateForDispatchers(ctx context.Context, db *store.Store, depotID, typ, ref string) error {
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := CreateForDispatchersTx(ctx, tx, depotID, "", typ, "title", "message", ref); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
