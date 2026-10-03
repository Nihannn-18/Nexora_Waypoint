package delivery

import (
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

func validDelivered() EventInput {
	return EventInput{
		LegID: "LEG1", ClientEventID: "EV1", Outcome: OutcomeDelivered,
		OccurredAt: "2026-09-26T07:42:00+05:30",
		Pod:        Pod{Type: PodPhoto, ReceiverName: "Nimal", PhotoRef: "pod/LEG1/abc"},
	}
}

func TestValidateEvent(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*EventInput)
		wantErr bool
		field   string
	}{
		{"valid delivered with photo", func(*EventInput) {}, false, ""},
		{"valid delivered with signature", func(e *EventInput) { e.Pod = Pod{Type: PodSignature, ReceiverName: "N", SignatureRef: "pod/LEG1/sig"} }, false, ""},
		{"valid failed without pod", func(e *EventInput) { e.Outcome = OutcomeFailed; e.ReasonCode = "OUTLET_CLOSED"; e.Pod = Pod{} }, false, ""},
		{"failed without reason", func(e *EventInput) { e.Outcome = OutcomeFailed; e.Pod = Pod{} }, true, "reasonCode"},
		{"failed with unknown reason", func(e *EventInput) { e.Outcome = OutcomeFailed; e.ReasonCode = "ALIENS"; e.Pod = Pod{} }, true, "reasonCode"},
		{"valid delayed without pod", func(e *EventInput) { e.Outcome = OutcomeDelayed; e.Pod = Pod{} }, false, ""},
		{"missing leg", func(e *EventInput) { e.LegID = "" }, true, "legId"},
		{"missing client event id", func(e *EventInput) { e.ClientEventID = "" }, true, "clientEventId"},
		{"bad outcome", func(e *EventInput) { e.Outcome = "PARTIAL" }, true, "outcome"},
		{"missing occurredAt", func(e *EventInput) { e.OccurredAt = "" }, true, "occurredAt"},
		{"delivered without receiver", func(e *EventInput) { e.Pod.ReceiverName = "" }, true, "proofOfDelivery.receiverName"},
		{"delivered without artefact", func(e *EventInput) { e.Pod = Pod{Type: PodNone, ReceiverName: "N"} }, true, "proofOfDelivery"},
		{"pod photo wrong leg", func(e *EventInput) { e.Pod.PhotoRef = "pod/LEG2/abc" }, true, "proofOfDelivery.fileRef"},
		{"pod photo wrong purpose", func(e *EventInput) { e.Pod.PhotoRef = "shortfall/OI1/abc" }, true, "proofOfDelivery.fileRef"},
		{"pod photo traversal", func(e *EventInput) { e.Pod.PhotoRef = "pod/LEG1/../x" }, true, "proofOfDelivery.fileRef"},
		{"bad pod type", func(e *EventInput) { e.Pod.Type = "AUDIO" }, true, "proofOfDelivery.type"},
		{"negative item qty", func(e *EventInput) { e.Items = []ItemDelivery{{OrderItemID: "OI1", DeliveredQty: -1}} }, true, "deliveredItems[0].quantity"},
		{"item without id", func(e *EventInput) { e.Items = []ItemDelivery{{DeliveredQty: 1}} }, true, "deliveredItems[0].orderItemId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validDelivered()
			tt.mutate(&e)
			err := ValidateEvent(e)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("error %v does not wrap ErrInvalid", err)
				}
				var ve ValidationError
				if !errors.As(err, &ve) || ve.Field != tt.field {
					t.Fatalf("field = %q, want %q (err=%v)", ve.Field, tt.field, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidOutcome(t *testing.T) {
	for _, o := range []string{OutcomeDelivered, OutcomeFailed, OutcomeDelayed} {
		if !ValidOutcome(o) {
			t.Errorf("%s should be valid", o)
		}
	}
	for _, o := range []string{"", "PARTIAL", "delivered"} {
		if ValidOutcome(o) {
			t.Errorf("%q should be invalid", o)
		}
	}
}

func TestOrderStatusForOutcome(t *testing.T) {
	if OrderStatusForOutcome(OutcomeDelivered) != domain.OrderDelivered {
		t.Fatal("delivered maps to DELIVERED")
	}
	if OrderStatusForOutcome(OutcomeDelayed) != domain.OrderDelivered {
		t.Fatal("delayed is still a delivery")
	}
	if OrderStatusForOutcome(OutcomeFailed) != domain.OrderFailed {
		t.Fatal("failed maps to FAILED")
	}
}

func TestValidPodRef(t *testing.T) {
	if !ValidPodRef("LEG1", "") {
		t.Fatal("empty is valid")
	}
	if !ValidPodRef("LEG1", "pod/LEG1/abc") {
		t.Fatal("scoped key valid")
	}
	if ValidPodRef("LEG1", "pod/LEG2/abc") {
		t.Fatal("other leg rejected")
	}
	if ValidPodRef("LEG1", "pod/LEG1/") {
		t.Fatal("empty object id rejected")
	}
}
