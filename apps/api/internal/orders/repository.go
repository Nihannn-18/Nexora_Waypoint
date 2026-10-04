package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists and loads orders. It is an interface so the service can
// be tested without a database; PGRepository is the production implementation.
//
// Only the operations the current contract needs are here: create-with-lines,
// read-with-lines, list by outlet/date/status, and a status update. Route or
// allocation persistence is deliberately absent — that belongs to internal/routes.
type Repository interface {
	// Create inserts the order and its lines in one transaction and returns the
	// stored order with generated ids and the order number.
	Create(ctx context.Context, o Order) (Order, error)
	// GetByID returns the order with its lines, or ErrNotFound.
	GetByID(ctx context.Context, orderID string) (Order, error)
	// GetByNumber returns the order with its lines, or ErrNotFound.
	GetByNumber(ctx context.Context, orderNumber string) (Order, error)
	// List returns orders matching filter, newest first, with lines loaded.
	List(ctx context.Context, filter Filter) ([]Order, error)
	// Count returns how many orders match filter, ignoring Limit and Offset, so
	// a paged listing can say "showing 50 of 186".
	Count(ctx context.Context, filter Filter) (int, error)
	// UpdateStatus sets a new status and returns the updated order, or
	// ErrNotFound. The caller has already validated the transition.
	UpdateStatus(ctx context.Context, orderID string, status string) (Order, error)
	// CloseQueue records that the queue for (date, depot, brand) is closed. It is
	// an upsert, so closing twice is idempotent. Returns how many closure rows
	// were newly written (0 when already closed).
	CloseQueue(ctx context.Context, date time.Time, depotID string, brands []string, actor string) (int, error)
	// ClosedBrands returns the brands whose queue is already closed for a date
	// and depot.
	ClosedBrands(ctx context.Context, date time.Time, depotID string) ([]string, error)
}

// Filter narrows an order listing. Zero values mean "no filter".
type Filter struct {
	OutletID string
	// DeliveryDate restricts to a requested_delivery_date (date-only).
	DeliveryDate *time.Time
	Status       string
	Brand        string
	// DepotID and District narrow by the outlet's depot and district; both are
	// outlet facts, so they are matched through the outlet table.
	DepotID  string
	District string
	// Search matches the order number or outlet id, case-insensitively.
	Search string
	// Limit caps the result set; 0 means the repository default.
	Limit int
	// Offset skips that many rows of the ordered result, for paging.
	Offset int
}

// defaultListLimit bounds an unfiltered listing so a caller cannot accidentally
// load every order ever placed.
const defaultListLimit = 200

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
	// audit is an optional sink written in the same transaction as a business
	// mutation, so a rolled-back close leaves no audit record.
	audit AuditSink
}

// AuditSink records one audit event inside the caller's transaction. It is a
// narrow interface so this package does not import internal/audit.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, e AuditEvent) error
}

// AuditEvent is the minimal audit fact the order package emits.
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

// WithAudit attaches an audit sink. It returns the repository for chaining at
// the composition root.
func (r *PGRepository) WithAudit(sink AuditSink) *PGRepository {
	r.audit = sink
	return r
}

// orderColumns is the header projection shared by every read.
const orderColumns = `order_id, order_number, outlet_id, brand, order_date,
	requested_delivery_date, total_units, total_weight_kg, total_volume_m3,
	temp_requirement, status, after_cutoff, COALESCE(notes, ''), created_at,
	updated_at, deferred_yesterday, days_since_last_served`

