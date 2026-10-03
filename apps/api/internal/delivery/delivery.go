// Package delivery owns the driver's delivery execution: recording an outcome
// at a route stop, capturing proof of delivery, and the idempotent reconciliation
// of offline-captured events.
//
// Boundaries:
//   - Upstream: internal/routes produced the confirmed route, legs and
//     allocations; internal/loading recorded the load state. Delivery reads the
//     operational route/leg/order state; it does not create it.
//   - Downstream: store-manager receipt/GRN is a later concern; this package
//     stops at the delivery event, POD and the order/leg status it implies.
//
// delivery_event is the authoritative delivery record, mirroring the schema:
// one row per driver outcome, keyed for idempotency by client_event_id (generated
// on the device before any network attempt). The event carries the POD
// references (receiver name, signature, photo), never image bytes.
package delivery

import (
	"errors"
	"fmt"
	"strings"

	"waypoint.lk/api/internal/domain"
)

// Outcomes, mirroring delivery_event.outcome's CHECK and DELIVERY_OUTCOMES in
// libs/shared-types. DELAYED is a delivery outcome, not a terminal order state.
const (
	OutcomeDelivered = "DELIVERED"
	OutcomeFailed    = "FAILED"
	OutcomeDelayed   = "DELAYED"
)

// POD types, mirroring POD_TYPES in libs/shared-types.
const (
	PodPhoto     = "PHOTO"
	PodSignature = "SIGNATURE"
	PodNone      = "NONE"
)

// Sync reconciliation results, mirroring SYNC_RESULTS in libs/shared-types.
const (
	SyncAccepted  = "ACCEPTED"
	SyncDuplicate = "DUPLICATE"
	SyncRejected  = "REJECTED"
	SyncConflict  = "CONFLICT"
)

// ValidOutcome reports whether o is a canonical delivery outcome.
func ValidOutcome(o string) bool {
	switch o {
	case OutcomeDelivered, OutcomeFailed, OutcomeDelayed:
		return true
	}
	return false
}

// ValidPodType reports whether t is a canonical POD type.
func ValidPodType(t string) bool { return t == PodPhoto || t == PodSignature || t == PodNone }

// Pod is the proof of delivery attached to an event. Fields map to the
// delivery_event POD columns. A POD is optional for FAILED/DELAYED and required
// (with a receiver name) for DELIVERED.
type Pod struct {
	Type         string
	ReceiverName string
	SignatureRef string
	PhotoRef     string
}

// ItemDelivery is one order line's delivered/damaged/short quantities for an
// outcome. It mirrors order_item_delivery. delivered + damaged + short must
// reconcile against the ordered quantity when supplied.
type ItemDelivery struct {
	OrderItemID  string
	DeliveredQty int
	DamagedQty   int
	ShortQty     int
}

// EventInput is one driver outcome to record. ClientEventID is required and is
// the idempotency key; OccurredAt is the device clock at capture, preserved
// through sync.
type EventInput struct {
	LegID          string
	ClientEventID  string
	Outcome        string
	OccurredAt     string // RFC 3339; the device capture time
	CreatedOffline bool
	Items          []ItemDelivery
	ReasonCode     string
	Notes          string
	Pod            Pod
}

// Event is a stored delivery event, returned to the client.
type Event struct {
	EventID        string
	LegID          string
	Outcome        string
	ClientEventID  string
	OccurredAt     string
	SyncedAt       string
	CreatedOffline bool
	Notes          string
	Pod            Pod
	Items          []ItemDelivery
}

// EventResult is the outcome of reconciling one event (single or in a batch).
type EventResult struct {
	ClientEventID string
	Status        string // ACCEPTED, DUPLICATE, REJECTED, CONFLICT
	ServerEventID string
	// Reason is a short, non-sensitive explanation for REJECTED/CONFLICT.
	Reason string
}

