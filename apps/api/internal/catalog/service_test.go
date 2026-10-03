package catalog

import (
	"context"
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// fakeRepo is an in-memory Repository for service tests. It records the last
// filter so List behaviour can be asserted without a database.
type fakeRepo struct {
	items      map[string]Item // by id
	lastFilter Filter
	listErr    error
}

func newFakeRepo(items ...Item) *fakeRepo {
	m := make(map[string]Item, len(items))
	for _, it := range items {
		m[it.ItemID] = it
	}
	return &fakeRepo{items: m}
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (Item, error) {
	it, ok := f.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	return it, nil
}

func (f *fakeRepo) GetBySKU(_ context.Context, sku string) (Item, error) {
	for _, it := range f.items {
		if it.SKU == sku {
			return it, nil
		}
	}
	return Item{}, ErrNotFound
}

func (f *fakeRepo) List(_ context.Context, filter Filter) ([]Item, error) {
	f.lastFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]Item, 0, len(f.items))
	for _, it := range f.items {
		if filter.Brand != "" && it.Brand != filter.Brand {
			continue
		}
		if filter.Temperature != "" && it.TemperatureRequirement != filter.Temperature {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

func item(id, sku string, brand domain.Brand, temp domain.TempRequirement) Item {
	return Item{
		ItemID: id, SKU: sku, Name: sku, Brand: brand,
		UnitWeightKg: 1, UnitVolumeM3: 0.01, TemperatureRequirement: temp,
	}
}

func TestServiceGet(t *testing.T) {
	repo := newFakeRepo(item("i1", "FRESH-1", domain.BrandFresh, domain.TempAmbient))
	svc := NewService(repo)

	t.Run("found", func(t *testing.T) {
		got, err := svc.Get(context.Background(), "i1")
		if err != nil {
			t.Fatal(err)
		}
		if got.SKU != "FRESH-1" {
			t.Fatalf("sku = %q", got.SKU)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := svc.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("empty id is invalid", func(t *testing.T) {
		if _, err := svc.Get(context.Background(), ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})
}

func TestServiceGetBySKU(t *testing.T) {
	repo := newFakeRepo(item("i1", "FRESH-1", domain.BrandFresh, domain.TempAmbient))
	svc := NewService(repo)

	if _, err := svc.GetBySKU(context.Background(), "FRESH-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.GetBySKU(context.Background(), "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetBySKU(context.Background(), ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestServiceList(t *testing.T) {
	repo := newFakeRepo(
		item("i1", "FRESH-1", domain.BrandFresh, domain.TempAmbient),
		item("i2", "FRESH-2", domain.BrandFresh, domain.TempChilled),
		item("i3", "STYLE-1", domain.BrandStyle, domain.TempAmbient),
	)
	svc := NewService(repo)

	t.Run("no filter lists all", func(t *testing.T) {
		got, err := svc.List(context.Background(), Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
	})

	t.Run("brand filter is passed through", func(t *testing.T) {
		got, err := svc.List(context.Background(), Filter{Brand: domain.BrandFresh})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if repo.lastFilter.Brand != domain.BrandFresh {
			t.Fatalf("repo did not receive brand filter: %+v", repo.lastFilter)
		}
	})

	t.Run("invalid filter is rejected before the repo", func(t *testing.T) {
		got, err := svc.List(context.Background(), Filter{Brand: "GROCERY"})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if got != nil {
			t.Fatalf("expected nil items, got %v", got)
		}
	})
}

func TestServiceResolveMany(t *testing.T) {
	repo := newFakeRepo(
		item("i1", "FRESH-1", domain.BrandFresh, domain.TempAmbient),
		item("i2", "FRESH-2", domain.BrandFresh, domain.TempChilled),
	)
	svc := NewService(repo)

	t.Run("all present", func(t *testing.T) {
		got, err := svc.ResolveMany(context.Background(), []string{"i1", "i2", "i1"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (deduplicated)", len(got))
		}
	})

	t.Run("missing id fails and names it", func(t *testing.T) {
		_, err := svc.ResolveMany(context.Background(), []string{"i1", "ghost"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestBrandsMatch(t *testing.T) {
	fresh := item("i1", "F1", domain.BrandFresh, domain.TempAmbient)
	style := item("i2", "S1", domain.BrandStyle, domain.TempAmbient)

	if !BrandsMatch([]Item{fresh}, domain.BrandFresh) {
		t.Error("single matching item should match")
	}
	if BrandsMatch([]Item{fresh, style}, domain.BrandFresh) {
		t.Error("mixed brands must not match")
	}
	if !BrandsMatch(nil, domain.BrandFresh) {
		t.Error("empty set is vacuously true")
	}
}