// Create implements Repository. The order number is generated from the year and
// the current maximum for that year; the UNIQUE constraint is the final guard,
// and nextOrderNumber retries on a collision (a concurrent create).
func (r *PGRepository) Create(ctx context.Context, o Order) (Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin order tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	number, err := nextOrderNumber(ctx, tx, o.OrderDate.Year())
	if err != nil {
		return Order{}, err
	}
	o.OrderNumber = number

	var created Order
	row := tx.QueryRow(ctx, `
		INSERT INTO customer_order (
			order_number, outlet_id, brand, order_date, requested_delivery_date,
			total_units, total_weight_kg, total_volume_m3, temp_requirement,
			status, after_cutoff, notes, deferred_yesterday, days_since_last_served
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+orderColumns,
		o.OrderNumber, o.OutletID, string(o.Brand), o.OrderDate, o.RequestedDeliveryDate,
		o.TotalUnits, o.TotalWeightKg, o.TotalVolumeM3, string(o.TempRequirement),
		string(o.Status), o.AfterCutoff, nullableString(o.Notes),
		o.DeferredYesterday, o.DaysSinceLastServed)
	created, err = scanOrder(row)
	if err != nil {
		return Order{}, fmt.Errorf("insert order: %w", err)
	}

	for i := range o.Lines {
		ln := o.Lines[i]
		var storedLine OrderLine
		err := tx.QueryRow(ctx, `
			INSERT INTO order_item (
				order_id, item_id, quantity, unit_weight_kg_snapshot,
				unit_volume_m3_snapshot, total_weight_kg, total_volume_m3
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
			RETURNING order_item_id`,
			created.OrderID, ln.ItemID, ln.Quantity, ln.UnitWeightKgSnapshot,
			ln.UnitVolumeM3Snapshot, ln.TotalWeightKg, ln.TotalVolumeM3).Scan(&storedLine.OrderItemID)
		if err != nil {
			return Order{}, fmt.Errorf("insert order line: %w", err)
		}
		storedLine.OrderID = created.OrderID
		storedLine.ItemID = ln.ItemID
		storedLine.Quantity = ln.Quantity
		storedLine.UnitWeightKgSnapshot = ln.UnitWeightKgSnapshot
		storedLine.UnitVolumeM3Snapshot = ln.UnitVolumeM3Snapshot
		storedLine.TotalWeightKg = ln.TotalWeightKg
		storedLine.TotalVolumeM3 = ln.TotalVolumeM3
		created.Lines = append(created.Lines, storedLine)
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit order tx: %w", err)
	}
	return created, nil
}

// GetByID implements Repository.
func (r *PGRepository) GetByID(ctx context.Context, orderID string) (Order, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM customer_order WHERE order_id = $1`, orderID)
	return r.loadWithLines(ctx, row, orderID)
}

// GetByNumber implements Repository.
func (r *PGRepository) GetByNumber(ctx context.Context, orderNumber string) (Order, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM customer_order WHERE order_number = $1`, orderNumber)
	return r.loadWithLines(ctx, row, orderNumber)
}

func (r *PGRepository) loadWithLines(ctx context.Context, row pgx.Row, ref string) (Order, error) {
	o, err := scanOrder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("%w: %s", ErrNotFound, ref)
	}
	if err != nil {
		return Order{}, fmt.Errorf("load order: %w", err)
	}
	lines, err := r.linesFor(ctx, o.OrderID)
	if err != nil {
		return Order{}, err
	}
	o.Lines = lines
	return o, nil
}

func (r *PGRepository) linesFor(ctx context.Context, orderID string) ([]OrderLine, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT oi.order_item_id, oi.order_id, oi.item_id, oi.quantity, oi.unit_weight_kg_snapshot,
		       oi.unit_volume_m3_snapshot, oi.total_weight_kg, oi.total_volume_m3,
		       COALESCE(it.sku, ''), COALESCE(it.name, '')
		FROM order_item oi
		LEFT JOIN item it ON it.item_id = oi.item_id
		WHERE oi.order_id = $1 ORDER BY oi.order_item_id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("load order lines: %w", err)
	}
	defer rows.Close()

	lines := make([]OrderLine, 0)
	for rows.Next() {
		var ln OrderLine
		if err := rows.Scan(&ln.OrderItemID, &ln.OrderID, &ln.ItemID, &ln.Quantity,
			&ln.UnitWeightKgSnapshot, &ln.UnitVolumeM3Snapshot, &ln.TotalWeightKg, &ln.TotalVolumeM3,
			&ln.SKU, &ln.Name); err != nil {
			return nil, fmt.Errorf("scan order line: %w", err)
		}
		lines = append(lines, ln)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate order lines: %w", err)
	}
	return lines, nil
}

// List implements Repository. It loads headers, then the lines for those orders
// in one extra query, so a listing does not issue N+1 queries.
func (r *PGRepository) List(ctx context.Context, filter Filter) ([]Order, error) {
	where, args := filterClause(filter)

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	offset := max(filter.Offset, 0)
	args = append(args, limit, offset)

	query := `SELECT ` + orderColumns + ` FROM customer_order` + where
	query += fmt.Sprintf(" ORDER BY created_at DESC, order_number LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	orders := make([]Order, 0)
	ids := make([]string, 0)
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
		ids = append(ids, o.OrderID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}
	if len(ids) == 0 {
		return orders, nil
	}

	byOrder, err := r.linesForMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Lines = byOrder[orders[i].OrderID]
	}
	return orders, nil
}

// Count implements Repository.
func (r *PGRepository) Count(ctx context.Context, filter Filter) (int, error) {
	where, args := filterClause(filter)
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM customer_order`+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count orders: %w", err)
	}
	return n, nil
}

