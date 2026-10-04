package routes

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/domain"
)

// Repository persists and loads the operational route/allocation state. The
// confirmation write is a single transactional method because route + legs +
// allocations + order status + deferral log must all land together or not at
// all.
type Repository interface {
	// GetRoute returns a route with its legs, or ErrNotFound.
	GetRoute(ctx context.Context, routeID string) (Route, error)
	// ListRoutes returns routes for a date, optionally filtered by depot, with
	// legs, in a deterministic order.
	ListRoutes(ctx context.Context, routeDate string, depotID string) ([]Route, error)
	// ListDeferrals returns deferral_log rows, newest first, optionally filtered
	// by outlet.
	ListDeferrals(ctx context.Context, outletID string) ([]DeferralEntry, error)
	// Confirm applies a confirmation in one transaction.
	Confirm(ctx context.Context, plan ConfirmationPlan) (ConfirmationResult, error)
}

// DeferredOrderForConfirm is one order the dispatcher chose to leave unserved at
// confirmation, with its reason. It mirrors the deferral_log row.
type DeferredOrderForConfirm struct {
	OrderID        string
	ReasonType     string
	Reason         string
	ConstraintCode domain.ConstraintCode
	DeferredToDate string // YYYY-MM-DD, optional
}

// ConfirmationPlan is the fully-validated set of writes one confirmation makes.
// The service builds it after validation; the repository only persists it.
type ConfirmationPlan struct {
	DepotID   string
	RouteDate string
	// Routes to create, each with its ordered legs. Route status is CONFIRMED.
	Routes []Route
	// Deferred orders to record.
	Deferrals []DeferredOrderForConfirm
	// Actor is the confirming user id, recorded on allocations and deferrals.
	Actor string
	// PlanningJobID ties the confirmation to the proposal it came from.
	PlanningJobID string
}

// ConfirmationResult reports what was written.
type ConfirmationResult struct {
	RouteIDs        []string
	AllocatedOrders []string
	DeferredOrders  []string
}

// DeferralEntry is one row of the operational deferral history.
type DeferralEntry struct {
	DeferralID     string
	OrderID        string
	OrderNumber    string
	OutletID       string
	ReasonType     string
	Reason         string
	ConstraintCode string
	DecidedBy      string
	DecidedAt      time.Time
	DeferredToDate string
}

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

