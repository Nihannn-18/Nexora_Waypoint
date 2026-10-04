package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/audit"
	"waypoint.lk/api/internal/seed"
)

// operationalTables is every table the running app writes to, children before
// parents so each DELETE satisfies the foreign keys. A reset empties these and
// nothing else.
//
// Deliberately kept:
//   - reference data (depot, outlet, vehicle, item, calendar, travel tables),
//     which the seed upserts anyway;
//   - app_user and session, so the dispatcher who pressed Reset stays signed in;
//   - audit_log, which is append-only by contract (internal/audit). The reset
//     itself is recorded there.
var operationalTables = []string{
	"order_item_delivery",
	"delivery_event",
	"load_item",
	"receipt_line",
	"receipt",
	"notification",
	"allocation",
	"deferral_log",
	"planning_result",
	"planning_job",
	"route_leg",
	"route",
	"order_queue_close",
	"vehicle_fuel_usage",
	// Per-day driver-to-vehicle assignments. Cleared so the seed restores the
	// demo assignment (seed-driver on VEH014): the seed's insert only tolerates
	// a vehicle conflict, so a demo driver moved to another vehicle would
	// otherwise make the re-seed fail on the one-vehicle-per-driver rule.
	"driver_vehicle_assignment",
	"order_item",
	"customer_order",
}

// resetLockKey serialises concurrent resets. Two dispatchers pressing Reset at
// once would otherwise interleave deletes and seed inserts.
const resetLockKey = "waypoint.demo.reset"

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

// RecordClockJump implements Repository.
func (r *PGRepository) RecordClockJump(ctx context.Context, actor, depotID string, stage Stage, from, to time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin demo clock audit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := audit.RecordTx(ctx, tx, string(audit.ActionDemoClockSet), string(audit.EntityDemo), string(stage),
		actor, depotID, "", string(audit.ResultSuccess), map[string]any{
			"stage": string(stage),
			"from":  from.Format(time.RFC3339),
			"to":    to.Format(time.RFC3339),
		}); err != nil {
		return fmt.Errorf("audit demo clock jump: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit demo clock audit: %w", err)
	}
	return nil
}

// Reset implements Repository. Clearing, re-seeding and the audit row share one
// transaction: a failure anywhere leaves the database exactly as it was.
func (r *PGRepository) Reset(ctx context.Context, actor, depotID string, clockTo time.Time) (ResetSummary, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ResetSummary{}, fmt.Errorf("begin demo reset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, resetLockKey); err != nil {
		return ResetSummary{}, fmt.Errorf("lock demo reset: %w", err)
	}

	sum := ResetSummary{Cleared: make(map[string]int64, len(operationalTables))}
	for _, table := range operationalTables {
		// table comes from the fixed list above, never from a request.
		tag, err := tx.Exec(ctx, "DELETE FROM "+pgx.Identifier{table}.Sanitize())
		if err != nil {
			return ResetSummary{}, fmt.Errorf("clear %s: %w", table, err)
		}
		sum.Cleared[table] = tag.RowsAffected()
	}

	res, err := seed.RunTx(ctx, tx)
	if err != nil {
		return ResetSummary{}, fmt.Errorf("re-seed: %w", err)
	}
	sum.DemoOrders = res.DemoOrders
	sum.DemoVehicleDays = res.DemoAvailability

	cleared := make(map[string]any, len(sum.Cleared))
	for k, v := range sum.Cleared {
		cleared[k] = v
	}
	if err := audit.RecordTx(ctx, tx, string(audit.ActionDemoReset), string(audit.EntityDemo), "",
		actor, depotID, "", string(audit.ResultSuccess), map[string]any{
			"cleared":    cleared,
			"demoOrders": sum.DemoOrders,
			"clockTo":    clockTo.Format(time.RFC3339),
		}); err != nil {
		return ResetSummary{}, fmt.Errorf("audit demo reset: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ResetSummary{}, fmt.Errorf("commit demo reset: %w", err)
	}
	return sum, nil
}
