package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service is the delivery business surface: it validates driver scope and event
// shape, then delegates the transactional write to the repository. It also
// implements the batch sync reconciliation, processing each event independently.
type Service struct {
	repo  Repository
	clock Clock
	// assignments resolves the vehicle a driver is assigned to for a date. It is
	// optional: when nil, or when no assignment exists, the cockpit falls back to
	// the depot's routes and the driver selects explicitly (the pre-assignment
	// behaviour), so nothing regresses on a system with no assignments.
	assignments AssignmentResolver
}

// Clock reports the current business-time instant, so the active-run lookup
// uses the API clock (the demo clock under DEMO_MODE), never a device clock.
type Clock interface{ Now() time.Time }

// AssignmentResolver resolves the vehicle a driver is assigned to on a date.
// It is satisfied by *assignment.PGStore; the narrow signature keeps delivery
// free of the assignment package's types.
type AssignmentResolver interface {
	AssignedVehicle(ctx context.Context, driverID, date string) (vehicleID string, ok bool, err error)
}

// NewService builds the delivery service. clock may be nil in narrow tests, in
// which case the active-run lookup falls back to the wall clock.
func NewService(repo Repository, clock Clock) *Service { return &Service{repo: repo, clock: clock} }

// WithAssignments attaches the driver-assignment resolver.
func (s *Service) WithAssignments(r AssignmentResolver) *Service {
	s.assignments = r
	return s
}

func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock.Now()
}

// RecordOne validates and records a single delivery event for a leg. The caller
// identity's depot must match the leg's route depot (the strongest available
// driver-to-route scope; see the driver assignment note in the docs).
func (s *Service) RecordOne(ctx context.Context, actor, actorDepot string, in EventInput) (EventResult, error) {
	if err := ValidateEvent(in); err != nil {
		return EventResult{}, err
	}
	leg, err := s.repo.LegContext(ctx, in.LegID)
	if err != nil {
		return EventResult{}, err
	}
	if !depotAllowed(actorDepot, leg.DepotID) {
		// Out of scope is reported as not found so a driver cannot probe other
		// depots' routes.
		return EventResult{}, fmt.Errorf("%w: leg %s", ErrNotFound, in.LegID)
	}
	return s.repo.Record(ctx, actor, in, leg)
}

// SyncBatch processes offline events independently, in capture order. Each event
// yields its own result; one bad event never discards the rest. The batch is not
// itself transactional: a retry of the whole batch is safe because each event is
// idempotent on client_event_id.
func (s *Service) SyncBatch(ctx context.Context, actor, actorDepot string, events []EventInput) ([]EventResult, error) {
	if len(events) == 0 {
		return nil, ValidationError{Field: "events", Message: "at least one event is required"}
	}
	results := make([]EventResult, 0, len(events))
	for _, in := range events {
		res, err := s.RecordOne(ctx, actor, actorDepot, in)
		if err != nil {
			// Reconcile per event: a rejected event gets its own result and does
			// not fail the batch.
			results = append(results, EventResult{
				ClientEventID: in.ClientEventID,
				Status:        SyncRejected,
				Reason:        rejectReason(err),
			})
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// LegDetail returns a leg's stop detail, scoped to the caller's depot.
func (s *Service) LegDetail(ctx context.Context, legID, actorDepot string) (LegDetail, error) {
	if strings.TrimSpace(legID) == "" {
		return LegDetail{}, ValidationError{Field: "legId", Message: "is required"}
	}
	leg, err := s.repo.LegDetail(ctx, legID)
	if err != nil {
		return LegDetail{}, err
	}
	if !depotAllowed(actorDepot, leg.DepotID) {
		return LegDetail{}, fmt.Errorf("%w: leg %s", ErrNotFound, legID)
	}
	return leg, nil
}

// DriverRoutes lists the caller's driveable routes.
//
// The date is optional: when it is omitted the depot's active run is resolved
// from the routes themselves — the earliest driveable run that has not passed,
// or the latest one when none is upcoming — so the cockpit opens on the planned
// run rather than a date taken from the phone.
//
// When the Dispatcher has assigned this driver to a vehicle for the resolved
// date, only that vehicle's routes are returned, so the driver sees their own
// run and not every route in the depot. Without an assignment (or with no
// resolver wired) the depot's routes are returned and the driver chooses
// explicitly, exactly as before. A driver is never silently handed an arbitrary
// route: with several candidates and no assignment the client asks.
func (s *Service) DriverRoutes(ctx context.Context, actorUserID, actorDepot, date string) ([]DriverRoute, error) {
	if actorDepot == "" {
		return []DriverRoute{}, nil // fail closed: no depot, no routes
	}
	resolvedDate := strings.TrimSpace(date)
	if resolvedDate == "" {
		active, err := s.repo.ActiveRouteDate(ctx, actorDepot, s.now())
		if err != nil {
			return nil, err
		}
		resolvedDate = active
	} else if _, err := time.Parse(time.DateOnly, resolvedDate); err != nil {
		return nil, ValidationError{Field: "date", Message: "must be YYYY-MM-DD"}
	}
	if resolvedDate == "" {
		return []DriverRoute{}, nil
	}
	routes, err := s.repo.DriverRoutes(ctx, actorDepot, resolvedDate)
	if err != nil {
		return nil, err
	}
	return s.narrowToAssignment(ctx, actorUserID, resolvedDate, routes)
}

// narrowToAssignment returns only the assigned vehicle's routes, if the driver
// has an assignment for the date. A missing assignment is not an error: it
// leaves the caller with the depot's routes and the existing explicit choice.
func (s *Service) narrowToAssignment(ctx context.Context, driverID, date string, routes []DriverRoute) ([]DriverRoute, error) {
	if s.assignments == nil || driverID == "" {
		return routes, nil
	}
	vehicleID, ok, err := s.assignments.AssignedVehicle(ctx, driverID, date)
	if err != nil {
		return nil, err
	}
	if !ok {
		return routes, nil
	}
	out := make([]DriverRoute, 0, len(routes))
	for _, r := range routes {
		if r.VehicleID == vehicleID {
			out = append(out, r)
		}
	}
	return out, nil
}

// SyncStatus returns the caller's synced/conflict counts.
func (s *Service) SyncStatus(ctx context.Context, actor string) (SyncStatus, error) {
	return s.repo.SyncStatus(ctx, actor)
}

// depotAllowed reports whether a depot-scoped driver may operate on a leg's
// route. An empty caller depot is refused (fail closed).
func depotAllowed(callerDepot, legDepot string) bool {
	return callerDepot != "" && callerDepot == legDepot
}

// rejectReason maps an error to a short, non-sensitive reason string.
func rejectReason(err error) string {
	switch {
	case errors.Is(err, ErrInvalid):
		return "invalid event"
	case errors.Is(err, ErrNotFound):
		return "unknown or out-of-scope leg"
	case errors.Is(err, ErrConflict):
		return "leg changed; needs review"
	default:
		return "could not be applied"
	}
}
