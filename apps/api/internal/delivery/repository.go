package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/domain"
)

// LegContext is the operational state a delivery event must be validated
// against: the leg's route, depot, target outlet and the orders on it.
type LegContext struct {
	LegID     string
	RouteID   string
	DepotID   string
	RouteDate string
	ToOutlet  string
	Status    string
	// OrderIDs on this leg (usually one; a leg is one stop, one order).
	OrderIDs []string
}

// Repository persists delivery events and loads the leg context they validate
// against.
type Repository interface {
	// LegContext returns the operational context for a leg, or ErrNotFound.
	LegContext(ctx context.Context, legID string) (LegContext, error)
	// Record applies one delivery event in a transaction. It returns the sync
	// result: ACCEPTED for a new event, DUPLICATE when the client_event_id was
	// already stored. Validation failures the caller should have caught are
	// returned as ErrInvalid/ErrConflict.
	Record(ctx context.Context, actor string, in EventInput, leg LegContext) (EventResult, error)
	// SyncStatus returns per-driver counts for the driver's routes.
	SyncStatus(ctx context.Context, actor string) (SyncStatus, error)
	// GetEvent returns a stored event by its client event id, or ErrNotFound.
	GetEvent(ctx context.Context, clientEventID string) (Event, error)
}

// SyncStatus reports how many of a driver's events are pending, accepted and
// failed. Pending is queued in the driver's outbox on the device and is not
// visible server-side, so it is reported as 0 here with the count of accepted
// and failed server events.
type SyncStatus struct {
	Synced    int
	Conflicts int
}

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
	// audit and notify are optional sinks written in the same transaction as the
	// delivery event, so a rolled-back mutation leaves no audit/notification.
	// They are narrow interfaces so this package does not import audit/notify.
	audit  AuditSink
	notify NotifySink
}

// AuditSink records one audit event inside the delivery transaction.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, e AuditEvent) error
}

// NotifySink raises a dispatcher notification inside the delivery transaction.
type NotifySink interface {
	NotifyDispatchersTx(ctx context.Context, tx pgx.Tx, depotID, outletID, notifType, title, message, reference string) error
}

// AuditEvent is the minimal audit fact delivery emits.
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
func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

// WithSinks attaches audit and notification sinks. They are optional: a nil sink
// disables that side-effect.
func (r *PGRepository) WithSinks(a AuditSink, n NotifySink) *PGRepository {
	r.audit = a
	r.notify = n
	return r
}