// Confirm writes a confirmation inside one transaction.
//
// Concurrency and idempotency:
//   - The orders being confirmed are locked with SELECT ... FOR UPDATE in a
//     deterministic id order before any check, so two concurrent confirmations
//     of the same order serialise instead of both passing the "not already
//     allocated" check. This is the row-lock protection the data model requires
//     in place of a partial unique index that the schema cannot express.
//   - route has UNIQUE (vehicle_id, route_date, trip_no), so a duplicate route
//     for the same vehicle/trip/day fails at the database.
//   - route_leg has UNIQUE (route_id, order_id), so an order cannot appear twice
//     on a route.
//   - Active-allocation duplication for an order on one date is prevented by the
//     FOR UPDATE lock plus the in-transaction duplicate check.
func (r *PGRepository) Confirm(ctx context.Context, plan ConfirmationPlan) (ConfirmationResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ConfirmationResult{}, fmt.Errorf("begin confirmation tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Every order touched by this confirmation, in a deterministic order, so
	// concurrent transactions lock in the same sequence and cannot deadlock.
	orderIDs := collectOrderIDs(plan)
	for _, id := range orderIDs {
		var exists bool
		err := tx.QueryRow(ctx, `SELECT TRUE FROM customer_order WHERE order_id = $1 FOR UPDATE`, id).Scan(&exists)
		if err != nil {
			return ConfirmationResult{}, fmt.Errorf("lock order %s: %w", id, err)
		}
	}

	// Reject an order already actively allocated for the same date. This is the
	// cross-row rule the schema cannot enforce with a cheap constraint; the lock
	// above makes the check race-free.
	if err := assertNotAlreadyAllocated(ctx, tx, plan); err != nil {
		return ConfirmationResult{}, err
	}

	var result ConfirmationResult
	for _, route := range plan.Routes {
		routeID, err := insertRoute(ctx, tx, route)
		if err != nil {
			return ConfirmationResult{}, err
		}
		result.RouteIDs = append(result.RouteIDs, routeID)
		for _, leg := range route.Legs {
			if err := insertLeg(ctx, tx, routeID, route.RouteDate, route.Location, leg); err != nil {
				return ConfirmationResult{}, err
			}
			if err := insertAllocation(ctx, tx, leg.OrderID, routeID, plan.Actor); err != nil {
				return ConfirmationResult{}, err
			}
			if err := setOrderAllocated(ctx, tx, leg.OrderID); err != nil {
				return ConfirmationResult{}, err
			}
			result.AllocatedOrders = append(result.AllocatedOrders, leg.OrderID)
		}
	}

	for _, d := range plan.Deferrals {
		if err := insertDeferral(ctx, tx, d, plan.Actor); err != nil {
			return ConfirmationResult{}, err
		}
		if err := insertDeferralAllocation(ctx, tx, d.OrderID, plan.Actor); err != nil {
			return ConfirmationResult{}, err
		}
		if err := setOrderDeferred(ctx, tx, d.OrderID); err != nil {
			return ConfirmationResult{}, err
		}
		result.DeferredOrders = append(result.DeferredOrders, d.OrderID)
	}

	if err := tx.Commit(ctx); err != nil {
		return ConfirmationResult{}, fmt.Errorf("commit confirmation: %w", err)
	}
	return result, nil
}

// collectOrderIDs returns every order id in the plan, deduplicated, in sorted
// order for deterministic locking.
func collectOrderIDs(plan ConfirmationPlan) []string {
	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, route := range plan.Routes {
		for _, leg := range route.Legs {
			add(leg.OrderID)
		}
	}
	for _, d := range plan.Deferrals {
		add(d.OrderID)
	}
	sortStrings(ids)
	return ids
}

func assertNotAlreadyAllocated(ctx context.Context, tx pgx.Tx, plan ConfirmationPlan) error {
	var ids []string
	for _, route := range plan.Routes {
		for _, leg := range route.Legs {
			ids = append(ids, leg.OrderID)
		}
	}
	for _, id := range ids {
		var n int
		err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM allocation a
			JOIN route rt ON rt.route_id = a.route_id
			WHERE a.order_id = $1
			  AND a.decision = 'ALLOCATED'
			  AND rt.route_date = $2::date`, id, plan.RouteDate).Scan(&n)
		if err != nil {
			return fmt.Errorf("check existing allocation for %s: %w", id, err)
		}
		if n > 0 {
			return fmt.Errorf("%w: order %s is already allocated on %s", ErrConflict, id, plan.RouteDate)
		}
	}
	return nil
}

func insertRoute(ctx context.Context, tx pgx.Tx, route Route) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO route (
			vehicle_id, depot_id, route_date, trip_no, brand, district, status,
			outbound_min, inter_stop_min, handling_min, total_trip_min,
			total_weight_kg, total_volume_m3, distance_km
		) VALUES ($1,$2,$3::date,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING route_id`,
		route.VehicleID, route.DepotID, route.RouteDate, route.TripNo, string(route.Brand),
		route.District, RouteConfirmed, route.OutboundMin, route.InterStopMin,
		route.HandlingMin, route.TotalTripMin, route.TotalWeightKg, route.TotalVolumeM3,
		route.DistanceKm).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("%w: create route %s/%d: %v", ErrConflict, route.VehicleID, route.TripNo, err)
	}
	return id, nil
}

func insertLeg(ctx context.Context, tx pgx.Tx, routeID, routeDate string, loc *time.Location, leg RouteLeg) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO route_leg (
			route_id, order_id, seq, from_point, to_outlet, distance_km,
			planned_arrival, service_time_min, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		routeID, leg.OrderID, leg.Seq, leg.FromPoint, leg.ToOutlet, leg.DistanceKm,
		plannedArrivalTS(routeDate, loc, leg), nullableServiceMinutes(leg), LegPending)
	if err != nil {
		return fmt.Errorf("create route leg (route %s seq %d): %w", routeID, leg.Seq, err)
	}
	return nil
}

// plannedArrivalTS anchors a leg's "HH:MM" planned arrival to the route date in
// the business timezone, returning nil when unset. The arrival is a wall-clock
// business time, so it must be stored as the instant that clock reads in the
// business zone (e.g. 05:52 Asia/Colombo), not as 05:52 UTC — otherwise a
// session-timezone read (to_char 'HH24:MI') and the driver's RFC3339 would both
// shift it. The date comes from the authoritative Route, never duplicated onto
// the leg.
func plannedArrivalTS(routeDate string, loc *time.Location, leg RouteLeg) any {
	if leg.PlannedArrival == "" {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}
	ts, err := time.ParseInLocation("2006-01-02T15:04", routeDate+"T"+leg.PlannedArrival, loc)
	if err != nil {
		// A malformed clock value is a programming error, not client input; drop
		// it rather than persist a wrong instant.
		return nil
	}
	return ts
}

