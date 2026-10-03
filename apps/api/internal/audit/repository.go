package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository reads the audit trail and provides a transaction-bound Recorder.
type Repository interface {
	// List returns audit records matching filter, newest first.
	List(ctx context.Context, filter Filter) ([]Record, error)
	// WithTx runs fn with a Recorder bound to a transaction. The audit rows fn
	// writes commit with the transaction, or roll back with it.
	WithTx(ctx context.Context, fn func(ctx context.Context, rec Recorder) error) error
}

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

// txRecorder writes audit rows on a transaction.
type txRecorder struct{ tx pgx.Tx }

// Record implements Recorder. The audit row joins the caller's transaction.
func (r txRecorder) Record(ctx context.Context, e Event) error {
	return insertAudit(ctx, r.tx, e)
}

// WithTx implements Repository.
func (r *PGRepository) WithTx(ctx context.Context, fn func(ctx context.Context, rec Recorder) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, txRecorder{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit tx: %w", err)
	}
	return nil
}

// RecordTx writes one audit row on the given transaction. It is the seam the
// composition root adapts so a business repository can write an audit record in
// its own transaction without importing this package's types.
func RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error {
	return insertAudit(ctx, tx, Event{
		Action: Action(action), EntityType: EntityType(entityType), EntityID: entityID,
		Actor: actor, DepotID: depotID, OutletID: outletID, Result: Result(result), Detail: detail,
	})
}

// insertAudit writes one row on the given executor (pool or tx). before_json is
// left NULL: this foundation has no before-state diff. The structured envelope
// (scope, result, detail) is stored in after_json.
func insertAudit(ctx context.Context, q execer, e Event) error {
	envelope := map[string]any{
		"scope": map[string]any{
			"depotId":  e.DepotID,
			"outletId": e.OutletID,
		},
		"result": e.Result,
	}
	if e.Role != "" {
		envelope["role"] = string(e.Role)
	}
	if len(e.Detail) > 0 {
		envelope["detail"] = e.Detail
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal audit envelope: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO audit_log (actor, action, entity_type, entity_id, after_json, "timestamp")
		VALUES ($1,$2,$3,$4,$5, now())`,
		nullable(e.Actor), string(e.Action), string(e.EntityType), nullable(e.EntityID), payload)
	if err != nil {
		return fmt.Errorf("insert audit record: %w", err)
	}
	return nil
}

// execer is satisfied by both pgxpool.Pool and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// List implements Repository.
func (r *PGRepository) List(ctx context.Context, filter Filter) ([]Record, error) {
	var (
		where []string
		args  []any
	)
	if filter.Actor != "" {
		args = append(args, filter.Actor)
		where = append(where, fmt.Sprintf("actor = $%d", len(args)))
	}
	if filter.Action != "" {
		args = append(args, filter.Action)
		where = append(where, fmt.Sprintf("action = $%d", len(args)))
	}
	if filter.EntityType != "" {
		args = append(args, filter.EntityType)
		where = append(where, fmt.Sprintf("entity_type = $%d", len(args)))
	}
	if filter.EntityID != "" {
		args = append(args, filter.EntityID)
		where = append(where, fmt.Sprintf("entity_id = $%d", len(args)))
	}
	if filter.DepotID != "" {
		args = append(args, filter.DepotID)
		where = append(where, fmt.Sprintf("after_json #>> '{scope,depotId}' = $%d", len(args)))
	}
	if !filter.From.IsZero() {
		args = append(args, filter.From)
		where = append(where, fmt.Sprintf(`"timestamp" >= $%d`, len(args)))
	}
	if !filter.To.IsZero() {
		args = append(args, filter.To)
		where = append(where, fmt.Sprintf(`"timestamp" <= $%d`, len(args)))
	}

	query := `SELECT id, COALESCE(actor,''), action, entity_type, COALESCE(entity_id,''), after_json, "timestamp" FROM audit_log`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	// Defensive clamp: a zero/negative limit must never become `LIMIT 0`, which
	// silently returns no rows. The service clamps too; the repository does not
	// assume it was called through the service.
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	query += fmt.Sprintf(` ORDER BY "timestamp" DESC, id LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	out := make([]Record, 0)
	for rows.Next() {
		var (
			rec      Record
			envelope []byte
		)
		if err := rows.Scan(&rec.ID, &rec.Actor, &rec.Action, &rec.EntityType, &rec.EntityID, &envelope, &rec.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan audit: %w", err)
		}
		decodeEnvelope(&rec, envelope)
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit: %w", err)
	}
	return out, nil
}

// decodeEnvelope lifts scope/result/detail out of the stored after_json. A
// malformed envelope is ignored rather than failing the read: the core columns
// (action, actor, entity) are always present.
func decodeEnvelope(rec *Record, raw []byte) {
	var env struct {
		Scope struct {
			DepotID  string `json:"depotId"`
			OutletID string `json:"outletId"`
		} `json:"scope"`
		Result string         `json:"result"`
		Role   string         `json:"role"`
		Detail map[string]any `json:"detail"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	rec.DepotID = env.Scope.DepotID
	rec.OutletID = env.Scope.OutletID
	rec.Result = env.Result
	rec.Role = env.Role
	rec.Detail = env.Detail
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