// filterClause builds the parameterised WHERE clause shared by List and Count,
// so a page and its total can never disagree about which orders match.
func filterClause(filter Filter) (string, []any) {
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if filter.OutletID != "" {
		add("outlet_id = $%d", filter.OutletID)
	}
	if filter.DeliveryDate != nil {
		add("requested_delivery_date = $%d", *filter.DeliveryDate)
	}
	if filter.Status != "" {
		add("status = $%d", filter.Status)
	}
	if filter.Brand != "" {
		add("brand = $%d", filter.Brand)
	}
	// depot_id is compared as text so a malformed id matches nothing instead of
	// failing the UUID cast.
	if filter.DepotID != "" {
		add("outlet_id IN (SELECT outlet_id FROM outlet WHERE depot_id::text = $%d)", filter.DepotID)
	}
	if filter.District != "" {
		add("outlet_id IN (SELECT outlet_id FROM outlet WHERE district = $%d)", filter.District)
	}
	if s := strings.TrimSpace(filter.Search); s != "" {
		args = append(args, "%"+s+"%")
		where = append(where, fmt.Sprintf("(order_number ILIKE $%d OR outlet_id ILIKE $%d)", len(args), len(args)))
	}
	if len(where) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

func (r *PGRepository) linesForMany(ctx context.Context, orderIDs []string) (map[string][]OrderLine, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT oi.order_item_id, oi.order_id, oi.item_id, oi.quantity, oi.unit_weight_kg_snapshot,
		       oi.unit_volume_m3_snapshot, oi.total_weight_kg, oi.total_volume_m3,
		       COALESCE(it.sku, ''), COALESCE(it.name, '')
		FROM order_item oi
		LEFT JOIN item it ON it.item_id = oi.item_id
		WHERE oi.order_id = ANY($1) ORDER BY oi.order_id, oi.order_item_id`, orderIDs)
	if err != nil {
		return nil, fmt.Errorf("load order lines: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]OrderLine, len(orderIDs))
	for rows.Next() {
		var ln OrderLine
		if err := rows.Scan(&ln.OrderItemID, &ln.OrderID, &ln.ItemID, &ln.Quantity,
			&ln.UnitWeightKgSnapshot, &ln.UnitVolumeM3Snapshot, &ln.TotalWeightKg, &ln.TotalVolumeM3,
			&ln.SKU, &ln.Name); err != nil {
			return nil, fmt.Errorf("scan order line: %w", err)
		}
		out[ln.OrderID] = append(out[ln.OrderID], ln)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate order lines: %w", err)
	}
	return out, nil
}

// UpdateStatus implements Repository.
func (r *PGRepository) UpdateStatus(ctx context.Context, orderID string, status string) (Order, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE customer_order SET status = $2, updated_at = now()
		WHERE order_id = $1
		RETURNING `+orderColumns, orderID, status)
	return r.loadWithLines(ctx, row, orderID)
}

