package loading

import (
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

func TestValidateUpdate(t *testing.T) {
	tests := []struct {
		name    string
		update  LineUpdate
		ordered int
		wantErr bool
		field   string
	}{
		{"full load", LineUpdate{OrderItemID: "OI1", LoadedQty: 10}, 10, false, ""},
		{"partial load with missing", LineUpdate{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2}, 10, false, ""},
		{"damaged counts", LineUpdate{OrderItemID: "OI1", LoadedQty: 8, DamagedQty: 2}, 10, false, ""},
		{"all missing", LineUpdate{OrderItemID: "OI1", MissingQty: 10}, 10, false, ""},
		{"sum below ordered", LineUpdate{OrderItemID: "OI1", LoadedQty: 7}, 10, true, "quantity"},
		{"sum above ordered", LineUpdate{OrderItemID: "OI1", LoadedQty: 11}, 10, true, "quantity"},
		{"overload rejected", LineUpdate{OrderItemID: "OI1", LoadedQty: 10, MissingQty: 1}, 10, true, "quantity"},
		{"negative loaded", LineUpdate{OrderItemID: "OI1", LoadedQty: -1, MissingQty: 11}, 10, true, "quantity"},
		{"negative missing", LineUpdate{OrderItemID: "OI1", LoadedQty: 10, MissingQty: -1}, 10, true, "quantity"},
		{"missing order item", LineUpdate{LoadedQty: 10}, 10, true, "orderItemId"},
		{"valid shortfall photo", LineUpdate{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2, PhotoRef: "shortfall/OI1/abc123"}, 10, false, ""},
		{"photo for wrong item", LineUpdate{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2, PhotoRef: "shortfall/OI2/abc"}, 10, true, "photoRef"},
		{"photo wrong purpose", LineUpdate{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2, PhotoRef: "pod/LEG1/abc"}, 10, true, "photoRef"},
		{"photo traversal", LineUpdate{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2, PhotoRef: "shortfall/OI1/../x"}, 10, true, "photoRef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdate(tt.update, tt.ordered)
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

func TestValidShortfallPhoto(t *testing.T) {
	if !ValidShortfallPhoto("OI1", "") {
		t.Fatal("empty photo is optional and valid")
	}
	if !ValidShortfallPhoto("OI1", "shortfall/OI1/abc") {
		t.Fatal("scoped key should be valid")
	}
	if ValidShortfallPhoto("OI1", "shortfall/OI2/abc") {
		t.Fatal("key for another order item must be rejected")
	}
	if ValidShortfallPhoto("OI1", "shortfall/OI1/") {
		t.Fatal("empty object id must be rejected")
	}
	if ValidShortfallPhoto("OI1", "shortfall/OI1/a/b") {
		t.Fatal("nested key must be rejected")
	}
}

func TestLineHelpers(t *testing.T) {
	l := Line{OrderedQty: 10, LoadedQty: 8, DamagedQty: 1, MissingQty: 1}
	if l.ShortfallQty() != 2 {
		t.Fatalf("shortfall = %d, want 2", l.ShortfallQty())
	}
	if !l.Complete() {
		t.Fatal("8+1+1=10 should be complete")
	}
	if !l.HasShortfall() {
		t.Fatal("should have a shortfall")
	}
	l.LoadedQty = 9
	if l.Complete() {
		t.Fatal("9+1+1=11 must not be complete")
	}
}

func TestRouteLoadingReady(t *testing.T) {
	ready := RouteLoading{Lines: []Line{{OrderedQty: 5, LoadedQty: 5}, {OrderedQty: 3, LoadedQty: 1, MissingQty: 2}}}
	if !ready.Ready() {
		t.Fatal("all lines reconcile, ready should be true")
	}
	notReady := RouteLoading{Lines: []Line{{OrderedQty: 5, LoadedQty: 4}}}
	if notReady.Ready() {
		t.Fatal("unreconciled line means not ready")
	}
	if (RouteLoading{}).Ready() {
		t.Fatal("a route with no lines is not ready")
	}
}

var _ = domain.RoleLoader
