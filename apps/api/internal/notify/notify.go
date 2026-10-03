// Package notify owns internal operational notifications: short-lived,
// role-addressed alerts that a dispatcher (or outlet) must act on. It is not a
// marketing or multi-channel system — there is one channel, in-app, persisted in
// the notification table.
//
// A notification is addressed to a recipient (a user id) and may carry an outlet
// scope. Creation is idempotent on (user_id, type, reference): the reference is
// derived from the originating business event, so a retried delivery/loading
// event cannot produce a duplicate notification.
package notify

import (
	"context"
	"errors"
	"strings"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Type is an operational notification category.
type Type string

const (
	// TypeShortfall is raised when a loader records a shortfall on a route.
	TypeShortfall Type = "SHORTFALL"
	// TypeDeliveryFailed is raised when a stop's outcome is FAILED.
	TypeDeliveryFailed Type = "DELIVERY_FAILED"
	// TypeDeliveryDelayed is raised when a stop's outcome is DELAYED.
	TypeDeliveryDelayed Type = "DELIVERY_DELAYED"
	// TypeRouteAttention is a generic route/operational alert for the dispatcher.
	TypeRouteAttention Type = "ROUTE_ATTENTION"
)

// Notification is one stored notification.
type Notification struct {
	ID        string
	UserID    string
	OutletID  string
	Type      string
	Title     string
	Message   string
	Reference string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// Read reports whether the notification has been read.
func (n Notification) Read() bool { return n.ReadAt != nil }

// New is a notification to create. Reference is the idempotency key derived from
// the originating event; UserID is a resolved recipient.
type New struct {
	UserID    string
	OutletID  string
	Type      Type
	Title     string
	Message   string
	Reference string
}

// ErrInvalid means the notification request failed validation.
var ErrInvalid = errors.New("notify: invalid input")

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// Creator creates notifications. It is the seam business repositories use inside
// their own transaction: a transaction-bound Creator writes the notification row
// in the same transaction as the mutation, so a rolled-back mutation leaves no
// notification.
type Creator interface {
	// Create writes a notification, idempotently on (user_id, type, reference).
	Create(ctx context.Context, n New) error
	// CreateForDispatchers resolves the active dispatchers of a depot and creates
	// the notification for each, idempotently. It is a no-op when no dispatcher
	// matches.
	CreateForDispatchers(ctx context.Context, depotID string, n New) error
}

// Repository reads and writes notifications.
type Repository interface {
	// Create inserts one notification idempotently on (user_id, type, reference).
	Create(ctx context.Context, n New) error
	// List returns a recipient's notifications, newest first.
	List(ctx context.Context, recipient Recipient, limit, offset int) ([]Notification, error)
	// UnreadCount returns a recipient's unread count.
	UnreadCount(ctx context.Context, recipient Recipient) (int, error)
	// MarkRead sets read_at for one notification owned by the recipient.
	// Idempotent. Returns ErrNotFound when the id is not the recipient's.
	MarkRead(ctx context.Context, recipient Recipient, notificationID string) error
	// MarkAllRead sets read_at for every unread notification of the recipient.
	MarkAllRead(ctx context.Context, recipient Recipient) (int, error)
	// DispatcherUserIDs returns the user ids of dispatchers for a depot.
	DispatcherUserIDs(ctx context.Context, depotID string) ([]string, error)
}

// Recipient is the authenticated user a notification read is scoped to. A
// notification is visible when it is addressed to the user id, or (for an
// outlet-scoped user) to their outlet.
type Recipient struct {
	UserID   string
	OutletID string
}

// Service is the notification surface for the API.
type Service struct {
	repo Repository
}

// NewService builds the notification service.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

const (
	maxLimit     = 200
	defaultLimit = 50
)

// List returns the recipient's notifications.
func (s *Service) List(ctx context.Context, r Recipient, limit, offset int) ([]Notification, error) {
	if r.UserID == "" {
		return nil, ValidationError{Field: "recipient", Message: "is required"}
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, r, limit, offset)
}

// UnreadCount returns the recipient's unread count.
func (s *Service) UnreadCount(ctx context.Context, r Recipient) (int, error) {
	if r.UserID == "" {
		return 0, ValidationError{Field: "recipient", Message: "is required"}
	}
	return s.repo.UnreadCount(ctx, r)
}

// MarkRead marks one notification read.
func (s *Service) MarkRead(ctx context.Context, r Recipient, id string) error {
	if strings.TrimSpace(id) == "" {
		return ValidationError{Field: "notificationId", Message: "is required"}
	}
	return s.repo.MarkRead(ctx, r, id)
}

// MarkAllRead marks all the recipient's notifications read.
func (s *Service) MarkAllRead(ctx context.Context, r Recipient) (int, error) {
	return s.repo.MarkAllRead(ctx, r)
}

// RecipientFrom builds a recipient from a user id and role/scope. Kept here so
// the handler does not reach into the auth package's shape.
func RecipientFrom(userID string, role domain.Role, outletID string) Recipient {
	return Recipient{UserID: userID, OutletID: outletID}
}
