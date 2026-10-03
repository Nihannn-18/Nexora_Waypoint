package loading

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists and loads loading state for a route.
type Repository interface {
	// RouteLoading returns the picking list for a route: every order line on its
	// legs, with the ordered quantity and any recorded load state.
	RouteLoading(ctx context.Context, routeID string) (RouteLoading, error)
	// RecordShortfalls applies a full set of line updates for a route in one
	// transaction, upserting each load_item, and returns the resulting state.
	RecordShortfalls(ctx context.Context, routeID, actor string, updates []LineUpdate) (RouteLoading, error)
}

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

// RouteLoading implements Repository.
//
// The picking list is the route's order lines, ordered by the leg stop order
// (the client renders that reversed, per the loader design). The expected
// quantity comes from order_item.quantity via route_leg — never from
// planning_result.
func (r *PGRepository) RouteLoading(ctx context.Context, routeID string) (RouteLoading, error) {
	var rl RouteLoading
	err := r.pool.QueryRow(ctx, `
		SELECT route_id, vehicle_id, depot_id, route_date::text, trip_no, brand, district, status
		FROM route WHERE route_id = $1`, routeID).
		Scan(&rl.RouteID, &rl.VehicleID, &rl.DepotID, &rl.RouteDate, &rl.TripNo, &rl.Brand, &rl.District, &rl.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return RouteLoading{}, fmt.Errorf("%w: route %s", ErrNotFound, routeID)
	}
	if err != nil {
		return RouteLoading{}, fmt.Errorf("load route: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT oi.order_item_id, oi.order_id, oi.item_id, it.sku, it.name, oi.quantity,
		       COALESCE(li.load_item_id::text, ''), COALESCE(li.loaded_qty, 0),
		       COALESCE(li.damaged_qty, 0), COALESCE(li.missing_qty, 0),
		       COALESCE(li.photo_ref, ''), COALESCE(li.recorded_by, ''),
		       COALESCE(to_char(li.recorded_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
		FROM route_leg rl
		JOIN order_item oi ON oi.order_id = rl.order_id
		JOIN item it ON it.item_id = oi.item_id
		LEFT JOIN load_item li ON li.route_id = rl.route_id AND li.order_item_id = oi.order_item_id
		WHERE rl.route_id = $1
		ORDER BY rl.seq, oi.order_item_id`, routeID)
	if err != nil {
		return RouteLoading{}, fmt.Errorf("load picking list: %w", err)
	}
	defer rows.Close()

	lines := make([]Line, 0)
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.OrderItemID, &l.OrderID, &l.ItemID, &l.SKU, &l.Name, &l.OrderedQty,
			&l.LoadItemID, &l.LoadedQty, &l.DamagedQty, &l.MissingQty, &l.PhotoRef,
			&l.RecordedBy, &l.RecordedAt); err != nil {
			return RouteLoading{}, fmt.Errorf("scan picking line: %w", err)
		}
		l.RouteID = routeID
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return RouteLoading{}, fmt.Errorf("iterate picking lines: %w", err)
	}
	rl.Lines = lines
	return rl, nil
}

// RecordShortfalls implements Repository.
//
// Every submitted line is validated by the service against the ordered quantity
// before this is called; here the writes are made atomic:
//
//   - The route's lines are read inside the transaction to resolve the
//     authoritative ordered quantity and confirm each order_item belongs to the
//     route. A client cannot load a line that is not on the route.
//   - Each line is upserted on (route_id, order_item_id). The unique index makes
//     a concurrent double-submit race-safe: one insert wins, the other conflicts
//     or updates the same row — never a duplicate.
func (r *PGRepository) RecordShortfalls(ctx context.Context, routeID, actor string, updates []LineUpdate) (RouteLoading, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RouteLoading{}, fmt.Errorf("begin loading tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Confirm the route is CONFIRMED before any write. Loading is only for an
	// operational (confirmed) route.
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM route WHERE route_id = $1 FOR UPDATE`, routeID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteLoading{}, fmt.Errorf("%w: route %s", ErrNotFound, routeID)
		}
		return RouteLoading{}, fmt.Errorf("lock route: %w", err)
	}
	if status != "CONFIRMED" {
		return RouteLoading{}, fmt.Errorf("%w: route %s is %s, not CONFIRMED", ErrConflict, routeID, status)
	}

	for _, u := range updates {
		// The ordered quantity and route membership are read inside the
		// transaction, so the client cannot supply either.
		var orderedQty int
		var onRoute bool
		err := tx.QueryRow(ctx, `
			SELECT oi.quantity, TRUE
			FROM route_leg rl
			JOIN order_item oi ON oi.order_id = rl.order_id
			WHERE rl.route_id = $1 AND oi.order_item_id = $2
			LIMIT 1`, routeID, u.OrderItemID).Scan(&orderedQty, &onRoute)
		if errors.Is(err, pgx.ErrNoRows) {
			return RouteLoading{}, fmt.Errorf("%w: order line %s is not on route %s", ErrInvalid, u.OrderItemID, routeID)
		}
		if err != nil {
			return RouteLoading{}, fmt.Errorf("resolve order line %s: %w", u.OrderItemID, err)
		}
		if err := ValidateUpdate(u, orderedQty); err != nil {
			return RouteLoading{}, err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO load_item (
				route_id, order_item_id, ordered_qty, loaded_qty, damaged_qty, missing_qty,
				photo_ref, recorded_by, recorded_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now())
			ON CONFLICT (route_id, order_item_id) DO UPDATE
			SET loaded_qty = EXCLUDED.loaded_qty,
			    damaged_qty = EXCLUDED.damaged_qty,
			    missing_qty = EXCLUDED.missing_qty,
			    photo_ref = EXCLUDED.photo_ref,
			    recorded_by = EXCLUDED.recorded_by,
			    recorded_at = now()`,
			routeID, u.OrderItemID, orderedQty, u.LoadedQty, u.DamagedQty, u.MissingQty,
			nullable(u.PhotoRef), nullable(actor))
		if err != nil {
			return RouteLoading{}, fmt.Errorf("record load line %s: %w", u.OrderItemID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return RouteLoading{}, fmt.Errorf("commit loading tx: %w", err)
	}

	// Return the resulting state (reads committed data).
	return r.RouteLoading(ctx, routeID)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
