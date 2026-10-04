package loading

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service is the loading business surface. It validates submissions and enforces
// route/authorization rules; the repository owns persistence and transactions.
type Service struct {
	repo  Repository
	clock Clock
}

// Clock reports the current business-time instant. Injected so "today" is the
// API clock (the demo clock under DEMO_MODE), never the wall clock or a device.
type Clock interface{ Now() time.Time }

// NewService builds the loading service. clock may be nil in narrow tests, in
// which case the active-run lookup falls back to the wall clock.
func NewService(repo Repository, clock Clock) *Service { return &Service{repo: repo, clock: clock} }

func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock.Now()
}

// PickingList returns a route's picking list, after checking the caller may see
// the route (same depot). depotID is the caller's depot; empty means the caller
// is not depot-scoped and is refused.
func (s *Service) PickingList(ctx context.Context, routeID, callerDepotID string) (RouteLoading, error) {
	if strings.TrimSpace(routeID) == "" {
		return RouteLoading{}, ValidationError{Field: "routeId", Message: "is required"}
	}
	rl, err := s.repo.RouteLoading(ctx, routeID)
	if err != nil {
		return RouteLoading{}, err
	}
	if !depotAllowed(callerDepotID, rl.DepotID) {
		return RouteLoading{}, fmt.Errorf("%w: route %s is at another depot", ErrNotFound, routeID)
	}
	return rl, nil
}

// Routes lists the confirmed routes the caller's depot must load.
//
// The date is optional: when it is omitted the depot's active run is resolved
// from the routes themselves — the earliest confirmed run that has not passed,
// or the latest one when none is upcoming — so the loader opens on the work
// that is actually actionable instead of a date guessed from a device clock.
// A caller with no depot gets nothing rather than everything: the loader
// endpoints fail closed, exactly as PickingList does.
func (s *Service) Routes(ctx context.Context, callerDepotID, date string) ([]RouteSummary, error) {
	if callerDepotID == "" {
		return []RouteSummary{}, nil
	}
	if strings.TrimSpace(date) == "" {
		active, err := s.repo.ActiveRouteDate(ctx, callerDepotID, s.now())
		if err != nil {
			return nil, err
		}
		if active == "" {
			return []RouteSummary{}, nil
		}
		return s.repo.RoutesForDepot(ctx, callerDepotID, active)
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return nil, ValidationError{Field: "date", Message: "must be YYYY-MM-DD"}
	}
	return s.repo.RoutesForDepot(ctx, callerDepotID, date)
}

// RecordShortfalls validates and records a full set of line updates for a route.
// It rejects an empty submission, duplicate order lines, and any line whose
// quantities do not reconcile against the database's ordered quantity; the
// per-line validation happens again inside the repository transaction against
// the authoritative quantity.
func (s *Service) RecordShortfalls(ctx context.Context, routeID, callerDepotID, actor string, updates []LineUpdate) (RouteLoading, error) {
	if strings.TrimSpace(routeID) == "" {
		return RouteLoading{}, ValidationError{Field: "routeId", Message: "is required"}
	}
	if len(updates) == 0 {
		return RouteLoading{}, ValidationError{Field: "items", Message: "at least one line is required"}
	}
	seen := make(map[string]bool, len(updates))
	for i, u := range updates {
		if strings.TrimSpace(u.OrderItemID) == "" {
			return RouteLoading{}, ValidationError{Field: fmt.Sprintf("items[%d].orderItemId", i), Message: "is required"}
		}
		if seen[u.OrderItemID] {
			return RouteLoading{}, ValidationError{Field: fmt.Sprintf("items[%d].orderItemId", i), Message: "duplicate order line in one submission"}
		}
		seen[u.OrderItemID] = true
		if u.LoadedQty < 0 || u.DamagedQty < 0 || u.MissingQty < 0 {
			return RouteLoading{}, ValidationError{Field: fmt.Sprintf("items[%d].quantity", i), Message: "quantities must not be negative"}
		}
		if !ValidShortfallPhoto(u.OrderItemID, u.PhotoRef) {
			return RouteLoading{}, ValidationError{Field: fmt.Sprintf("items[%d].photoRef", i), Message: "must be a shortfall media key for this order line"}
		}
	}

	// Resolve the route to check scope before writing.
	current, err := s.repo.RouteLoading(ctx, routeID)
	if err != nil {
		return RouteLoading{}, err
	}
	if !depotAllowed(callerDepotID, current.DepotID) {
		return RouteLoading{}, fmt.Errorf("%w: route %s is at another depot", ErrNotFound, routeID)
	}

	return s.repo.RecordShortfalls(ctx, routeID, actor, updates)
}

// depotAllowed reports whether a depot-scoped caller may operate on a route at
// routeDepot. An empty caller depot is refused (fail closed).
func depotAllowed(callerDepotID, routeDepot string) bool {
	return callerDepotID != "" && callerDepotID == routeDepot
}
