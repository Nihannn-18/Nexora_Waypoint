// Package orders owns the customer order: its header, its lines, the lifecycle
// status, and the intake rules that decide whether an order may be placed.
//
// It mirrors the `customer_order` and `order_item` tables in docs/data-model.md
// and the `CustomerOrder`/`OrderLine` read models in libs/shared-types. It
// depends on internal/catalog for SKU data and internal/auth for identity, and
// on nothing in planning, routes, loading or delivery — the order is a fact,
// not a plan.
//
// Boundaries that belong elsewhere and are deliberately NOT here:
//   - vehicle/trip feasibility, capacity, fuel and deferral decisions
//     (internal/constraint, internal/planning);
//   - route/leg construction and allocation persistence (internal/routes);
//   - loading, shortfalls and delivery outcomes.
//
// Aggregation rule (docs/data-model.md): totals are computed from the lines on
// write and never accepted from the client. Each line snapshots the catalogue's
// unit weight/volume so a later catalogue edit cannot retroactively change a
// historical order's totals — and therefore cannot invalidate a plan that was
// feasible when it was made.
package orders

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Order is a customer order header. IDs are server-generated UUIDs; the
// human-facing identifier is OrderNumber.
type Order struct {
	OrderID     string
	OrderNumber string
	OutletID    string
	Brand       domain.Brand
	// OrderDate is the business day the order was placed on.
	OrderDate time.Time
	// RequestedDeliveryDate is the day the outlet wants delivery; it defaults to
	// the next operating day and is never assumed from the wall clock here.
	RequestedDeliveryDate time.Time
	// Totals are computed from the lines on write (see ComputeTotals).
	TotalUnits    int
	TotalWeightKg float64
	TotalVolumeM3 float64
	// TempRequirement is CHILLED/FROZEN if any line requires it, else AMBIENT.
	TempRequirement domain.TempRequirement
	Status          domain.OrderStatus
	// AfterCutoff is true when the order arrived after the 16:00 cutoff and is
	// held for the following operating run (accepted, never rejected).
	AfterCutoff bool
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// DeferredYesterday and DaysSinceLastServed carry the supplied fairness
	// history (migration 00003). They are stored per order and read per outlet by
	// the planning policy; orders exposes them verbatim and does not aggregate.
	DeferredYesterday   bool
	DaysSinceLastServed *int
	// Deferral is the latest deferral decision, loaded on a scoped read so the
	// store manager can see why their order was deferred. Nil when the order has
	// never been deferred.
	Deferral *Deferral
	// Lines are the order's lines. Populated on read; required on create.
	Lines []OrderLine
}

// Deferral is the store-facing view of the latest deferral_log row for an order.
type Deferral struct {
	ReasonText     string
	ConstraintCode string
	DecidedAt      time.Time
	DeferredToDate string
}

// OrderLine is one SKU on an order, in the quantity requested. The snapshot
// columns are the point of the line: they freeze the catalogue dimensions at
// order time.
type OrderLine struct {
	OrderItemID string
	OrderID     string
	ItemID      string
	Quantity    int
	// SKU and Name are the item's current display identity, joined on read for
	// the order detail. They are not snapshots: only the dimensions are frozen.
	SKU  string
	Name string
	// Snapshot values captured from the catalogue at creation.
	UnitWeightKgSnapshot float64
	UnitVolumeM3Snapshot float64
	// Line totals = quantity × snapshot, computed on write.
	TotalWeightKg float64
	TotalVolumeM3 float64
}

// LineRequest is the minimal input to create a line: a SKU and a quantity. The
// dimensions are resolved from the catalogue, never supplied by the client.
type LineRequest struct {
	ItemID   string
	Quantity int
}

