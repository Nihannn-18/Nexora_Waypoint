// Package receipts owns the store manager's goods received note (GRN, S-06):
// confirming what arrived for a delivered order, line by line, and the issues
// that confirmation raises.
//
// Boundaries:
//   - Upstream: internal/loading recorded what left the dock per order line
//     (load_item), and internal/delivery recorded the driver's outcome and POD
//     (delivery_event). Receipts reads both; it never changes them.
//   - The order transition DELIVERED → RECEIVED happens here, in the same
//     transaction as the receipt, its audit row and any dispatcher notification.
//
// What the store is told to expect for a line is the loader's loaded count when
// the loader recorded one (a flagged shortfall never left the dock), otherwise
// the ordered quantity. The GRN compares what arrived against that, so a
// shortfall the loader already reported is pre-filled rather than reported a
// second time as a new issue.
package receipts

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Receipt statuses, mirroring RECEIPT_STATUSES in libs/shared-types and the
// receipt.status CHECK (migration 00011). The status is derived from the lines,
// never accepted from the client.
const (
	StatusReceived          = "RECEIVED"
	StatusReceivedWithIssue = "RECEIVED_WITH_ISSUE"
)

// Line conditions, mirroring RECEIPT_LINE_CONDITIONS in libs/shared-types.
const (
	ConditionGood            = "GOOD"
	ConditionDamaged         = "DAMAGED"
	ConditionShort           = "SHORT"
	ConditionDamagedAndShort = "DAMAGED_AND_SHORT"
)

// Issue types, mirroring ISSUE_TYPES in libs/shared-types.
const (
	IssueDamaged = "DAMAGED"
	IssueShort   = "SHORT"
)

// Where a line's expected quantity came from, mirroring
// RECEIPT_EXPECTED_SOURCES in libs/shared-types.
const (
	ExpectedFromOrder  = "ORDER"
	ExpectedFromLoader = "LOADER"
)

// maxNotesLength bounds the free-text note on a GRN.
const maxNotesLength = 500

