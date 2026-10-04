package receipts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Clock reports the current business-time instant; a GRN's received time is
// the API clock (the demo clock under DEMO_MODE), never the device's.
type Clock interface{ Now() time.Time }

// Repository loads the GRN's inputs and records it.
type Repository interface {
	// OrderContext returns the order with its expected lines, or ErrNotFound.
	OrderContext(ctx context.Context, orderID string) (OrderContext, error)
	// Pod returns the driver's latest delivered/delayed POD for the order, or
	// nil when none is recorded.
	Pod(ctx context.Context, orderID string) (*Pod, error)
	// Receipt returns the order's GRN, or nil when none is recorded.
	Receipt(ctx context.Context, orderID string) (*Receipt, error)
	// Create records the GRN, moves the order DELIVERED → RECEIVED and writes
	// its audit and notification in one transaction. It re-checks the order's
	// status under a row lock, returning ErrNotDelivered or ErrAlreadyReceived.
	Create(ctx context.Context, order OrderContext, rec Receipt) (Receipt, error)
}

// Scope is the caller's role and outlet, from the authenticated identity.
type Scope struct {
	Role     domain.Role
	OutletID string
}

// Service is the GRN business surface.
type Service struct {
	repo  Repository
	clock Clock
}

// NewService builds the receipts service.
func NewService(repo Repository, clock Clock) *Service { return &Service{repo: repo, clock: clock} }

// View returns the S-06 read model for an order: expected lines with the
// loader's flags, the driver's POD and the GRN if one exists.
func (s *Service) View(ctx context.Context, orderID string, scope Scope) (View, error) {
	order, err := s.scopedOrder(ctx, orderID, scope)
	if err != nil {
		return View{}, err
	}
	pod, err := s.repo.Pod(ctx, order.OrderID)
	if err != nil {
		return View{}, err
	}
	rec, err := s.repo.Receipt(ctx, order.OrderID)
	if err != nil {
		return View{}, err
	}
	return View{Order: order, Pod: pod, Receipt: rec}, nil
}

// Submit records the store's GRN for a delivered order.
func (s *Service) Submit(ctx context.Context, orderID, actor string, scope Scope, in Input) (Receipt, error) {
	if scope.Role != domain.RoleStoreManager {
		// The handler already refuses other roles; this keeps the rule true for
		// any other caller of the service.
		return Receipt{}, fmt.Errorf("%w: order %s", ErrNotFound, orderID)
	}
	order, err := s.scopedOrder(ctx, orderID, scope)
	if err != nil {
		return Receipt{}, err
	}
	switch domain.OrderStatus(order.Status) {
	case domain.OrderDelivered:
	case domain.OrderReceived:
		return Receipt{}, ErrAlreadyReceived
	default:
		return Receipt{}, ErrNotDelivered
	}
	lines, err := BuildLines(order, in)
	if err != nil {
		return Receipt{}, err
	}
	return s.repo.Create(ctx, order, Receipt{
		OrderID:    order.OrderID,
		ReceivedAt: s.clock.Now(),
		ReceivedBy: actor,
		Notes:      strings.TrimSpace(in.Notes),
		Lines:      lines,
	})
}

// scopedOrder loads the order and applies the caller's scope. A store manager
// sees only their own outlet's orders; anything else reads as not found.
func (s *Service) scopedOrder(ctx context.Context, orderID string, scope Scope) (OrderContext, error) {
	if strings.TrimSpace(orderID) == "" {
		return OrderContext{}, fmt.Errorf("%w: order id is required", ErrNotFound)
	}
	order, err := s.repo.OrderContext(ctx, orderID)
	if err != nil {
		return OrderContext{}, err
	}
	switch scope.Role {
	case domain.RoleDispatcher:
		return order, nil
	case domain.RoleStoreManager:
		if scope.OutletID != "" && scope.OutletID == order.OutletID {
			return order, nil
		}
	}
	return OrderContext{}, fmt.Errorf("%w: order %s", ErrNotFound, orderID)
}