// Sentinel errors. Handlers map these onto the shared HTTP error contract; they
// are not wire codes in their own right.
var (
	// ErrNotFound means no order matches the lookup.
	ErrNotFound = errors.New("orders: not found")
	// ErrInvalid means the order failed domain validation.
	ErrInvalid = errors.New("orders: invalid input")
	// ErrConflict means the requested transition is not allowed from the
	// order's current status.
	ErrConflict = errors.New("orders: illegal transition")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// isNotFound reports whether err wraps ErrNotFound.
func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// OrderStatusValues is the canonical set of order statuses, matching the
// customer_order CHECK constraint and ORDER_STATUSES in shared-types.
var OrderStatusValues = []domain.OrderStatus{
	domain.OrderPlaced, domain.OrderConfirmed, domain.OrderAllocated,
	domain.OrderLoaded, domain.OrderInTransit, domain.OrderDelivered,
	domain.OrderFailed, domain.OrderReceived, domain.OrderDeferred,
}

// ValidStatus reports whether s is one of the canonical order statuses. A zero
// or unknown status is never valid.
func ValidStatus(s domain.OrderStatus) bool {
	for _, known := range OrderStatusValues {
		if s == known {
			return true
		}
	}
	return false
}

// transitions mirrors ORDER_STATUS_TRANSITIONS in libs/shared-types. The client
// uses it to grey out impossible actions; the server enforces it here. Keep the
// two in step: a change on one side is a change on the other.
var transitions = map[domain.OrderStatus][]domain.OrderStatus{
	domain.OrderPlaced:    {domain.OrderConfirmed, domain.OrderDeferred},
	domain.OrderConfirmed: {domain.OrderAllocated, domain.OrderDeferred},
	domain.OrderAllocated: {domain.OrderLoaded, domain.OrderDeferred},
	domain.OrderLoaded:    {domain.OrderInTransit, domain.OrderDeferred},
	domain.OrderInTransit: {domain.OrderDelivered, domain.OrderFailed},
	domain.OrderDelivered: {domain.OrderReceived},
	domain.OrderFailed:    {domain.OrderDeferred},
	domain.OrderReceived:  {},
	domain.OrderDeferred:  {domain.OrderConfirmed},
}

// CanTransition reports whether from → to is a legal order transition.
func CanTransition(from, to domain.OrderStatus) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ValidateOrderNumber checks the human identifier's shape, ORD-YYYY-NNNNNN:
// the literal "ORD-", a four-digit year, "-", then six digits.
func ValidateOrderNumber(n string) bool {
	const (
		prefix = "ORD-"
		suffix = 6
	)
	if len(n) != len(prefix)+4+1+suffix || !strings.HasPrefix(n, prefix) {
		return false
	}
	year := n[len(prefix) : len(prefix)+4]
	sep := n[len(prefix)+4]
	seq := n[len(prefix)+5:]
	if sep != '-' {
		return false
	}
	for _, r := range year + seq {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Validate checks the header invariants the schema enforces, so the domain
// rejects the same bad data the database would — before a round trip. It does
// not check catalogue existence or brand matching; the service does that with
// the catalogue in hand.
func (o Order) Validate() error {
	if strings.TrimSpace(o.OutletID) == "" {
		return ValidationError{Field: "outletId", Message: "is required"}
	}
	if !o.Brand.Valid() {
		return ValidationError{Field: "brand", Message: "must be one of FRESH, STYLE, TECH"}
	}
	if !ValidStatus(o.Status) {
		return ValidationError{Field: "status", Message: "is not a known order status"}
	}
	if !o.TempRequirement.Valid() {
		return ValidationError{Field: "temperatureRequirement", Message: "must be one of AMBIENT, CHILLED, FROZEN"}
	}
	if o.OrderDate.IsZero() {
		return ValidationError{Field: "orderDate", Message: "is required"}
	}
	if o.RequestedDeliveryDate.IsZero() {
		return ValidationError{Field: "requestedDeliveryDate", Message: "is required"}
	}
	if o.RequestedDeliveryDate.Before(o.OrderDate) {
		return ValidationError{Field: "requestedDeliveryDate", Message: "must not be before orderDate"}
	}
	if len(o.Lines) == 0 {
		return ValidationError{Field: "lines", Message: "an order needs at least one line"}
	}
	for i, ln := range o.Lines {
		if strings.TrimSpace(ln.ItemID) == "" {
			return ValidationError{Field: fmt.Sprintf("lines[%d].itemId", i), Message: "is required"}
		}
		if ln.Quantity <= 0 {
			return ValidationError{Field: fmt.Sprintf("lines[%d].quantity", i), Message: "must be greater than zero"}
		}
	}
	return nil
}

// ComputeTotals derives the header totals from the lines, using each line's
// snapshot. Temperature is a catalogue property, so it is not derived here;
// TemperatureFor computes it separately.
func ComputeTotals(lines []OrderLine) (units int, weightKg, volumeM3 float64) {
	for _, ln := range lines {
		units += ln.Quantity
		weightKg += ln.TotalWeightKg
		volumeM3 += ln.TotalVolumeM3
	}
	return units, weightKg, volumeM3
}

// TemperatureFor returns the header temperature requirement for a set of
// catalogue temperature requirements. The strictest wins: FROZEN over CHILLED
// over AMBIENT. Only Fresh carries chilled/frozen today, but the rule is
// temperature-driven so it holds if that ever changes.
func TemperatureFor(reqs []domain.TempRequirement) domain.TempRequirement {
	result := domain.TempAmbient
	for _, r := range reqs {
		switch r {
		case domain.TempFrozen:
			return domain.TempFrozen
		case domain.TempChilled:
			result = domain.TempChilled
		}
	}
	return result
}