// Sentinel errors. Handlers map these onto the shared HTTP error contract.
var (
	// ErrNotFound means no order matches, or the order is outside the caller's
	// scope (reported identically so scope cannot be probed).
	ErrNotFound = errors.New("receipts: not found")
	// ErrInvalid means the GRN failed validation.
	ErrInvalid = errors.New("receipts: invalid input")
	// ErrNotDelivered means the order has no delivery to receive yet.
	ErrNotDelivered = errors.New("receipts: order not delivered")
	// ErrAlreadyReceived means a GRN already exists for the order.
	ErrAlreadyReceived = errors.New("receipts: already received")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// LoaderFlag is the loader's shortfall on one order line (L-03), shown to the
// store before it counts.
type LoaderFlag struct {
	MissingQty int
	DamagedQty int
	PhotoRef   string
}

// ExpectedLine is one order line as the store should expect it.
type ExpectedLine struct {
	OrderItemID string
	SKU         string
	Name        string
	OrderedQty  int
	// LoadedQty is the loader's loaded count; nil when the loader recorded no
	// count for the line.
	LoadedQty  *int
	LoaderFlag *LoaderFlag
}

// ExpectedQty is the quantity that should have arrived: the loader's loaded
// count when one was recorded, otherwise the ordered quantity.
func (l ExpectedLine) ExpectedQty() int {
	if l.LoadedQty != nil {
		return *l.LoadedQty
	}
	return l.OrderedQty
}

// ExpectedSource names where ExpectedQty came from.
func (l ExpectedLine) ExpectedSource() string {
	if l.LoadedQty != nil {
		return ExpectedFromLoader
	}
	return ExpectedFromOrder
}

// OrderContext is the order a GRN is recorded against, with its lines.
type OrderContext struct {
	OrderID     string
	OrderNumber string
	OutletID    string
	DepotID     string
	Status      string
	Lines       []ExpectedLine
}

// Pod is the driver's proof of delivery for the order, attached to the GRN.
type Pod struct {
	Outcome      string
	ReceiverName string
	SignatureRef string
	PhotoRef     string
	OccurredAt   time.Time
}

// LineInput is the store's count for one line: what arrived in good condition
// and what arrived damaged. Short is derived.
type LineInput struct {
	OrderItemID string
	ReceivedQty int
	DamagedQty  int
}

// Input is a GRN submission.
type Input struct {
	Lines []LineInput
	Notes string
}

// Line is one recorded GRN line.
type Line struct {
	OrderItemID string
	SKU         string
	Name        string
	OrderedQty  int
	ExpectedQty int
	ReceivedQty int
	DamagedQty  int
}

// ShortQty is what was expected but did not arrive. Derived, never stored.
func (l Line) ShortQty() int { return l.ExpectedQty - l.ReceivedQty - l.DamagedQty }

// Condition summarises the line for the GRN.
func (l Line) Condition() string {
	damaged, short := l.DamagedQty > 0, l.ShortQty() > 0
	switch {
	case damaged && short:
		return ConditionDamagedAndShort
	case damaged:
		return ConditionDamaged
	case short:
		return ConditionShort
	default:
		return ConditionGood
	}
}

// Issue is one discrepancy the GRN raised against a line.
type Issue struct {
	Type        string
	OrderItemID string
	SKU         string
	Name        string
	Quantity    int
}

// Receipt is a recorded GRN.
type Receipt struct {
	ReceiptID      string
	OrderID        string
	ReceivedAt     time.Time
	ReceivedBy     string
	ReceivedByName string
	Notes          string
	Lines          []Line
}

// Status is RECEIVED when every line arrived as expected, otherwise
// RECEIVED_WITH_ISSUE.
func (r Receipt) Status() string {
	if len(r.Issues()) > 0 {
		return StatusReceivedWithIssue
	}
	return StatusReceived
}

// Issues lists every damaged and short quantity, in line order.
func (r Receipt) Issues() []Issue {
	var out []Issue
	for _, l := range r.Lines {
		if l.DamagedQty > 0 {
			out = append(out, Issue{Type: IssueDamaged, OrderItemID: l.OrderItemID, SKU: l.SKU, Name: l.Name, Quantity: l.DamagedQty})
		}
		if s := l.ShortQty(); s > 0 {
			out = append(out, Issue{Type: IssueShort, OrderItemID: l.OrderItemID, SKU: l.SKU, Name: l.Name, Quantity: s})
		}
	}
	return out
}

// View is the S-06 read model: the order's expected lines, the driver's POD
// and, once recorded, the GRN.
type View struct {
	Order   OrderContext
	Pod     *Pod
	Receipt *Receipt
}

// BuildLines validates a submission against the order and returns the GRN
// lines to record:
//
//   - every order line is counted exactly once, and nothing else is;
//   - quantities are not negative;
//   - received + damaged never exceeds what was expected to arrive — a store
//     cannot receive more than left the dock;
//   - the note is bounded.
func BuildLines(order OrderContext, in Input) ([]Line, error) {
	if len(strings.TrimSpace(in.Notes)) > maxNotesLength {
		return nil, ValidationError{Field: "notes", Message: fmt.Sprintf("must be %d characters or fewer", maxNotesLength)}
	}
	if len(in.Lines) == 0 {
		return nil, ValidationError{Field: "lines", Message: "count every line on the order"}
	}
	byID := make(map[string]LineInput, len(in.Lines))
	for i, l := range in.Lines {
		id := strings.TrimSpace(l.OrderItemID)
		if id == "" {
			return nil, ValidationError{Field: fmt.Sprintf("lines[%d].orderItemId", i), Message: "is required"}
		}
		if _, dup := byID[id]; dup {
			return nil, ValidationError{Field: fmt.Sprintf("lines[%d].orderItemId", i), Message: "is counted more than once"}
		}
		if l.ReceivedQty < 0 || l.DamagedQty < 0 {
			return nil, ValidationError{Field: fmt.Sprintf("lines[%d]", i), Message: "quantities must not be negative"}
		}
		byID[id] = l
	}

	lines := make([]Line, 0, len(order.Lines))
	for _, exp := range order.Lines {
		in, ok := byID[exp.OrderItemID]
		if !ok {
			return nil, ValidationError{Field: "lines", Message: fmt.Sprintf("%s has no count; count every line on the order", exp.SKU)}
		}
		delete(byID, exp.OrderItemID)
		expected := exp.ExpectedQty()
		if in.ReceivedQty+in.DamagedQty > expected {
			return nil, ValidationError{
				Field:   "lines." + exp.OrderItemID,
				Message: fmt.Sprintf("%s: received + damaged (%d) is more than the %d expected", exp.SKU, in.ReceivedQty+in.DamagedQty, expected),
			}
		}
		lines = append(lines, Line{
			OrderItemID: exp.OrderItemID, SKU: exp.SKU, Name: exp.Name,
			OrderedQty: exp.OrderedQty, ExpectedQty: expected,
			ReceivedQty: in.ReceivedQty, DamagedQty: in.DamagedQty,
		})
	}
	for id := range byID {
		return nil, ValidationError{Field: "lines", Message: fmt.Sprintf("%s is not a line on this order", id)}
	}
	return lines, nil
}
