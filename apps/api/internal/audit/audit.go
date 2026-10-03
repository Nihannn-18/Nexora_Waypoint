// Package audit owns the append-only operational audit trail: who did what,
// when, on which entity, in which scope, with what result.
//
// audit_log is the authoritative store, mirroring the schema. It is
// append-only: this package only inserts and reads, never updates or deletes.
// An audit record is a business/security fact, not a log line — it is written
// with the mutation it describes.
//
// Sensitivity: audit records never carry passwords, tokens, session secrets or
// raw request bodies. The structured detail is a small envelope of
// operationally useful facts.
package audit

import (
	"context"
	"errors"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Action is the business/security action an audit record describes. Values are
// UPPER_SNAKE, matching the wire convention. They identify meaningful
// operations, not every endpoint.
type Action string

const (
	// Orders
	ActionOrderCreated   Action = "ORDER_CREATED"
	ActionOrderConfirmed Action = "ORDER_CONFIRMED"
	// Planning
	ActionPlanningProposed Action = "PLANNING_PROPOSED"
	// Routes / allocation
	ActionRouteConfirmed    Action = "ROUTE_CONFIRMED"
	ActionAllocationDecided Action = "ALLOCATION_DECIDED"
	// Loading
	ActionLoadRecorded      Action = "LOAD_RECORDED"
	ActionShortfallRecorded Action = "SHORTFALL_RECORDED"
	// Delivery
	ActionDeliveryRecorded Action = "DELIVERY_RECORDED"
	// Sync
	ActionSyncProcessed Action = "SYNC_PROCESSED"
)

// EntityType is the business entity an action concerns.
type EntityType string

const (
	EntityOrder       EntityType = "ORDER"
	EntityPlanningJob EntityType = "PLANNING_JOB"
	EntityRoute       EntityType = "ROUTE"
	EntityAllocation  EntityType = "ALLOCATION"
	EntityLoadItem    EntityType = "LOAD_ITEM"
	EntityDelivery    EntityType = "DELIVERY_EVENT"
	EntitySync        EntityType = "SYNC_BATCH"
)

// Result is the outcome an audit record reports.
type Result string

const (
	ResultSuccess Result = "SUCCESS"
	ResultFailure Result = "FAILURE"
	ResultDenied  Result = "DENIED"
)

// Event is one audit record to write. Actor is the acting user id (empty for a
// system action); Role is the actor's role at the time; Scope carries the
// depot/outlet the action applied to. Detail is a small, non-sensitive map of
// operational facts (ids, counts, codes) — never credentials or bodies.
type Event struct {
	Action     Action
	EntityType EntityType
	EntityID   string
	Actor      string
	Role       domain.Role
	// DepotID and OutletID are the scope the action applied to, empty when not
	// applicable.
	DepotID  string
	OutletID string
	Result   Result
	// Detail is a small envelope of operationally useful facts. It is stored in
	// audit_log.after_json. Keep it minimal and free of secrets.
	Detail map[string]any
}

// Record is a stored audit record, returned to a query.
type Record struct {
	ID         string
	Actor      string
	Role       string
	Action     string
	EntityType string
	EntityID   string
	DepotID    string
	OutletID   string
	Result     string
	Detail     map[string]any
	OccurredAt time.Time
}

// Filter narrows an audit query. Zero values mean "no filter". DepotID is the
// scope a dispatcher may narrow to; the service applies the caller's own scope
// on top.
type Filter struct {
	Actor      string
	Action     string
	EntityType string
	EntityID   string
	DepotID    string
	// From and To bound occurred_at (inclusive). Zero means unbounded.
	From time.Time
	To   time.Time
	// Limit is the page size (clamped by the service); Offset is the page start.
	Limit  int
	Offset int
}

// ErrInvalid means the audit request failed validation.
var ErrInvalid = errors.New("audit: invalid input")

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// Recorder writes audit events. It is the seam business repositories use inside
// their own transaction: a DBRecorder bound to a pgx.Tx writes the audit row in
// the same transaction as the mutation, so a rolled-back mutation leaves no
// audit record.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// TxFunc runs fn against a recorder bound to a transaction. Implementations
// supply a transaction-bound Recorder. Business packages accept a Recorder and
// a "begin transaction" hook as small interfaces, so the audit write is atomic
// with the mutation without this package importing pgx into their signatures.
type TxAuditor interface {
	// WithTx begins a transaction, calls fn with a transaction-bound Recorder,
	// and commits unless fn returns an error.
	WithTx(ctx context.Context, fn func(ctx context.Context, rec Recorder) error) error
}

// Service is the audit query surface. It validates filters and applies the
// caller's scope; writes go through a Recorder.
type Service struct {
	repo Repository
}

// NewService builds the audit service over a repository.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// maxLimit bounds a query page so a caller cannot request the whole trail.
const maxLimit = 200

// defaultLimit is the page size when the caller asks for none.
const defaultLimit = 50

// List returns audit records matching filter, scoped to the caller.
//
// Scope rule: a dispatcher may see both depots and may narrow with DepotID; any
// other role is refused. This matches the API contract that audit is
// dispatcher-only.
func (s *Service) List(ctx context.Context, id Identity, filter Filter) ([]Record, error) {
	if id.Role != domain.RoleDispatcher {
		return nil, ValidationError{Field: "role", Message: "audit is dispatcher-only"}
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultLimit
	}
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && filter.To.Before(filter.From) {
		return nil, ValidationError{Field: "to", Message: "must not be before from"}
	}
	// A non-dispatcher never reaches here; a dispatcher sees both depots, so the
	// filter's DepotID is the only narrowing and is applied by the repository.
	return s.repo.List(ctx, filter)
}

// Identity is the minimal caller view the service needs, kept local so audit
// does not import the auth package (matching how other domains pass a narrow
// scope).
type Identity struct {
	UserID string
	Role   domain.Role
}