// nullableServiceMinutes writes the handling allowance, NULL when unset so the
// column remains distinguishable from a genuine zero-minute service.
func nullableServiceMinutes(leg RouteLeg) any {
	if leg.ServiceTimeMin <= 0 {
		return nil
	}
	return leg.ServiceTimeMin
}

func insertAllocation(ctx context.Context, tx pgx.Tx, orderID, routeID, actor string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO allocation (order_id, route_id, decision, is_automatic, created_by)
		VALUES ($1, $2, 'ALLOCATED', FALSE, $3)`,
		orderID, routeID, nullable(actor))
	if err != nil {
		return fmt.Errorf("create allocation for %s: %w", orderID, err)
	}
	return nil
}

func insertDeferralAllocation(ctx context.Context, tx pgx.Tx, orderID, actor string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO allocation (order_id, route_id, decision, is_automatic, created_by)
		VALUES ($1, NULL, 'DEFERRED', FALSE, $2)`,
		orderID, nullable(actor))
	if err != nil {
		return fmt.Errorf("create deferral allocation for %s: %w", orderID, err)
	}
	return nil
}

func insertDeferral(ctx context.Context, tx pgx.Tx, d DeferredOrderForConfirm, actor string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO deferral_log (order_id, reason_type, reason, constraint_code, decided_by, deferred_to_date)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		d.OrderID, d.ReasonType, d.Reason, nullableConstraint(d.ConstraintCode), nullable(actor), nullableDate(d.DeferredToDate))
	if err != nil {
		return fmt.Errorf("create deferral for %s: %w", d.OrderID, err)
	}
	return nil
}

// setOrderAllocated transitions a served order to ALLOCATED. The order was
// validated CONFIRMED before confirmation; this is the state a confirmed
// allocation implies.
func setOrderAllocated(ctx context.Context, tx pgx.Tx, orderID string) error {
	_, err := tx.Exec(ctx, `UPDATE customer_order SET status = 'ALLOCATED', updated_at = now() WHERE order_id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("set order %s allocated: %w", orderID, err)
	}
	return nil
}

// setOrderDeferred transitions an unserved order to DEFERRED.
func setOrderDeferred(ctx context.Context, tx pgx.Tx, orderID string) error {
	_, err := tx.Exec(ctx, `UPDATE customer_order SET status = 'DEFERRED', updated_at = now() WHERE order_id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("set order %s deferred: %w", orderID, err)
	}
	return nil
}

// GetRoute implements Repository.
func (r *PGRepository) GetRoute(ctx context.Context, routeID string) (Route, error) {
	var rt Route
	err := r.pool.QueryRow(ctx, `
		SELECT route_id, vehicle_id, depot_id, route_date::text, trip_no, brand, district,
		       status, route_version, COALESCE(outbound_min,0), COALESCE(inter_stop_min,0),
		       COALESCE(handling_min,0), COALESCE(total_trip_min,0), COALESCE(total_weight_kg,0),
		       COALESCE(total_volume_m3,0), COALESCE(distance_km,0)
		FROM route WHERE route_id = $1`, routeID).
		Scan(&rt.RouteID, &rt.VehicleID, &rt.DepotID, &rt.RouteDate, &rt.TripNo, &rt.Brand,
			&rt.District, &rt.Status, &rt.RouteVersion, &rt.OutboundMin, &rt.InterStopMin,
			&rt.HandlingMin, &rt.TotalTripMin, &rt.TotalWeightKg, &rt.TotalVolumeM3, &rt.DistanceKm)
	if err != nil {
		return Route{}, mapNotFound(err, routeID)
	}
	legs, err := r.legsFor(ctx, routeID)
	if err != nil {
		return Route{}, err
	}
	rt.Legs = legs
	return rt, nil
}

