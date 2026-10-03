package orders

import (
	"errors"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
)

func testDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func validOrder() Order {
	return Order{
		OrderID:               "o1",
		OrderNumber:           "ORD-2026-000001",
		OutletID:              "OUT001",
		Brand:                 domain.BrandFresh,
		OrderDate:             testDate(2026, time.September, 25),
		RequestedDeliveryDate: testDate(2026, time.September, 26),
		TempRequirement:       domain.TempAmbient,
		Status:                domain.OrderPlaced,
		Lines:                 []OrderLine{{ItemID: "i1", Quantity: 2}},
	}
}

func TestOrderValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Order)
		wantErr bool
		field   string
	}{
		{"valid", func(*Order) {}, false, ""},
		{"missing outlet", func(o *Order) { o.OutletID = "" }, true, "outletId"},
		{"unknown brand", func(o *Order) { o.Brand = "GROCERY" }, true, "brand"},
		{"unknown status", func(o *Order) { o.Status = "PLANNED" }, true, "status"},
		{"unknown temperature", func(o *Order) { o.TempRequirement = "WARM" }, true, "temperatureRequirement"},
		{"missing order date", func(o *Order) { o.OrderDate = time.Time{} }, true, "orderDate"},
		{"missing delivery date", func(o *Order) { o.RequestedDeliveryDate = time.Time{} }, true, "requestedDeliveryDate"},
		{"delivery before order date", func(o *Order) { o.RequestedDeliveryDate = testDate(2026, time.September, 24) }, true, "requestedDeliveryDate"},
		{"no lines", func(o *Order) { o.Lines = nil }, true, "lines"},
		{"line without item", func(o *Order) { o.Lines = []OrderLine{{ItemID: "", Quantity: 1}} }, true, "lines[0].itemId"},
		{"line with zero quantity", func(o *Order) { o.Lines = []OrderLine{{ItemID: "i1", Quantity: 0}} }, true, "lines[0].quantity"},
		{"line with negative quantity", func(o *Order) { o.Lines = []OrderLine{{ItemID: "i1", Quantity: -3}} }, true, "lines[0].quantity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := validOrder()
			tt.mutate(&o)
			err := o.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
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

func TestValidStatus(t *testing.T) {
	for _, s := range OrderStatusValues {
		if !ValidStatus(s) {
			t.Errorf("%s should be valid", s)
		}
	}
	for _, s := range []string{"", "PLANNED", "CANCELLED", "placed"} {
		if ValidStatus(domain.OrderStatus(s)) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to domain.OrderStatus
		want     bool
	}{
		{domain.OrderPlaced, domain.OrderConfirmed, true},
		{domain.OrderPlaced, domain.OrderDeferred, true},
		{domain.OrderConfirmed, domain.OrderAllocated, true},
		{domain.OrderAllocated, domain.OrderLoaded, true},
		{domain.OrderLoaded, domain.OrderInTransit, true},
		{domain.OrderInTransit, domain.OrderDelivered, true},
		{domain.OrderInTransit, domain.OrderFailed, true},
		{domain.OrderDelivered, domain.OrderReceived, true},
		{domain.OrderDeferred, domain.OrderConfirmed, true},
		{domain.OrderPlaced, domain.OrderLoaded, false},
		{domain.OrderReceived, domain.OrderConfirmed, false},
		{domain.OrderConfirmed, domain.OrderPlaced, false},
		{domain.OrderFailed, domain.OrderConfirmed, false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestComputeTotals(t *testing.T) {
	lines := []OrderLine{
		{Quantity: 2, TotalWeightKg: 3.5, TotalVolumeM3: 0.02},
		{Quantity: 5, TotalWeightKg: 10.0, TotalVolumeM3: 0.05},
	}
	units, weight, volume := ComputeTotals(lines)
	if units != 7 {
		t.Errorf("units = %d, want 7", units)
	}
	if weight != 13.5 {
		t.Errorf("weight = %v, want 13.5", weight)
	}
	if volume < 0.0699 || volume > 0.0701 {
		t.Errorf("volume = %v, want ~0.07", volume)
	}

	u, w, v := ComputeTotals(nil)
	if u != 0 || w != 0 || v != 0 {
		t.Errorf("empty totals = %d, %v, %v; want zeros", u, w, v)
	}
}

func TestTemperatureFor(t *testing.T) {
	tests := []struct {
		name string
		reqs []domain.TempRequirement
		want domain.TempRequirement
	}{
		{"none is ambient", nil, domain.TempAmbient},
		{"all ambient", []domain.TempRequirement{domain.TempAmbient, domain.TempAmbient}, domain.TempAmbient},
		{"any chilled wins", []domain.TempRequirement{domain.TempAmbient, domain.TempChilled}, domain.TempChilled},
		{"frozen wins", []domain.TempRequirement{domain.TempChilled, domain.TempFrozen}, domain.TempFrozen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TemperatureFor(tt.reqs); got != tt.want {
				t.Errorf("TemperatureFor = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValidateOrderNumber(t *testing.T) {
	tests := []struct {
		n    string
		want bool
	}{
		{"ORD-2026-000001", true},
		{"ORD-2026-999999", true},
		{"ORD-2026-1", false},
		{"ord-2026-000001", false},
		{"ORD-26-000001", false},
		{"2026-000001", false},
		{"ORD-2026-00000X", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidateOrderNumber(tt.n); got != tt.want {
			t.Errorf("ValidateOrderNumber(%q) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

func TestScopeCanRead(t *testing.T) {
	dispatcher := Scope{Role: domain.RoleDispatcher}
	store := Scope{Role: domain.RoleStoreManager, OutletID: "OUT014"}
	otherStore := Scope{Role: domain.RoleStoreManager, OutletID: "OUT015"}
	loader := Scope{Role: domain.RoleLoader, DepotID: "d1"}

	if !dispatcher.CanRead("OUT999", domain.BrandFresh) {
		t.Error("dispatcher should read any outlet")
	}
	if !store.CanRead("OUT014", domain.BrandFresh) {
		t.Error("store manager should read its own outlet")
	}
	if otherStore.CanRead("OUT014", domain.BrandFresh) {
		t.Error("store manager must not read another outlet")
	}
	if loader.CanRead("OUT014", domain.BrandFresh) {
		t.Error("loader order access is decided by route assignment, not here")
	}
}