// Sentinel errors. Handlers map these onto the shared HTTP error contract.
var (
	// ErrNotFound means no leg/event matches the lookup.
	ErrNotFound = errors.New("delivery: not found")
	// ErrInvalid means the event failed validation.
	ErrInvalid = errors.New("delivery: invalid input")
	// ErrConflict means the leg changed underneath the event (reallocated).
	ErrConflict = errors.New("delivery: conflict")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// PhotoPrefixFor returns the required prefix of a POD media key for a leg:
// "pod/<legID>/". Mirrors media.KeyFor with PurposePOD and the leg as owner.
func PhotoPrefixFor(legID string) string { return "pod/" + legID + "/" }

// ValidPodRef reports whether ref is a server-generated key of the given POD
// purpose for this leg. An empty ref is valid when the POD does not use that
// artefact.
func ValidPodRef(legID, ref string) bool {
	if ref == "" {
		return true
	}
	prefix := PhotoPrefixFor(legID)
	if !strings.HasPrefix(ref, prefix) {
		return false
	}
	rest := strings.TrimPrefix(ref, prefix)
	return rest != "" && !strings.Contains(rest, "/") && !strings.Contains(rest, "..")
}

// Validate checks one event against the business rules that do not need the
// database:
//
//   - the leg and a client event id are present;
//   - the outcome is a canonical value;
//   - an occurredAt is present (the device capture time);
//   - DELIVERED requires a receiver name and a POD artefact (photo or
//     signature), exactly as delivery_event's CHECK and the POD contract state;
//   - POD references are pod/<legID>/ keys (photo and signature), never another
//     purpose or leg;
//   - item quantities are non-negative.
//
// Ordered-quantity reconciliation for items is enforced in the repository, which
// reads the authoritative order line.
func ValidateEvent(e EventInput) error {
	if strings.TrimSpace(e.LegID) == "" {
		return ValidationError{Field: "legId", Message: "is required"}
	}
	if strings.TrimSpace(e.ClientEventID) == "" {
		return ValidationError{Field: "clientEventId", Message: "is required"}
	}
	if !ValidOutcome(e.Outcome) {
		return ValidationError{Field: "outcome", Message: "must be DELIVERED, FAILED or DELAYED"}
	}
	if strings.TrimSpace(e.OccurredAt) == "" {
		return ValidationError{Field: "occurredAt", Message: "is required"}
	}
	if !ValidPodType(normalizePodType(e.Pod.Type)) {
		return ValidationError{Field: "proofOfDelivery.type", Message: "must be PHOTO, SIGNATURE or NONE"}
	}
	if e.Pod.PhotoRef != "" && !ValidPodRef(e.LegID, e.Pod.PhotoRef) {
		return ValidationError{Field: "proofOfDelivery.fileRef", Message: "must be a POD media key for this leg"}
	}
	if e.Pod.SignatureRef != "" && !strings.HasPrefix(e.Pod.SignatureRef, PhotoPrefixFor(e.LegID)) {
		return ValidationError{Field: "proofOfDelivery.signature", Message: "must be a POD media key for this leg"}
	}
	for i, it := range e.Items {
		if strings.TrimSpace(it.OrderItemID) == "" {
			return ValidationError{Field: fmt.Sprintf("deliveredItems[%d].orderItemId", i), Message: "is required"}
		}
		if it.DeliveredQty < 0 || it.DamagedQty < 0 || it.ShortQty < 0 {
			return ValidationError{Field: fmt.Sprintf("deliveredItems[%d].quantity", i), Message: "quantities must not be negative"}
		}
	}

	if e.Outcome == OutcomeDelivered {
		if strings.TrimSpace(e.Pod.ReceiverName) == "" {
			return ValidationError{Field: "proofOfDelivery.receiverName", Message: "a delivery requires the receiver's name"}
		}
		if !hasPodArtefact(e.Pod) {
			return ValidationError{Field: "proofOfDelivery", Message: "a delivery requires a signature or photo"}
		}
	}
	return nil
}

// normalizePodType maps an unset type to NONE so a body without a POD is valid
// for FAILED/DELAYED.
func normalizePodType(t string) string {
	if strings.TrimSpace(t) == "" {
		return PodNone
	}
	return strings.ToUpper(strings.TrimSpace(t))
}

// hasPodArtefact reports whether the POD carries a signature or photo.
func hasPodArtefact(p Pod) bool { return p.PhotoRef != "" || p.SignatureRef != "" }

// NormalizePodType is the exported form used by the service and tests.
func NormalizePodType(t string) string { return normalizePodType(t) }

// OrderStatusForOutcome maps a delivery outcome to the order status it implies.
// DELIVERED and DELAYED both leave the order delivered (a delay is late, not
// unserved); FAILED marks the order failed, which the deferral flow may pick up.
// The mapping is derived from ORDER_STATUS_TRANSITIONS in libs/shared-types.
func OrderStatusForOutcome(outcome string) domain.OrderStatus {
	if outcome == OutcomeFailed {
		return domain.OrderFailed
	}
	return domain.OrderDelivered
}
