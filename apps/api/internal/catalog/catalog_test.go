package catalog

import (
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

func validItem() Item {
	return Item{
		ItemID:                 "11111111-1111-1111-1111-111111111111",
		SKU:                    "FRESH-0001",
		Name:                   "Red lentils 1kg",
		Brand:                  domain.BrandFresh,
		UnitWeightKg:           1.0,
		UnitVolumeM3:           0.0012,
		TemperatureRequirement: domain.TempAmbient,
	}
}

func TestItemValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Item)
		wantErr bool
		field   string
	}{
		{"valid", func(*Item) {}, false, ""},
		{"valid chilled", func(i *Item) { i.TemperatureRequirement = domain.TempChilled }, false, ""},
		{"valid frozen", func(i *Item) { i.TemperatureRequirement = domain.TempFrozen }, false, ""},
		{"zero weight is allowed by the schema", func(i *Item) { i.UnitWeightKg = 0 }, false, ""},
		{"zero volume is allowed by the schema", func(i *Item) { i.UnitVolumeM3 = 0 }, false, ""},
		{"empty sku", func(i *Item) { i.SKU = "" }, true, "sku"},
		{"blank sku", func(i *Item) { i.SKU = "   " }, true, "sku"},
		{"empty name", func(i *Item) { i.Name = "" }, true, "name"},
		{"unknown brand", func(i *Item) { i.Brand = domain.Brand("GROCERY") }, true, "brand"},
		{"missing brand", func(i *Item) { i.Brand = "" }, true, "brand"},
		{"unknown temperature", func(i *Item) { i.TemperatureRequirement = "WARM" }, true, "temperatureRequirement"},
		{"negative weight", func(i *Item) { i.UnitWeightKg = -0.5 }, true, "unitWeightKg"},
		{"negative volume", func(i *Item) { i.UnitVolumeM3 = -1 }, true, "unitVolumeM3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := validItem()
			tt.mutate(&item)
			err := item.Validate()
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

func TestItemRequiresReefer(t *testing.T) {
	tests := []struct {
		temp domain.TempRequirement
		want bool
	}{
		{domain.TempAmbient, false},
		{domain.TempChilled, true},
		{domain.TempFrozen, true},
	}
	for _, tt := range tests {
		item := validItem()
		item.TemperatureRequirement = tt.temp
		if got := item.RequiresReefer(); got != tt.want {
			t.Errorf("%s.RequiresReefer() = %v, want %v", tt.temp, got, tt.want)
		}
	}
}

func TestFilterValidate(t *testing.T) {
	tests := []struct {
		name    string
		filter  Filter
		wantErr bool
		field   string
	}{
		{"empty filter is valid", Filter{}, false, ""},
		{"valid brand", Filter{Brand: domain.BrandStyle}, false, ""},
		{"valid temperature", Filter{Temperature: domain.TempChilled}, false, ""},
		{"free-text search", Filter{Search: "lentil"}, false, ""},
		{"unknown brand", Filter{Brand: "GROCERY"}, true, "brand"},
		{"unknown temperature", Filter{Temperature: "COLD"}, true, "temperature"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
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