// ListRoutes implements Repository.
func (r *PGRepository) ListRoutes(ctx context.Context, routeDate, depotID string) ([]Route, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if depotID != "" {
		rows, err = r.pool.Query(ctx, `
			SELECT route_id, vehicle_id, depot_id, route_date::text, trip_no, brand, district,
			       status, route_version, COALESCE(outbound_min,0), COALESCE(inter_stop_min,0),
			       COALESCE(handling_min,0), COALESCE(total_trip_min,0), COALESCE(total_weight_kg,0),
			       COALESCE(total_volume_m3,0), COALESCE(distance_km,0)
			FROM route WHERE route_date = $1::date AND depot_id = $2
			ORDER BY vehicle_id, trip_no`, routeDate, depotID)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT route_id, vehicle_id, depot_id, route_date::text, trip_no, brand, district,
			       status, route_version, COALESCE(outbound_min,0), COALESCE(inter_stop_min,0),
			       COALESCE(handling_min,0), COALESCE(total_trip_min,0), COALESCE(total_weight_kg,0),
			       COALESCE(total_volume_m3,0), COALESCE(distance_km,0)
			FROM route WHERE route_date = $1::date
			ORDER BY vehicle_id, trip_no`, routeDate)
	}
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	routes := make([]Route, 0)
	for rows.Next() {
		var rt Route
		if err := rows.Scan(&rt.RouteID, &rt.VehicleID, &rt.DepotID, &rt.RouteDate, &rt.TripNo,
			&rt.Brand, &rt.District, &rt.Status, &rt.RouteVersion, &rt.OutboundMin,
			&rt.InterStopMin, &rt.HandlingMin, &rt.TotalTripMin, &rt.TotalWeightKg,
			&rt.TotalVolumeM3, &rt.DistanceKm); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, rt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routes: %w", err)
	}
	for i := range routes {
		legs, err := r.legsFor(ctx, routes[i].RouteID)
		if err != nil {
			return nil, err
		}
		routes[i].Legs = legs
	}
	return routes, nil
}

func (r *PGRepository) legsFor(ctx context.Context, routeID string) ([]RouteLeg, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT leg_id, route_id, order_id, seq, from_point, to_outlet,
		       COALESCE(distance_km,0), status,
		       COALESCE(to_char(planned_arrival, 'HH24:MI'), ''),
		       COALESCE(service_time_min, 0)
		FROM route_leg WHERE route_id = $1 ORDER BY seq`, routeID)
	if err != nil {
		return nil, fmt.Errorf("load route legs: %w", err)
	}
	defer rows.Close()
	legs := make([]RouteLeg, 0)
	for rows.Next() {
		var l RouteLeg
		if err := rows.Scan(&l.LegID, &l.RouteID, &l.OrderID, &l.Seq, &l.FromPoint,
			&l.ToOutlet, &l.DistanceKm, &l.Status, &l.PlannedArrival, &l.ServiceTimeMin); err != nil {
			return nil, fmt.Errorf("scan route leg: %w", err)
		}
		legs = append(legs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route legs: %w", err)
	}
	return legs, nil
}

// ListDeferrals implements Repository.
func (r *PGRepository) ListDeferrals(ctx context.Context, outletID string) ([]DeferralEntry, error) {
	query := `
		SELECT d.deferral_id, d.order_id, o.order_number, o.outlet_id, d.reason_type, d.reason,
		       COALESCE(d.constraint_code, ''), COALESCE(d.decided_by, ''), d.decided_at,
		       COALESCE(d.deferred_to_date::text, '')
		FROM deferral_log d
		JOIN customer_order o ON o.order_id = d.order_id`
	args := []any{}
	if outletID != "" {
		query += ` WHERE o.outlet_id = $1`
		args = append(args, outletID)
	}
	query += ` ORDER BY d.decided_at DESC, d.deferral_id`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list deferrals: %w", err)
	}
	defer rows.Close()

	out := make([]DeferralEntry, 0)
	for rows.Next() {
		var e DeferralEntry
		if err := rows.Scan(&e.DeferralID, &e.OrderID, &e.OrderNumber, &e.OutletID, &e.ReasonType,
			&e.Reason, &e.ConstraintCode, &e.DecidedBy, &e.DecidedAt, &e.DeferredToDate); err != nil {
			return nil, fmt.Errorf("scan deferral: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deferrals: %w", err)
	}
	return out, nil
}

func mapNotFound(err error, ref string) error {
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: %s", ErrNotFound, ref)
	}
	return fmt.Errorf("load route: %w", err)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableConstraint(c domain.ConstraintCode) *string {
	if c == "" {
		return nil
	}
	s := string(c)
	return &s
}

func nullableDate(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