// LegContext implements Repository.
func (r *PGRepository) LegContext(ctx context.Context, legID string) (LegContext, error) {
	var lc LegContext
	err := r.pool.QueryRow(ctx, `
		SELECT leg.leg_id, leg.route_id, route.depot_id, route.route_date::text,
		       leg.to_outlet, leg.status
		FROM route_leg leg
		JOIN route ON route.route_id = leg.route_id
		WHERE leg.leg_id = $1`, legID).
		Scan(&lc.LegID, &lc.RouteID, &lc.DepotID, &lc.RouteDate, &lc.ToOutlet, &lc.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return LegContext{}, fmt.Errorf("%w: leg %s", ErrNotFound, legID)
	}
	if err != nil {
		return LegContext{}, fmt.Errorf("load leg context: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT order_id FROM route_leg WHERE leg_id = $1 ORDER BY order_id`, legID)
	if err != nil {
		return LegContext{}, fmt.Errorf("load leg orders: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var orderID string
		if err := rows.Scan(&orderID); err != nil {
			return LegContext{}, fmt.Errorf("scan leg order: %w", err)
		}
		lc.OrderIDs = append(lc.OrderIDs, orderID)
	}
	return lc, rows.Err()
}

// Record implements Repository.
//
// Idempotency is enforced by the unique index on delivery_event.client_event_id:
// a replayed event is detected with an INSERT ... ON CONFLICT DO NOTHING and
// reported as DUPLICATE, never double-applied. The leg row is locked so two
// concurrent events for the same leg serialise, and the whole write (event,
// item lines, leg status, order status) is one transaction.
func (r *PGRepository) Record(ctx context.Context, actor string, in EventInput, leg LegContext) (EventResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return EventResult{}, fmt.Errorf("begin delivery tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the leg: serialises concurrent events and lets us check current
	// status without a race.
	var legStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM route_leg WHERE leg_id = $1 FOR UPDATE`, leg.LegID).Scan(&legStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventResult{}, fmt.Errorf("%w: leg %s", ErrNotFound, leg.LegID)
		}
		return EventResult{}, fmt.Errorf("lock leg: %w", err)
	}

	occurredAt, err := time.Parse(time.RFC3339, in.OccurredAt)
	if err != nil {
		return EventResult{}, ValidationError{Field: "occurredAt", Message: "must be RFC 3339 with offset"}
	}

	// Idempotency: a known client_event_id is a duplicate, not an error.
	var existingEventID string
	err = tx.QueryRow(ctx, `SELECT event_id FROM delivery_event WHERE client_event_id = $1`, in.ClientEventID).Scan(&existingEventID)
	if err == nil {
		_ = tx.Rollback(ctx)
		return EventResult{ClientEventID: in.ClientEventID, Status: SyncDuplicate, ServerEventID: existingEventID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return EventResult{}, fmt.Errorf("check duplicate event: %w", err)
	}

	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO delivery_event (
			leg_id, recorded_by, outcome, pod_receiver_name, pod_signature, pod_photo,
			note, client_event_id, created_offline, client_created_at, synced_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, now())
		RETURNING event_id`,
		leg.LegID, nullable(actor), in.Outcome, nullable(in.Pod.ReceiverName),
		nullable(in.Pod.SignatureRef), nullable(in.Pod.PhotoRef), nullable(in.Notes),
		in.ClientEventID, in.CreatedOffline, occurredAt).Scan(&eventID)
	if err != nil {
		return EventResult{}, fmt.Errorf("insert delivery event: %w", err)
	}

	for _, it := range in.Items {
		if err := insertItemDelivery(ctx, tx, eventID, it, leg); err != nil {
			return EventResult{}, err
		}
	}

	// Leg status follows the outcome.
	if err := setLegStatus(ctx, tx, leg.LegID, in.Outcome); err != nil {
		return EventResult{}, err
	}
	// Order status follows the outcome for each order on the leg.
	orderStatus := string(OrderStatusForOutcome(in.Outcome))
	for _, orderID := range leg.OrderIDs {
		if _, err := tx.Exec(ctx, `UPDATE customer_order SET status = $2, updated_at = now() WHERE order_id = $1`, orderID, orderStatus); err != nil {
			return EventResult{}, fmt.Errorf("update order %s status: %w", orderID, err)
		}
	}

	// Audit and notification join this transaction: a rolled-back delivery leaves
	// neither. The reference derives from client_event_id, so a replayed event
	// (already returned as DUPLICATE above) never double-notifies.
	if err := r.emitSideEffects(ctx, tx, actor, in, leg, eventID, orderStatus); err != nil {
		return EventResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return EventResult{}, fmt.Errorf("commit delivery: %w", err)
	}
	return EventResult{ClientEventID: in.ClientEventID, Status: SyncAccepted, ServerEventID: eventID}, nil
}

// emitSideEffects writes the audit record and, for FAILED/DELAYED outcomes, a
// dispatcher notification, all on the delivery transaction.
func (r *PGRepository) emitSideEffects(ctx context.Context, tx pgx.Tx, actor string, in EventInput, leg LegContext, eventID, orderStatus string) error {
	if r.audit != nil {
		if err := r.audit.RecordTx(ctx, tx, AuditEvent{
			Action: "DELIVERY_RECORDED", EntityType: "DELIVERY_EVENT", EntityID: eventID,
			Actor: actor, DepotID: leg.DepotID, OutletID: leg.ToOutlet, Result: "SUCCESS",
			Detail: map[string]any{"outcome": in.Outcome, "clientEventId": in.ClientEventID, "orderStatus": orderStatus},
		}); err != nil {
			return fmt.Errorf("audit delivery event: %w", err)
		}
	}
	if r.notify == nil {
		return nil
	}
	var nType, title, message string
	switch in.Outcome {
	case OutcomeFailed:
		nType, title, message = "DELIVERY_FAILED", "Delivery failed",
			"A delivery at "+leg.ToOutlet+" failed and needs attention."
	case OutcomeDelayed:
		nType, title, message = "DELIVERY_DELAYED", "Delivery delayed",
			"A delivery at "+leg.ToOutlet+" was delayed."
	default:
		return nil
	}
	if err := r.notify.NotifyDispatchersTx(ctx, tx, leg.DepotID, leg.ToOutlet, nType, title, message, "delivery:"+in.ClientEventID); err != nil {
		return fmt.Errorf("notify dispatcher: %w", err)
	}
	return nil
}

// insertItemDelivery writes one order_item_delivery row after confirming the
// line belongs to an order on this leg and its quantities reconcile with the
// authoritative ordered quantity.
func insertItemDelivery(ctx context.Context, tx pgx.Tx, eventID string, it ItemDelivery, leg LegContext) error {
	var orderedQty int
	var onLeg bool
	err := tx.QueryRow(ctx, `
		SELECT oi.quantity, TRUE
		FROM route_leg rl
		JOIN order_item oi ON oi.order_id = rl.order_id
		WHERE rl.leg_id = $1 AND oi.order_item_id = $2
		LIMIT 1`, leg.LegID, it.OrderItemID).Scan(&orderedQty, &onLeg)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: order line %s is not on leg %s", ErrInvalid, it.OrderItemID, leg.LegID)
	}
	if err != nil {
		return fmt.Errorf("resolve order line %s: %w", it.OrderItemID, err)
	}
	if it.DeliveredQty+it.DamagedQty+it.ShortQty > orderedQty {
		return ValidationError{
			Field:   "deliveredItems.quantity",
			Message: fmt.Sprintf("delivered + damaged + short (%d) must not exceed the ordered quantity (%d)", it.DeliveredQty+it.DamagedQty+it.ShortQty, orderedQty),
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO order_item_delivery (delivery_event_id, order_item_id, delivered_qty, damaged_qty, short_qty)
		VALUES ($1,$2,$3,$4,$5)`,
		eventID, it.OrderItemID, it.DeliveredQty, it.DamagedQty, it.ShortQty)
	if err != nil {
		return fmt.Errorf("insert item delivery for %s: %w", it.OrderItemID, err)
	}
	return nil
}

// setLegStatus maps an outcome to the leg status vocabulary.
func setLegStatus(ctx context.Context, tx pgx.Tx, legID, outcome string) error {
	status := "DELIVERED"
	switch outcome {
	case OutcomeFailed:
		status = "FAILED"
	case OutcomeDelayed:
		status = "DELAYED"
	}
	if _, err := tx.Exec(ctx, `UPDATE route_leg SET status = $2, actual_arrival = COALESCE(actual_arrival, now()) WHERE leg_id = $1`, legID, status); err != nil {
		return fmt.Errorf("update leg %s status: %w", legID, err)
	}
	return nil
}

// GetEvent implements Repository.
func (r *PGRepository) GetEvent(ctx context.Context, clientEventID string) (Event, error) {
	var e Event
	var occurredAt time.Time
	var syncedAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT event_id, leg_id, outcome, client_event_id, client_created_at, synced_at,
		       created_offline, COALESCE(note, ''), COALESCE(pod_receiver_name, ''),
		       COALESCE(pod_signature, ''), COALESCE(pod_photo, '')
		FROM delivery_event WHERE client_event_id = $1`, clientEventID).
		Scan(&e.EventID, &e.LegID, &e.Outcome, &e.ClientEventID, &occurredAt, &syncedAt,
			&e.CreatedOffline, &e.Notes, &e.Pod.ReceiverName, &e.Pod.SignatureRef, &e.Pod.PhotoRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, fmt.Errorf("%w: event %s", ErrNotFound, clientEventID)
	}
	if err != nil {
		return Event{}, fmt.Errorf("load event: %w", err)
	}
	e.OccurredAt = occurredAt.Format(time.RFC3339)
	if syncedAt != nil {
		e.SyncedAt = syncedAt.Format(time.RFC3339)
	}
	if e.Pod.PhotoRef != "" {
		e.Pod.Type = PodPhoto
	} else if e.Pod.SignatureRef != "" {
		e.Pod.Type = PodSignature
	} else {
		e.Pod.Type = PodNone
	}
	return e, nil
}

// SyncStatus implements Repository.
func (r *PGRepository) SyncStatus(ctx context.Context, actor string) (SyncStatus, error) {
	var s SyncStatus
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM delivery_event WHERE recorded_by = $1`, actor).Scan(&s.Synced); err != nil {
		return SyncStatus{}, fmt.Errorf("count synced events: %w", err)
	}
	// Conflicts are events that arrived but could not be applied. A stored event
	// is always applied, so conflicts are tracked at apply time; there is no
	// separate conflict store yet, so this reports 0 honestly.
	_ = domain.RoleDriver
	return s, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
