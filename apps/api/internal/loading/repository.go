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
	// RoutesForDepot lists the confirmed routes a loader at depotID may load on a
	// date, with the progress counts the route list shows.
	RoutesForDepot(ctx context.Context, depotID, date string) ([]RouteSummary, error)
}

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
	// audit and notify are optional sinks written in the same transaction as the
	// load state, so a rolled-back submission leaves no audit/notification.
	audit  AuditSink
	notify NotifySink
}

// AuditSink records one audit event inside the loading transaction.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, e AuditEvent) error
}

// NotifySink raises a dispatcher notification inside the loading transaction.
type NotifySink interface {
	NotifyDispatchersTx(ctx context.Context, tx pgx.Tx, depotID, outletID, notifType, title, message, reference string) error
}

// AuditEvent is the minimal audit fact loading emits.
type AuditEvent struct {
	Action     string
	EntityType string
	EntityID   string
	Actor      string
	DepotID    string
	OutletID   string
	Result     string
	Detail     map[string]any
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

// WithSinks attaches audit and notification sinks.
func (r *PGRepository) WithSinks(a AuditSink, n NotifySink) *PGRepository {
	r.audit = a
	r.notify = n
	return r
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
		SELECT r.route_id, r.vehicle_id, r.depot_id, r.route_date::text, r.trip_no,
		       r.brand, r.district, r.status,
		       v.type, v.temp, v.weight_cap_kg, v.volume_cap_m3
		FROM route r
		JOIN vehicle v ON v.vehicle_id = r.vehicle_id
		WHERE r.route_id = $1`, routeID).
		Scan(&rl.RouteID, &rl.VehicleID, &rl.DepotID, &rl.RouteDate, &rl.TripNo, &rl.Brand, &rl.District, &rl.Status,
			&rl.VehicleType, &rl.VehicleTemp, &rl.WeightCapKg, &rl.VolumeCapM3)
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
		       COALESCE(to_char(li.recorded_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		       rl.seq, o.outlet_id, ot.name, o.order_number, ot.dock_type, o.temp_requirement,
		       oi.total_weight_kg, oi.total_volume_m3
		FROM route_leg rl
		JOIN customer_order o ON o.order_id = rl.order_id
		JOIN outlet ot ON ot.outlet_id = o.outlet_id
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
			&l.RecordedBy, &l.RecordedAt, &l.Seq, &l.OutletID, &l.OutletName, &l.OrderNumber,
			&l.DockType, &l.TempRequirement, &l.WeightKg, &l.VolumeM3); err != nil {
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

// RoutesForDepot implements Repository.
//
// A loader needs to find the work before they can do it: the picking list is
// keyed by route id, and nothing else exposes a route to a loader (GET /routes
// is dispatcher-only). This is that list, and it is depot-scoped for the same
// reason the picking list is — a loader at Peliyagoda has no business seeing
// Kandy's dock.
//
// Only CONFIRMED routes appear. A DRAFT route is still being planned, and a
// DISPATCHED one has left; neither is loadable, and showing them would invite a
// loader to start work the server would then refuse.
func (r *PGRepository) RoutesForDepot(ctx context.Context, depotID, date string) ([]RouteSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.route_id, r.vehicle_id, r.depot_id, r.route_date::text, r.trip_no,
		       r.brand, r.district, r.status,
		       COUNT(DISTINCT leg.leg_id)      AS stops,
		       COUNT(oi.order_item_id)         AS lines,
		       COUNT(*) FILTER (
		           WHERE li.order_item_id IS NOT NULL
		             AND li.loaded_qty + li.damaged_qty + li.missing_qty = oi.quantity
		       )                               AS lines_complete,
		       COALESCE(SUM(COALESCE(li.damaged_qty, 0) + COALESCE(li.missing_qty, 0)), 0) AS shortfall_qty
		FROM route r
		JOIN route_leg leg ON leg.route_id = r.route_id
		JOIN order_item oi ON oi.order_id = leg.order_id
		LEFT JOIN load_item li ON li.route_id = r.route_id AND li.order_item_id = oi.order_item_id
		WHERE r.depot_id = $1 AND r.route_date = $2::date AND r.status = 'CONFIRMED'
		GROUP BY r.route_id, r.vehicle_id, r.depot_id, r.route_date, r.trip_no,
		         r.brand, r.district, r.status
		ORDER BY r.trip_no, r.vehicle_id`, depotID, date)
	if err != nil {
		return nil, fmt.Errorf("list loader routes: %w", err)
	}
	defer rows.Close()

	summaries := make([]RouteSummary, 0)
	for rows.Next() {
		var s RouteSummary
		if err := rows.Scan(&s.RouteID, &s.VehicleID, &s.DepotID, &s.RouteDate, &s.TripNo,
			&s.Brand, &s.District, &s.Status, &s.Stops, &s.Lines, &s.LinesComplete,
			&s.ShortfallQty); err != nil {
			return nil, fmt.Errorf("scan loader route: %w", err)
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loader routes: %w", err)
	}
	return summaries, nil
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

	if err := r.emitSideEffects(ctx, tx, routeID, actor, updates); err != nil {
		return RouteLoading{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RouteLoading{}, fmt.Errorf("commit loading tx: %w", err)
	}

	// Return the resulting state (reads committed data).
	return r.RouteLoading(ctx, routeID)
}

// emitSideEffects writes an audit row per line and, when any line has a
// shortfall, one dispatcher notification — all on the loading transaction.
func (r *PGRepository) emitSideEffects(ctx context.Context, tx pgx.Tx, routeID, actor string, updates []LineUpdate) error {
	// Resolve the route's depot and a representative outlet for scoping.
	var depotID, outletID string
	_ = tx.QueryRow(ctx, `
		SELECT route.depot_id, MIN(leg.to_outlet)
		FROM route JOIN route_leg leg ON leg.route_id = route.route_id
		WHERE route.route_id = $1
		GROUP BY route.depot_id`, routeID).Scan(&depotID, &outletID)

	shortfall := false
	for _, u := range updates {
		if r.audit != nil {
			if err := r.audit.RecordTx(ctx, tx, AuditEvent{
				Action: "LOAD_RECORDED", EntityType: "LOAD_ITEM", EntityID: u.OrderItemID,
				Actor: actor, DepotID: depotID, OutletID: outletID, Result: "SUCCESS",
				Detail: map[string]any{"routeId": routeID, "loaded": u.LoadedQty, "damaged": u.DamagedQty, "missing": u.MissingQty},
			}); err != nil {
				return fmt.Errorf("audit load line: %w", err)
			}
		}
		if u.DamagedQty+u.MissingQty > 0 {
			shortfall = true
		}
	}
	if shortfall && r.notify != nil {
		if err := r.notify.NotifyDispatchersTx(ctx, tx, depotID, outletID, "SHORTFALL",
			"Loading shortfall", "A load on a route at this depot has missing or damaged items.",
			"shortfall:"+routeID); err != nil {
			return fmt.Errorf("notify dispatcher of shortfall: %w", err)
		}
	}
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
