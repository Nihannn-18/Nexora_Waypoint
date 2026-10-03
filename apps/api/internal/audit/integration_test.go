package audit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"waypoint.lk/api/internal/store"
)

// TestAuditIntegration exercises the audit repository against a real PostgreSQL
// when one is reachable, and skips otherwise. It proves append-only record +
// query, filtering/pagination, the scope envelope, and rollback atomicity: an
// audit row written in a rolled-back transaction must not survive.
func TestAuditIntegration(t *testing.T) {
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

	repo := NewPGRepository(db.Pool())
	entity := "it-audit-entity"
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM audit_log WHERE entity_id = $1`, entity)
	})

	// Write via WithTx (committed) twice, plus a filtered-out record.
	err = repo.WithTx(ctx, func(ctx context.Context, rec Recorder) error {
		if err := rec.Record(ctx, Event{
			Action: ActionRouteConfirmed, EntityType: EntityRoute, EntityID: entity,
			Actor: "u-disp", DepotID: "d-peli", OutletID: "OUT1", Result: ResultSuccess,
			Detail: map[string]any{"routeId": "R1", "orders": 3},
		}); err != nil {
			return err
		}
		return rec.Record(ctx, Event{
			Action: ActionDeliveryRecorded, EntityType: EntityDelivery, EntityID: entity,
			Actor: "u-disp", DepotID: "d-kandy", Result: ResultSuccess,
		})
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	// Query by entity: two records, newest first, envelope decoded.
	recs, err := repo.List(ctx, Filter{EntityID: entity, Limit: 50})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	found := map[string]Record{}
	for _, r := range recs {
		found[r.Action] = r
	}
	rc := found[string(ActionRouteConfirmed)]
	if rc.DepotID != "d-peli" || rc.Result != "SUCCESS" || rc.Detail["orders"] != float64(3) {
		t.Fatalf("envelope = %+v", rc)
	}

	// Filter by depot.
	byDepot, err := repo.List(ctx, Filter{EntityID: entity, DepotID: "d-kandy"})
	if err != nil {
		t.Fatalf("list by depot: %v", err)
	}
	if len(byDepot) != 1 || byDepot[0].Action != string(ActionDeliveryRecorded) {
		t.Fatalf("by depot = %+v", byDepot)
	}

	// Pagination: page size 1.
	page, err := repo.List(ctx, Filter{EntityID: entity, Limit: 1, Offset: 0})
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("page size = %d, want 1", len(page))
	}

	// Rollback atomicity: a row written in a transaction that rolls back must not
	// persist.
	rolledBack := "it-audit-rollback"
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM audit_log WHERE entity_id = $1`, rolledBack)
	})
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := RecordTx(ctx, tx, "ROUTE_CONFIRMED", "ROUTE", rolledBack, "u", "d", "o", "SUCCESS", nil); err != nil {
		t.Fatalf("record in tx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
		t.Fatalf("rollback: %v", err)
	}
	after, err := repo.List(ctx, Filter{EntityID: rolledBack})
	if err != nil {
		t.Fatalf("list rollback: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("rolled-back audit survived: %+v", after)
	}
}
