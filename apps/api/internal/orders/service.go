package orders

import (
	"context"
	"fmt"
	"strings"
	"time"

	"waypoint.lk/api/internal/catalog"
	"waypoint.lk/api/internal/domain"
)

// CutoffHour is the business-day order cutoff: 16:00 Asia/Colombo. An order
// placed at or after this hour is accepted for the following operating run and
// flagged after_cutoff, never rejected.
const CutoffHour = 16

// Outlet is the slice of an outlet the order intake needs: brand (for the
// brand-match rule) and depot (for authorization). It is a narrow local view so
// orders does not depend on a full outlet domain that does not exist yet.
type Outlet struct {
	OutletID string
	Brand    domain.Brand
	DepotID  string
}

// OutletReader loads an outlet by id. The production implementation reads the
// outlet table; tests use a fake. Keep it read-only — nothing in order intake
// edits outlets.
type OutletReader interface {
	GetOutlet(ctx context.Context, outletID string) (Outlet, error)
}

// Clock supplies the current business-time instant. It is injected so the 16:00
// cutoff is deterministic in tests and so no business logic calls time.Now()
// directly (the repository's rule). The clock package does not exist yet
// (fix/waypoint/backend-contract-gaps), so this interface is the seam; main wires
// a small real implementation.
type Clock interface {
	Now() time.Time
}

// Catalogue is the part of catalog.Service the order intake uses. Declared here
// as an interface so tests can substitute a fake without a database.
type Catalogue interface {
	ResolveMany(ctx context.Context, itemIDs []string) (map[string]catalog.Item, error)
}

// Service owns order intake and the lifecycle rules that are already defined.
// It does not decide feasibility, allocation, routes or delivery.
type Service struct {
	repo      Repository
	catalogue Catalogue
	outlets   OutletReader
	clock     Clock
}

// NewService builds the order service. All four dependencies are required.
func NewService(repo Repository, catalogue Catalogue, outlets OutletReader, clock Clock) *Service {
	return &Service{repo: repo, catalogue: catalogue, outlets: outlets, clock: clock}
}

// CreateInput is the validated-free request to create an order. Brand is derived
// from the outlet, not supplied by the client; the client supplies outlet,
// delivery date, lines and optional notes.
type CreateInput struct {
	OutletID              string
	RequestedDeliveryDate time.Time
	Lines                 []LineRequest
	Notes                 string
}

// Create validates and persists a new order:
//
//  1. the outlet exists (and its brand drives the order brand);
//  2. every line's SKU exists and belongs to that brand;
//  3. dimensions are snapshotted from the catalogue and totals computed;
//  4. the 16:00 cutoff sets after_cutoff but never rejects the order.
//
// The order is created PLACED. Confirmation is a separate, explicit action.
func (s *Service) Create(ctx context.Context, in CreateInput) (Order, error) {
	if strings.TrimSpace(in.OutletID) == "" {
		return Order{}, ValidationError{Field: "outletId", Message: "is required"}
	}
	if in.RequestedDeliveryDate.IsZero() {
		return Order{}, ValidationError{Field: "requestedDeliveryDate", Message: "is required"}
	}
	if len(in.Lines) == 0 {
		return Order{}, ValidationError{Field: "lines", Message: "an order needs at least one line"}
	}

	outlet, err := s.outlets.GetOutlet(ctx, in.OutletID)
	if err != nil {
		if isNotFound(err) {
			return Order{}, fmt.Errorf("%w: outlet %s", ErrInvalid, in.OutletID)
		}
		return Order{}, err
	}

	// Resolve the SKUs once, then reject unknown ids and brand mismatches before
	// any write. An order may not reference an unknown SKU or span brands.
	itemIDs := make([]string, 0, len(in.Lines))
	for _, ln := range in.Lines {
		if strings.TrimSpace(ln.ItemID) == "" {
			return Order{}, ValidationError{Field: "lines.itemId", Message: "is required"}
		}
		if ln.Quantity <= 0 {
			return Order{}, ValidationError{Field: "lines.quantity", Message: "must be greater than zero"}
		}
		itemIDs = append(itemIDs, ln.ItemID)
	}

	resolved, err := s.catalogue.ResolveMany(ctx, itemIDs)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return Order{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return Order{}, err
	}

	lines := make([]OrderLine, 0, len(in.Lines))
	reqs := make([]domain.TempRequirement, 0, len(in.Lines))
	for _, ln := range in.Lines {
		item, ok := resolved[ln.ItemID]
		if !ok {
			return Order{}, ValidationError{Field: "lines.itemId", Message: ln.ItemID + " does not exist"}
		}
		if item.Brand != outlet.Brand {
			return Order{}, ValidationError{
				Field:   "lines.itemId",
				Message: fmt.Sprintf("%s is a %s item but the outlet is %s", item.SKU, item.Brand, outlet.Brand),
			}
		}
		lines = append(lines, OrderLine{
			ItemID:               item.ItemID,
			Quantity:             ln.Quantity,
			UnitWeightKgSnapshot: item.UnitWeightKg,
			UnitVolumeM3Snapshot: item.UnitVolumeM3,
			TotalWeightKg:        float64(ln.Quantity) * item.UnitWeightKg,
			TotalVolumeM3:        float64(ln.Quantity) * item.UnitVolumeM3,
		})
		reqs = append(reqs, item.TemperatureRequirement)
	}

	now := s.now()
	orderDate := dateOnly(now)
	afterCutoff := now.Hour() >= CutoffHour

	units, weight, volume := ComputeTotals(lines)
	order := Order{
		OutletID:              outlet.OutletID,
		Brand:                 outlet.Brand,
		OrderDate:             orderDate,
		RequestedDeliveryDate: dateOnly(in.RequestedDeliveryDate),
		TotalUnits:            units,
		TotalWeightKg:         weight,
		TotalVolumeM3:         volume,
		TempRequirement:       TemperatureFor(reqs),
		Status:                domain.OrderPlaced,
		AfterCutoff:           afterCutoff,
		Notes:                 strings.TrimSpace(in.Notes),
		Lines:                 lines,
	}

	if err := order.Validate(); err != nil {
		return Order{}, err
	}
	return s.repo.Create(ctx, order)
}

