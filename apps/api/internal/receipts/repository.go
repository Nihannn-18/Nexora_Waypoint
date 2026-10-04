package receipts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditSink records one audit event inside the GRN transaction.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, e AuditEvent) error
}

// NotifySink raises a dispatcher notification inside the GRN transaction.
type NotifySink interface {
	NotifyDispatchersTx(ctx context.Context, tx pgx.Tx, depotID, outletID, notifType, title, message, reference string) error
}

// AuditEvent is the minimal audit fact a GRN emits.
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

// NotificationType is the dispatcher notification raised when a GRN reports
// an issue. Mirrors NOTIFICATION_TYPES in libs/shared-types.
const NotificationType = "RECEIPT_ISSUE"

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool   *pgxpool.Pool
	audit  AuditSink
	notify NotifySink
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

// WithSinks attaches audit and notification sinks. A nil sink disables that
// side-effect.
func (r *PGRepository) WithSinks(a AuditSink, n NotifySink) *PGRepository {
	r.audit = a
	r.notify = n
	return r
}

// OrderContext implements Repository. The order id is compared as text so a
// malformed id reads as not found instead of failing the UUID cast.
func (r *PGRepository) OrderContext(ctx context.Context, orderID string) (OrderContext, error) {
	var o OrderContext
	err := r.pool.QueryRow(ctx, `
		SELECT o.order_id, o.order_number, o.outlet_id, COALESCE(outlet.depot_id::text, ''), o.status
		FROM customer_order o
		JOIN outlet ON outlet.outlet_id = o.outlet_id
		WHERE o.order_id::text = $1`, orderID).
		Scan(&o.OrderID, &o.OrderNumber, &o.OutletID, &o.DepotID, &o.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderContext{}, fmt.Errorf("%w: order %s", ErrNotFound, orderID)
	}
	if err != nil {
		return OrderContext{}, fmt.Errorf("load order: %w", err)
	}

	// One load_item per line per route (migration 00004). An order rides one
	// route, so at most one count exists per line; the latest is taken if a
	// re-plan ever left two.
	rows, err := r.pool.Query(ctx, `
		SELECT oi.order_item_id, COALESCE(it.sku, ''), COALESCE(it.name, ''), oi.quantity,
		       li.loaded_qty, COALESCE(li.missing_qty, 0), COALESCE(li.damaged_qty, 0),
		       COALESCE(li.photo_ref, '')
		FROM order_item oi
		LEFT JOIN item it ON it.item_id = oi.item_id
		LEFT JOIN LATERAL (
			SELECT loaded_qty, missing_qty, damaged_qty, photo_ref
			FROM load_item
			WHERE load_item.order_item_id = oi.order_item_id
			ORDER BY recorded_at DESC NULLS LAST
			LIMIT 1
		) li ON TRUE
		WHERE oi.order_id = $1
		ORDER BY it.sku, oi.order_item_id`, o.OrderID)
	if err != nil {
		return OrderContext{}, fmt.Errorf("load order lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l ExpectedLine
		var loaded *int
		var flag LoaderFlag
		if err := rows.Scan(&l.OrderItemID, &l.SKU, &l.Name, &l.OrderedQty,
			&loaded, &flag.MissingQty, &flag.DamagedQty, &flag.PhotoRef); err != nil {
			return OrderContext{}, fmt.Errorf("scan order line: %w", err)
		}
		l.LoadedQty = loaded
		if flag.MissingQty > 0 || flag.DamagedQty > 0 {
			l.LoaderFlag = &flag
		}
		o.Lines = append(o.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return OrderContext{}, fmt.Errorf("iterate order lines: %w", err)
	}
	return o, nil
}

// Pod implements Repository: the latest DELIVERED or DELAYED event on the
// order's leg. A FAILED event carries no goods, so it is never a GRN's proof.
func (r *PGRepository) Pod(ctx context.Context, orderID string) (*Pod, error) {
	var p Pod
	err := r.pool.QueryRow(ctx, `
		SELECT e.outcome, COALESCE(e.pod_receiver_name, ''), COALESCE(e.pod_signature, ''),
		       COALESCE(e.pod_photo, ''), e.client_created_at
		FROM delivery_event e
		JOIN route_leg leg ON leg.leg_id = e.leg_id
		WHERE leg.order_id = $1 AND e.outcome IN ('DELIVERED', 'DELAYED')
		ORDER BY e.client_created_at DESC
		LIMIT 1`, orderID).
		Scan(&p.Outcome, &p.ReceiverName, &p.SignatureRef, &p.PhotoRef, &p.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load proof of delivery: %w", err)
	}
	return &p, nil
}

// Receipt implements Repository.
func (r *PGRepository) Receipt(ctx context.Context, orderID string) (*Receipt, error) {
	var rec Receipt
	err := r.pool.QueryRow(ctx, `
		SELECT r.receipt_id, r.order_id, r.received_at, COALESCE(r.received_by, ''),
		       COALESCE(u.display_name, ''), COALESCE(r.notes, '')
		FROM receipt r
		LEFT JOIN app_user u ON u.user_id = r.received_by
		WHERE r.order_id = $1`, orderID).
		Scan(&rec.ReceiptID, &rec.OrderID, &rec.ReceivedAt, &rec.ReceivedBy, &rec.ReceivedByName, &rec.Notes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load receipt: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT rl.order_item_id, COALESCE(it.sku, ''), COALESCE(it.name, ''), oi.quantity,
		       rl.expected_qty, rl.received_qty, rl.damaged_qty
		FROM receipt_line rl
		JOIN order_item oi ON oi.order_item_id = rl.order_item_id
		LEFT JOIN item it ON it.item_id = oi.item_id
		WHERE rl.receipt_id = $1
		ORDER BY it.sku, rl.order_item_id`, rec.ReceiptID)
	if err != nil {
		return nil, fmt.Errorf("load receipt lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.OrderItemID, &l.SKU, &l.Name, &l.OrderedQty,
			&l.ExpectedQty, &l.ReceivedQty, &l.DamagedQty); err != nil {
			return nil, fmt.Errorf("scan receipt line: %w", err)
		}
		rec.Lines = append(rec.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate receipt lines: %w", err)
	}
	return &rec, nil
}

// Create implements Repository.
//
// The order row is locked first, so two submissions for one order serialise:
// the second sees RECEIVED and is refused. The unique index on
// receipt.order_id is the backstop if anything bypasses the lock.
func (r *PGRepository) Create(ctx context.Context, order OrderContext, rec Receipt) (Receipt, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Receipt{}, fmt.Errorf("begin receipt tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM customer_order WHERE order_id = $1 FOR UPDATE`, order.OrderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Receipt{}, fmt.Errorf("%w: order %s", ErrNotFound, order.OrderID)
		}
		return Receipt{}, fmt.Errorf("lock order: %w", err)
	}
	switch status {
	case "DELIVERED":
	case "RECEIVED":
		return Receipt{}, ErrAlreadyReceived
	default:
		return Receipt{}, ErrNotDelivered
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO receipt (order_id, status, received_at, received_by, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING receipt_id`,
		order.OrderID, rec.Status(), rec.ReceivedAt, nullable(rec.ReceivedBy), nullable(rec.Notes)).Scan(&rec.ReceiptID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Receipt{}, ErrAlreadyReceived
		}
		return Receipt{}, fmt.Errorf("insert receipt: %w", err)
	}
	for _, l := range rec.Lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO receipt_line (receipt_id, order_item_id, expected_qty, received_qty, damaged_qty)
			VALUES ($1, $2, $3, $4, $5)`,
			rec.ReceiptID, l.OrderItemID, l.ExpectedQty, l.ReceivedQty, l.DamagedQty); err != nil {
			return Receipt{}, fmt.Errorf("insert receipt line %s: %w", l.OrderItemID, err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE customer_order SET status = 'RECEIVED', updated_at = $2 WHERE order_id = $1`,
		order.OrderID, rec.ReceivedAt); err != nil {
		return Receipt{}, fmt.Errorf("mark order received: %w", err)
	}
	if err := r.emitSideEffects(ctx, tx, order, rec); err != nil {
		return Receipt{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(display_name, '') FROM app_user WHERE user_id = $1`, rec.ReceivedBy).
		Scan(&rec.ReceivedByName); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, fmt.Errorf("load receiver name: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Receipt{}, fmt.Errorf("commit receipt: %w", err)
	}
	return rec, nil
}

// emitSideEffects writes the audit row and, when the GRN raised an issue, a
// dispatcher notification, both on the receipt transaction.
func (r *PGRepository) emitSideEffects(ctx context.Context, tx pgx.Tx, order OrderContext, rec Receipt) error {
	damaged, short := 0, 0
	for _, l := range rec.Lines {
		damaged += l.DamagedQty
		short += l.ShortQty()
	}
	if r.audit != nil {
		if err := r.audit.RecordTx(ctx, tx, AuditEvent{
			Action: "RECEIPT_RECORDED", EntityType: "RECEIPT", EntityID: rec.ReceiptID,
			Actor: rec.ReceivedBy, DepotID: order.DepotID, OutletID: order.OutletID, Result: "SUCCESS",
			Detail: map[string]any{
				"orderId": order.OrderID, "orderNumber": order.OrderNumber, "status": rec.Status(),
				"damagedQty": damaged, "shortQty": short,
			},
		}); err != nil {
			return fmt.Errorf("audit receipt: %w", err)
		}
	}
	if r.notify == nil || (damaged == 0 && short == 0) {
		return nil
	}
	message := fmt.Sprintf("%s received %s with %d damaged and %d short.", order.OutletID, order.OrderNumber, damaged, short)
	if err := r.notify.NotifyDispatchersTx(ctx, tx, order.DepotID, order.OutletID, NotificationType,
		"Store reported a receipt issue", message, "receipt:"+order.OrderID); err != nil {
		return fmt.Errorf("notify dispatcher: %w", err)
	}
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Compile-time proof the repository satisfies the service's port.
var _ Repository = (*PGRepository)(nil)
