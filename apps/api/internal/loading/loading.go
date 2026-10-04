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

	// The stop this line belongs to. The loader loads a vehicle stop by stop, so
	// the picking list has to say which drop a carton is for: the design (L-02)
	// shows the stop number on every row and loads the LAST stop first, nearest
	// the door. Seq is the planned stop order; the client reverses it.
	Seq         int
	OutletID    string
	OutletName  string
	OrderNumber string
	// DockType is how the goods come off at the outlet (REAR_DOCK / STREET /
	// MALL_BAY) — it changes how the loader stacks the cage.
	DockType string
	// TempRequirement is the order's regime (CHILLED / FROZEN / AMBIENT), so
	// chilled stock is not left standing on a dock.
	TempRequirement string
	// WeightKg and VolumeM3 are the order line's snapshotted totals, shown on the
	// row and summed into the vehicle's payload meters.
	WeightKg float64
	VolumeM3 float64
}

// ShortfallQty is the total quantity not loaded (damaged + missing).
func (l Line) ShortfallQty() int { return l.DamagedQty + l.MissingQty }

// unitShare returns the per-unit share of a line total. The order line carries
// the total for the ordered quantity, so the loaded portion is prorated.
func (l Line) unitShare(total float64) float64 {
	if l.OrderedQty <= 0 {
		return 0
	}
	return total / float64(l.OrderedQty) * float64(l.LoadedQty)
}

// LoadedWeightKg is the weight of the loaded portion of this line.
func (l Line) LoadedWeightKg() float64 { return l.unitShare(l.WeightKg) }

// LoadedVolumeM3 is the volume of the loaded portion of this line.
func (l Line) LoadedVolumeM3() float64 { return l.unitShare(l.VolumeM3) }

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

	// The vehicle being loaded. Its capacity turns the loaded weight and volume
	// into the "how full is the truck" meters the loader design shows; without it
	// the numbers would be a total with nothing to measure against.
	VehicleType string
	VehicleTemp string
	WeightCapKg float64
	VolumeCapM3 float64
}

// LoadedWeightKg is the weight actually on board: only the loaded portion of
// each line counts, so a shortfall lightens the truck.
func (r RouteLoading) LoadedWeightKg() float64 {
	var total float64
	for _, l := range r.Lines {
		total += l.LoadedWeightKg()
	}
	return total
}

// LoadedVolumeM3 mirrors LoadedWeightKg for cubic capacity.
func (r RouteLoading) LoadedVolumeM3() float64 {
	var total float64
	for _, l := range r.Lines {
		total += l.LoadedVolumeM3()
	}
	return total
}

// Stops counts the distinct outlets on the route's picking list.
func (r RouteLoading) Stops() int {
	seen := make(map[string]bool, len(r.Lines))
	for _, l := range r.Lines {
		seen[l.OutletID] = true
	}
	return len(seen)
}

// RouteSummary is one row of the loader's route list: enough to decide which
// route to pick next, without fetching every picking list.
type RouteSummary struct {
	RouteID   string
	VehicleID string
	DepotID   string
	RouteDate string
	TripNo    int
	Brand     domain.Brand
	District  string
	Status    string
	// Stops is the number of drops; Lines the number of order lines to count.
	Stops int
	Lines int
	// LinesComplete counts lines whose quantities reconcile, so the list can show
	// progress without the caller doing the arithmetic.
	LinesComplete int
	// ShortfallQty is the total damaged + missing across the route, so a route
	// needing attention is visible in the list.
	ShortfallQty int
}

// Ready reports whether every line on the route reconciles. A route with no
// lines is not ready, matching RouteLoading.Ready.
func (s RouteSummary) Ready() bool {
	return s.Lines > 0 && s.LinesComplete == s.Lines
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
