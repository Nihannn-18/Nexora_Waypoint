// Package loading owns the warehouse/loader workflow for a confirmed route: the
// per-order-line load counts (loaded / damaged / missing), the optional
// shortfall photo, and whether a route is ready to leave the dock.
//
// Boundaries:
//   - Upstream: internal/routes produced the confirmed route, legs and
//     allocations. Loading reads that operational state; it never loads a
//     planning_result or an unconfirmed route.
//   - Downstream: Driver delivery / POD is a later agent. This package stops at
//     "route has been loaded / shortfalls recorded / ready for delivery".
//
// The order quantity is authoritative and is never mutated here. A shortfall is
// represented as damaged_qty + missing_qty on the load record, leaving
// order_item.quantity untouched. Per docs/data-model.md the line invariant is
// loaded + damaged + missing = ordered; the service enforces that equality.
package loading

import (
	"errors"
	"fmt"
	"strings"

	"waypoint.lk/api/internal/domain"
)

// Line is the loader's count for one order line of a route. It mirrors a
// load_item row plus the catalogue identity of the line, which the picking list
// shows.
type Line struct {
	LoadItemID  string
	RouteID     string
	OrderItemID string
	OrderID     string
	ItemID      string
	SKU         string
	Name        string
	// OrderedQty is read from order_item.quantity — authoritative, never trusted
	// from the client.
	OrderedQty int
	LoadedQty  int
	DamagedQty int
	MissingQty int
	// PhotoRef is a server-generated media key scoped to "shortfall/<orderItemID>/".
	PhotoRef   string
	RecordedBy string
	RecordedAt string
}

// ShortfallQty is the total quantity not loaded (damaged + missing).
func (l Line) ShortfallQty() int { return l.DamagedQty + l.MissingQty }

// Complete reports whether the line reconciles: loaded + damaged + missing
// equals the ordered quantity.
func (l Line) Complete() bool { return l.LoadedQty+l.DamagedQty+l.MissingQty == l.OrderedQty }

// HasShortfall reports whether any quantity is damaged or missing.
func (l Line) HasShortfall() bool { return l.ShortfallQty() > 0 }

// RouteLoading is the loader's view of one route: its ordered picks and whether
// every line reconciles.
type RouteLoading struct {
	RouteID   string
	VehicleID string
	DepotID   string
	RouteDate string
	TripNo    int
	Brand     domain.Brand
	District  string
	Status    string
	Lines     []Line
}

// Ready reports whether every line reconciles, so the route can be dispatched.
// A route with no lines is not ready.
func (r RouteLoading) Ready() bool {
	if len(r.Lines) == 0 {
		return false
	}
	for _, l := range r.Lines {
		if !l.Complete() {
			return false
		}
	}
	return true
}

// LineUpdate is one line in a shortfall submission. OrderedQty is deliberately
// absent: the server reads it from the database.
type LineUpdate struct {
	OrderItemID string
	LoadedQty   int
	DamagedQty  int
	MissingQty  int
	// PhotoRef is optional; when present it must be a shortfall key for this
	// order item.
	PhotoRef string
}

// Sentinel errors. Handlers map these onto the shared HTTP error contract.
var (
	// ErrNotFound means no route/line matches the lookup.
	ErrNotFound = errors.New("loading: not found")
	// ErrInvalid means a line failed validation.
	ErrInvalid = errors.New("loading: invalid input")
	// ErrConflict means the request conflicts with current state (e.g. a
	// non-confirmed route, or a stale/duplicate submission that cannot apply).
	ErrConflict = errors.New("loading: conflict")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// PhotoPrefixFor returns the required prefix of a shortfall media key for an
// order item: "shortfall/<orderItemID>/". This mirrors media.KeyFor with
// PurposeShortfall and the order item as owner.
func PhotoPrefixFor(orderItemID string) string { return "shortfall/" + orderItemID + "/" }

// ValidShortfallPhoto reports whether ref is a server-generated shortfall key
// for this order item. An empty ref is valid (the photo is optional).
func ValidShortfallPhoto(orderItemID, ref string) bool {
	if ref == "" {
		return true
	}
	prefix := PhotoPrefixFor(orderItemID)
	if !strings.HasPrefix(ref, prefix) {
		return false
	}
	// The remainder must be a non-empty, traversal-free object id.
	rest := strings.TrimPrefix(ref, prefix)
	return rest != "" && !strings.Contains(rest, "/") && !strings.Contains(rest, "..")
}

// ValidateUpdate checks one submitted line against the authoritative ordered
// quantity. It enforces the business invariant loaded + damaged + missing =
// ordered, non-negative quantities, and the photo scoping. It never mutates the
// order quantity.
func ValidateUpdate(u LineUpdate, orderedQty int) error {
	if strings.TrimSpace(u.OrderItemID) == "" {
		return ValidationError{Field: "orderItemId", Message: "is required"}
	}
	if u.LoadedQty < 0 || u.DamagedQty < 0 || u.MissingQty < 0 {
		return ValidationError{Field: "quantity", Message: "quantities must not be negative"}
	}
	sum := u.LoadedQty + u.DamagedQty + u.MissingQty
	if sum != orderedQty {
		return ValidationError{
			Field:   "quantity",
			Message: fmt.Sprintf("loaded + damaged + missing (%d) must equal the ordered quantity (%d)", sum, orderedQty),
		}
	}
	if !ValidShortfallPhoto(u.OrderItemID, u.PhotoRef) {
		return ValidationError{Field: "photoRef", Message: "must be a shortfall media key for this order line"}
	}
	return nil
}
