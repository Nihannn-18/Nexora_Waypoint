// Package routes owns the operational route/allocation layer: the confirmed
// trips a vehicle will actually run, the stops on them, and the allocation
// decisions (serve or defer) that record what happened to each order.
//
// Boundary with Planning (internal/planning): a planning run is proposal-only
// and writes planning_result rows. This package is where a dispatcher's
// confirmation turns a chosen proposal into authoritative route, route_leg and
// allocation rows, inside one transaction. Planning is never modified; it stays
// the source of proposals.
//
// Boundary with Orders (internal/orders): confirmation transitions a served
// order to ALLOCATED. Order lifecycle rules live there; this package only sets
// the status a confirmation implies.
//
// The package is deterministic: a confirmation is a pure function of the
// planning result plus current database state, and it touches no clock or
// randomness.
package routes

import (
	"errors"
	"fmt"

	"waypoint.lk/api/internal/domain"
)

// Route is one vehicle trip on one day, after confirmation. It mirrors the
// `route` table. `route_version` is the optimistic lock used by later edits.
type Route struct {
	RouteID       string
	VehicleID     string
	DepotID       string
	RouteDate     string // YYYY-MM-DD
	TripNo        int
	Brand         domain.Brand
	District      string
	Status        string
	RouteVersion  int
	OutboundMin   int
	InterStopMin  int
	HandlingMin   int
	TotalTripMin  int
	TotalWeightKg float64
	TotalVolumeM3 float64
	DistanceKm    float64
	// Legs are the ordered stops, seq starting at 0.
	Legs []RouteLeg
}

// RouteLeg is one stop on a route. It mirrors `route_leg`. `fromPoint` is
// "DEPOT" for the first leg and the previous outlet id thereafter.
type RouteLeg struct {
	LegID      string
	RouteID    string
	OrderID    string
	Seq        int
	FromPoint  string
	ToOutlet   string
	DistanceKm float64
	Status     string
	// PlannedArrival is the computed arrival as "HH:MM" on the route date, and
	// ServiceTimeMin the handling allowance for the stop. Both come from the
	// authoritative planner schedule, persisted so the loader and driver read a
	// stored fact. Empty/zero before confirmation computes them.
	PlannedArrival string
	ServiceTimeMin int
}

// Route statuses, mirroring the route CHECK constraint and ROUTE_STATUSES in
// libs/shared-types.
const (
	RouteDraft      = "DRAFT"
	RouteConfirmed  = "CONFIRMED"
	RouteDispatched = "DISPATCHED"
	RouteInTransit  = "IN_TRANSIT"
	RouteCompleted  = "COMPLETED"
	RouteCancelled  = "CANCELLED"
)

// Leg statuses, mirroring LEG_STATUSES.
const (
	LegPending = "PENDING"
)

// Allocation decision values, mirroring the allocation CHECK constraint.
// ALLOCATED here means the order was confirmed onto a route; DEFERRED means it
// was left unserved with a reason.
const (
	AllocationAllocated = "ALLOCATED"
	AllocationDeferred  = "DEFERRED"
)

// Sentinel errors. Handlers map these onto the shared HTTP error contract.
var (
	// ErrNotFound means no route/order matches the lookup.
	ErrNotFound = errors.New("routes: not found")
	// ErrInvalid means the confirmation input failed validation.
	ErrInvalid = errors.New("routes: invalid input")
	// ErrConflict means the requested confirmation conflicts with current
	// database state (e.g. an order is already allocated for the date, or the
	// route already exists).
	ErrConflict = errors.New("routes: conflict")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// ValidRouteStatus reports whether s is a canonical route status.
func ValidRouteStatus(s string) bool {
	switch s {
	case RouteDraft, RouteConfirmed, RouteDispatched, RouteInTransit, RouteCompleted, RouteCancelled:
		return true
	}
	return false
}

// ValidTripNo reports whether n is a legal trip number (1 or 2).
func ValidTripNo(n int) bool { return n == 1 || n == 2 }

// Validate checks a route's invariants before persistence. It enforces the same
// rules the schema does, so a bad route fails in the domain first.
func (r Route) Validate() error {
	if r.VehicleID == "" {
		return ValidationError{Field: "vehicleId", Message: "is required"}
	}
	if r.DepotID == "" {
		return ValidationError{Field: "depotId", Message: "is required"}
	}
	if r.RouteDate == "" {
		return ValidationError{Field: "routeDate", Message: "is required"}
	}
	if !ValidTripNo(r.TripNo) {
		return ValidationError{Field: "tripNo", Message: "must be 1 or 2"}
	}
	if !r.Brand.Valid() {
		return ValidationError{Field: "brand", Message: "must be one of FRESH, STYLE, TECH"}
	}
	if r.District == "" {
		return ValidationError{Field: "district", Message: "is required"}
	}
	if !ValidRouteStatus(r.Status) {
		return ValidationError{Field: "status", Message: "is not a known route status"}
	}
	if len(r.Legs) == 0 {
		return ValidationError{Field: "legs", Message: "a route needs at least one leg"}
	}
	for i, leg := range r.Legs {
		if leg.Seq != i {
			return ValidationError{Field: fmt.Sprintf("legs[%d].seq", i), Message: "must be contiguous from 0"}
		}
		if leg.OrderID == "" {
			return ValidationError{Field: fmt.Sprintf("legs[%d].orderId", i), Message: "is required"}
		}
		if leg.ToOutlet == "" {
			return ValidationError{Field: fmt.Sprintf("legs[%d].toOutlet", i), Message: "is required"}
		}
	}
	return nil
}

// ValidateLegOrder checks that a route's legs are sequenced contiguously from 0
// and that each stop's fromPoint is DEPOT for the first leg, else the previous
// outlet. It is separate from Validate so the ordering rule can be tested alone.
func ValidateLegOrder(legs []RouteLeg) error {
	for i, leg := range legs {
		if leg.Seq != i {
			return ValidationError{Field: "seq", Message: fmt.Sprintf("leg %d has seq %d, want %d", i, leg.Seq, i)}
		}
		wantFrom := "DEPOT"
		if i > 0 {
			wantFrom = legs[i-1].ToOutlet
		}
		if leg.FromPoint != wantFrom {
			return ValidationError{Field: "fromPoint", Message: fmt.Sprintf("leg %d fromPoint %q, want %q", i, leg.FromPoint, wantFrom)}
		}
	}
	return nil
}