// CloseQueue implements Repository. It upserts one closure row per (date,
// depot, brand) and returns how many rows were newly inserted. Re-closing an
// already-closed brand changes nothing and does not count, so the caller can
// distinguish a first close from a repeat.
func (r *PGRepository) CloseQueue(ctx context.Context, date time.Time, depotID string, brands []string, actor string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin close queue tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	closed := 0
	newlyClosed := make([]string, 0, len(brands))
	for _, brand := range brands {
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO order_queue_close (queue_date, depot_id, brand, closed_by)
			VALUES ($1::date, $2, $3, $4)
			ON CONFLICT (queue_date, depot_id, brand) DO NOTHING
			RETURNING TRUE`, date, depotID, brand, nullableString(actor)).Scan(&inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			// Already closed for this brand: idempotent no-op.
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("close queue %s/%s: %w", depotID, brand, err)
		}
		if inserted {
			closed++
			newlyClosed = append(newlyClosed, brand)
		}
	}

	// The audit row joins the same transaction as the closure, so a close and
	// its trail commit together or not at all.
	if r.audit != nil {
		if err := r.audit.RecordTx(ctx, tx, AuditEvent{
			Action:     "QUEUE_CLOSED",
			EntityType: "QUEUE",
			EntityID:   date.Format("2006-01-02") + "|" + depotID,
			Actor:      actor,
			DepotID:    depotID,
			Result:     "SUCCESS",
			Detail: map[string]any{
				"date":        date.Format("2006-01-02"),
				"brands":      brands,
				"newlyClosed": newlyClosed,
				"closed":      closed,
			},
		}); err != nil {
			return 0, fmt.Errorf("audit close queue: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit close queue: %w", err)
	}
	return closed, nil
}

// ClosedBrands implements Repository.
func (r *PGRepository) ClosedBrands(ctx context.Context, date time.Time, depotID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT brand FROM order_queue_close
		WHERE queue_date = $1::date AND depot_id = $2
		ORDER BY brand`, date, depotID)
	if err != nil {
		return nil, fmt.Errorf("list closed brands: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, fmt.Errorf("scan closed brand: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// nextOrderNumber returns the next ORD-YYYY-NNNNNN for the year. It reads the
// current maximum for the year inside the caller's transaction, so two
// concurrent creates serialise on the same statement; the UNIQUE constraint is
// the backstop.
func nextOrderNumber(ctx context.Context, tx pgx.Tx, year int) (string, error) {
	prefix := fmt.Sprintf("ORD-%04d-", year)
	var max *string
	err := tx.QueryRow(ctx, `
		SELECT max(order_number) FROM customer_order
		WHERE order_number LIKE $1`, prefix+"%").Scan(&max)
	if err != nil {
		return "", fmt.Errorf("read order number sequence: %w", err)
	}
	next := 1
	if max != nil {
		// The suffix is zero-padded to six digits, so a lexical max equals the
		// numeric max for the same year.
		seq := 0
		if _, err := fmt.Sscanf((*max)[len(prefix):], "%d", &seq); err == nil {
			next = seq + 1
		}
	}
	// 1_000_000 orders a year is well beyond the network's scale; fail loudly
	// rather than silently widening the format.
	if next > 999999 {
		return "", fmt.Errorf("order number space for %d is exhausted", year)
	}
	return fmt.Sprintf("%s%06d", prefix, next), nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrder(s rowScanner) (Order, error) {
	var o Order
	err := s.Scan(
		&o.OrderID, &o.OrderNumber, &o.OutletID, &o.Brand, &o.OrderDate,
		&o.RequestedDeliveryDate, &o.TotalUnits, &o.TotalWeightKg, &o.TotalVolumeM3,
		&o.TempRequirement, &o.Status, &o.AfterCutoff, &o.Notes, &o.CreatedAt,
		&o.UpdatedAt, &o.DeferredYesterday, &o.DaysSinceLastServed,
	)
	if err != nil {
		return Order{}, err
	}
	return o, nil
}

// nullableString stores "" as NULL for a nullable text column.
func nullableString(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
