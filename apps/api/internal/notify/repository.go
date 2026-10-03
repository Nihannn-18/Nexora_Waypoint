package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGRepository is the PostgreSQL-backed Repository. It also provides a
// transaction-bound Creator via WithTx.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

// execer is satisfied by both pgxpool.Pool and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WithTx runs fn with a Creator bound to a transaction, so notification writes
// commit with the business mutation (or roll back with it).
func (r *PGRepository) WithTx(ctx context.Context, fn func(ctx context.Context, c Creator) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notify tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, txCreator{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notify tx: %w", err)
	}
	return nil
}

// txCreator writes notifications on a transaction and resolves recipients there.
type txCreator struct{ tx pgx.Tx }

// Create implements Creator.
func (c txCreator) Create(ctx context.Context, n New) error { return insertNotification(ctx, c.tx, n) }

// CreateForDispatchers implements Creator.
func (c txCreator) CreateForDispatchers(ctx context.Context, depotID string, n New) error {
	ids, err := dispatcherIDs(ctx, c.tx, depotID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		row := n
		row.UserID = id
		if err := insertNotification(ctx, c.tx, row); err != nil {
			return err
		}
	}
	return nil
}

// insertNotification writes one row idempotently: the unique index on
// (user_id, type, reference) plus ON CONFLICT DO NOTHING makes a retried event a
// no-op.
func insertNotification(ctx context.Context, q execer, n New) error {
	if n.UserID == "" {
		return ValidationError{Field: "userId", Message: "is required"}
	}
	if strings.TrimSpace(string(n.Type)) == "" {
		return ValidationError{Field: "type", Message: "is required"}
	}
	if strings.TrimSpace(n.Title) == "" {
		return ValidationError{Field: "title", Message: "is required"}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO notification (user_id, outlet_id, type, title, message, reference)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (user_id, type, reference) DO NOTHING`,
		n.UserID, nullable(n.OutletID), string(n.Type), n.Title, n.Message, nullable(n.Reference))
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// dispatcherIDs returns the user ids of dispatchers for a depot. It does not
// invent an active flag: every matching dispatcher is a recipient.
func dispatcherIDs(ctx context.Context, q execer, depotID string) ([]string, error) {
	if depotID == "" {
		// No depot context: no safe recipient set, so no dispatcher fan-out.
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT user_id FROM app_user
		WHERE role = 'DISPATCHER' AND depot_id = $1
		ORDER BY user_id`, depotID)
	if err != nil {
		return nil, fmt.Errorf("resolve dispatchers: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dispatcher: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CreateForDispatchersTx is the seam the composition root adapts: it resolves
// the depot's dispatchers and creates the notification for each on the given
// transaction, idempotently.
func CreateForDispatchersTx(ctx context.Context, tx pgx.Tx, depotID, outletID, notifType, title, message, reference string) error {
	n := New{OutletID: outletID, Type: Type(notifType), Title: title, Message: message, Reference: reference}
	return txCreator{tx: tx}.CreateForDispatchers(ctx, depotID, n)
}

// Create implements Repository.
func (r *PGRepository) Create(ctx context.Context, n New) error {
	return insertNotification(ctx, r.pool, n)
}

// DispatcherUserIDs implements Repository.
func (r *PGRepository) DispatcherUserIDs(ctx context.Context, depotID string) ([]string, error) {
	return dispatcherIDs(ctx, r.pool, depotID)
}

// List implements Repository. Visibility is: addressed to the user, or (for an
// outlet-scoped recipient) addressed to their outlet.
func (r *PGRepository) List(ctx context.Context, recipient Recipient, limit, offset int) ([]Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, COALESCE(outlet_id,''), type, title, message,
		       COALESCE(reference,''), read_at, created_at
		FROM notification
		WHERE user_id = $1 OR ($2 <> '' AND outlet_id = $2)
		ORDER BY created_at DESC, id
		LIMIT $3 OFFSET $4`, recipient.UserID, recipient.OutletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	out := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.OutletID, &n.Type, &n.Title, &n.Message,
			&n.Reference, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadCount implements Repository.
func (r *PGRepository) UnreadCount(ctx context.Context, recipient Recipient) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notification
		WHERE read_at IS NULL AND (user_id = $1 OR ($2 <> '' AND outlet_id = $2))`,
		recipient.UserID, recipient.OutletID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}
	return n, nil
}

// MarkRead implements Repository.
func (r *PGRepository) MarkRead(ctx context.Context, recipient Recipient, notificationID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notification SET read_at = COALESCE(read_at, now())
		WHERE id = $1 AND (user_id = $2 OR ($3 <> '' AND outlet_id = $3))`,
		notificationID, recipient.UserID, recipient.OutletID)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead implements Repository.
func (r *PGRepository) MarkAllRead(ctx context.Context, recipient Recipient) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notification SET read_at = now()
		WHERE read_at IS NULL AND (user_id = $1 OR ($2 <> '' AND outlet_id = $2))`,
		recipient.UserID, recipient.OutletID)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ErrNotFound means no notification matches the recipient and id.
var ErrNotFound = errors.New("notify: not found")

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