// Get returns an order by id, scoped by the caller's outlet when they are a
// store manager. A dispatcher sees every order. The scope check is deliberately
// here rather than in the handler so every read path is consistent.
func (s *Service) Get(ctx context.Context, orderID string, scope Scope) (Order, error) {
	if strings.TrimSpace(orderID) == "" {
		return Order{}, ValidationError{Field: "orderId", Message: "is required"}
	}
	order, err := s.repo.GetByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if !scope.CanRead(order.OutletID, order.Brand) {
		// Out of scope reads are reported as not found so a caller cannot probe
		// for the existence of another outlet's order.
		return Order{}, fmt.Errorf("%w: %s", ErrNotFound, orderID)
	}
	return order, nil
}

// List returns orders matching filter, scoped to the caller. A store manager is
// pinned to their outlet; a dispatcher may list across outlets.
func (s *Service) List(ctx context.Context, filter Filter, scope Scope) ([]Order, error) {
	if filter.Status != "" && !ValidStatus(domain.OrderStatus(filter.Status)) {
		return nil, ValidationError{Field: "status", Message: "is not a known order status"}
	}
	if !scope.AllOutlets() {
		if scope.OutletID == "" {
			return nil, fmt.Errorf("%w: caller has no outlet scope", ErrInvalid)
		}
		if filter.OutletID != "" && filter.OutletID != scope.OutletID {
			return nil, fmt.Errorf("%w: outlet out of scope", ErrNotFound)
		}
		filter.OutletID = scope.OutletID
	}
	return s.repo.List(ctx, filter)
}

// Confirm moves a PLACED (or re-entered DEFERRED) order to CONFIRMED, applying
// the cutoff policy. Confirming again is a conflict, not a silent no-op.
//
// Feasibility is not checked here: whether the order can actually be served is
// the constraint validator's and planning engine's decision.
func (s *Service) Confirm(ctx context.Context, orderID string, scope Scope) (Order, error) {
	order, err := s.Get(ctx, orderID, scope)
	if err != nil {
		return Order{}, err
	}
	if !CanTransition(order.Status, domain.OrderConfirmed) {
		return Order{}, fmt.Errorf("%w: cannot confirm an order in status %s", ErrConflict, order.Status)
	}
	return s.repo.UpdateStatus(ctx, order.OrderID, string(domain.OrderConfirmed))
}

// now returns the injected clock's instant, falling back to the wall clock only
// if no clock was supplied (a wiring error that should fail loudly elsewhere).
func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock.Now()
}

// dateOnly truncates an instant to its date in UTC, matching the DATE columns.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Scope is the caller's outlet authorization for order reads. It is a narrow
// view of auth.Identity so orders does not import auth directly (keeps the
// dependency direction clean and the service unit-testable).
type Scope struct {
	// Role is the caller's role.
	Role domain.Role
	// OutletID is set for a store manager; empty otherwise.
	OutletID string
	// DepotID is set for a loader/driver; empty for a dispatcher.
	DepotID string
}

// AllOutlets reports whether the caller may read orders across outlets.
func (s Scope) AllOutlets() bool { return s.Role == domain.RoleDispatcher }

// CanRead reports whether the caller may read an order for outletID/brand.
func (s Scope) CanRead(outletID string, _ domain.Brand) bool {
	if s.AllOutlets() {
		return true
	}
	if s.Role == domain.RoleStoreManager {
		return s.OutletID != "" && s.OutletID == outletID
	}
	// Loader and Driver are depot-scoped, not outlet-scoped; their access to a
	// specific order is decided by the route they are assigned, which is not an
	// orders concern. Until that exists, only the store manager and dispatcher
	// are recognised here and any other role is denied.
	return false
}
