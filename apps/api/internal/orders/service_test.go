package orders

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"waypoint.lk/api/internal/catalog"
	"waypoint.lk/api/internal/domain"
)

// --- fakes -----------------------------------------------------------------

type fakeRepo struct {
	created   []Order
	byID      map[string]Order
	statuses  map[string]string
	createErr error
	// lastCount is the filter the last Count call received, so a test can
	// check the total was scoped like the page.
	lastCount  Filter
	closedRows map[string]bool
	closedBy   string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[string]Order{}, statuses: map[string]string{}}
}

func (f *fakeRepo) Create(_ context.Context, o Order) (Order, error) {
	if f.createErr != nil {
		return Order{}, f.createErr
	}
	o.OrderID = "o" + itoa(len(f.created)+1)
	o.OrderNumber = "ORD-2026-00000" + itoa(len(f.created)+1)
	f.created = append(f.created, o)
	f.byID[o.OrderID] = o
	return o, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (Order, error) {
	o, ok := f.byID[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	return o, nil
}

func (f *fakeRepo) GetByNumber(_ context.Context, n string) (Order, error) {
	for _, o := range f.byID {
		if o.OrderNumber == n {
			return o, nil
		}
	}
	return Order{}, ErrNotFound
}

func (f *fakeRepo) List(_ context.Context, filter Filter) ([]Order, error) {
	out := f.matching(filter)
	sort.Slice(out, func(i, j int) bool { return out[i].OrderNumber < out[j].OrderNumber })
	start := min(filter.Offset, len(out))
	end := len(out)
	if filter.Limit > 0 {
		end = min(start+filter.Limit, len(out))
	}
	return out[start:end], nil
}

func (f *fakeRepo) Count(_ context.Context, filter Filter) (int, error) {
	f.lastCount = filter
	return len(f.matching(filter)), nil
}

func (f *fakeRepo) matching(filter Filter) []Order {
	out := make([]Order, 0)
	for _, o := range f.byID {
		if filter.OutletID != "" && o.OutletID != filter.OutletID {
			continue
		}
		if filter.Status != "" && string(o.Status) != filter.Status {
			continue
		}
		if filter.Brand != "" && string(o.Brand) != filter.Brand {
			continue
		}
		out = append(out, o)
	}
	return out
}

func (f *fakeRepo) UpdateStatus(_ context.Context, id string, status string) (Order, error) {
	o, ok := f.byID[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	o.Status = domain.OrderStatus(status)
	f.byID[id] = o
	f.statuses[id] = status
	return o, nil
}

// closed stores the closure rows keyed by date|depot|brand.
func (f *fakeRepo) CloseQueue(_ context.Context, date time.Time, depotID string, brands []string, actor string) (int, error) {
	if f.closedRows == nil {
		f.closedRows = map[string]bool{}
	}
	f.closedBy = actor
	n := 0
	for _, b := range brands {
		k := date.Format("2006-01-02") + "|" + depotID + "|" + b
		if f.closedRows[k] {
			continue
		}
		f.closedRows[k] = true
		n++
	}
	return n, nil
}

func (f *fakeRepo) ClosedBrands(_ context.Context, date time.Time, depotID string) ([]string, error) {
	out := make([]string, 0)
	for _, b := range []string{"FRESH", "STYLE", "TECH"} {
		if f.closedRows[date.Format("2006-01-02")+"|"+depotID+"|"+b] {
			out = append(out, b)
		}
	}
	return out, nil
}

type fakeCatalogue struct {
	items   map[string]catalog.Item
	failErr error
}

func (f fakeCatalogue) ResolveMany(_ context.Context, ids []string) (map[string]catalog.Item, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	out := map[string]catalog.Item{}
	for _, id := range ids {
		if it, ok := f.items[id]; ok {
			out[id] = it
		} else {
			return nil, errors.New("catalog: not found: " + id)
		}
	}
	return out, nil
}

type fakeOutlets struct {
	outlets map[string]Outlet
}

func (f fakeOutlets) GetOutlet(_ context.Context, id string) (Outlet, error) {
	o, ok := f.outlets[id]
	if !ok {
		return Outlet{}, ErrNotFound
	}
	return o, nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// fixture builds a service over a fresh-catalogue outlet/item pair.
func fixture(now time.Time) (*Service, *fakeRepo, *fakeCatalogue) {
	freshItem := catalog.Item{
		ItemID: "i-fresh", SKU: "FRESH-1", Brand: domain.BrandFresh,
		UnitWeightKg: 2.0, UnitVolumeM3: 0.01, TemperatureRequirement: domain.TempChilled,
	}
	ambientItem := catalog.Item{
		ItemID: "i-amb", SKU: "FRESH-2", Brand: domain.BrandFresh,
		UnitWeightKg: 1.0, UnitVolumeM3: 0.005, TemperatureRequirement: domain.TempAmbient,
	}
	styleItem := catalog.Item{
		ItemID: "i-style", SKU: "STYLE-1", Brand: domain.BrandStyle,
		UnitWeightKg: 3.0, UnitVolumeM3: 0.02, TemperatureRequirement: domain.TempAmbient,
	}
	cat := &fakeCatalogue{items: map[string]catalog.Item{
		"i-fresh": freshItem, "i-amb": ambientItem, "i-style": styleItem,
	}}
	outlets := fakeOutlets{outlets: map[string]Outlet{
		"OUT001": {OutletID: "OUT001", Brand: domain.BrandFresh, DepotID: "d1"},
		"OUT050": {OutletID: "OUT050", Brand: domain.BrandStyle, DepotID: "d1"},
	}}
	repo := newFakeRepo()
	return NewService(repo, cat, outlets, fixedClock{t: now}), repo, cat
}

// --- tests -----------------------------------------------------------------

func TestServiceCreate(t *testing.T) {
	// 10:00 — before the cutoff.
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)

	t.Run("computes snapshots and totals", func(t *testing.T) {
		svc, repo, _ := fixture(before)
		o, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 10}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if o.TotalUnits != 10 || o.TotalWeightKg != 10.0 {
			t.Fatalf("totals = %d units, %v kg", o.TotalUnits, o.TotalWeightKg)
		}
		if o.Status != domain.OrderPlaced {
			t.Fatalf("status = %s, want PLACED", o.Status)
		}
		if o.Brand != domain.BrandFresh {
			t.Fatalf("brand = %s, want FRESH (from the outlet)", o.Brand)
		}
		if o.AfterCutoff {
			t.Fatal("10:00 is before the cutoff")
		}
		if len(repo.created) != 1 || repo.created[0].Lines[0].UnitWeightKgSnapshot != 1.0 {
			t.Fatalf("snapshot not persisted: %+v", repo.created)
		}
	})

	t.Run("chilled line sets the header temperature", func(t *testing.T) {
		svc, _, _ := fixture(before)
		o, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines: []LineRequest{
				{ItemID: "i-amb", Quantity: 1},
				{ItemID: "i-fresh", Quantity: 1},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if o.TempRequirement != domain.TempChilled {
			t.Fatalf("temp = %s, want CHILLED", o.TempRequirement)
		}
	})

	t.Run("after the cutoff is accepted but flagged", func(t *testing.T) {
		after := time.Date(2026, time.September, 25, 16, 5, 0, 0, time.UTC)
		svc, _, _ := fixture(after)
		o, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 27, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 1}},
		})
		if err != nil {
			t.Fatalf("post-cutoff order must be accepted, got %v", err)
		}
		if !o.AfterCutoff {
			t.Fatal("16:05 should set after_cutoff")
		}
	})

	t.Run("exactly 16:00 is after the cutoff", func(t *testing.T) {
		exact := time.Date(2026, time.September, 25, 16, 0, 0, 0, time.UTC)
		svc, _, _ := fixture(exact)
		o, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 27, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !o.AfterCutoff {
			t.Fatal("16:00 exactly should count as after the cutoff")
		}
	})

	t.Run("brand mismatch is rejected", func(t *testing.T) {
		svc, _, _ := fixture(before)
		_, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001", // Fresh outlet
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "i-style", Quantity: 1}},
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("unknown item is rejected", func(t *testing.T) {
		svc, _, _ := fixture(before)
		_, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "ghost", Quantity: 1}},
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("unknown outlet is rejected", func(t *testing.T) {
		svc, _, _ := fixture(before)
		_, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT999",
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 1}},
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("no lines is rejected", func(t *testing.T) {
		svc, _, _ := fixture(before)
		_, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("zero quantity is rejected", func(t *testing.T) {
		svc, _, _ := fixture(before)
		_, err := svc.Create(context.Background(), CreateInput{
			OutletID:              "OUT001",
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 0}},
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})
}

func TestServiceGetScoping(t *testing.T) {
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	svc, repo, _ := fixture(before)
	created, err := svc.Create(context.Background(), CreateInput{
		OutletID:              "OUT001",
		RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
		Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = repo

	dispatcher := Scope{Role: domain.RoleDispatcher}
	ownStore := Scope{Role: domain.RoleStoreManager, OutletID: "OUT001"}
	otherStore := Scope{Role: domain.RoleStoreManager, OutletID: "OUT014"}

	if _, err := svc.Get(context.Background(), created.OrderID, dispatcher); err != nil {
		t.Fatalf("dispatcher should read: %v", err)
	}
	if _, err := svc.Get(context.Background(), created.OrderID, ownStore); err != nil {
		t.Fatalf("own store manager should read: %v", err)
	}
	if _, err := svc.Get(context.Background(), created.OrderID, otherStore); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other outlet should be hidden as not found, got %v", err)
	}
	if _, err := svc.Get(context.Background(), "missing", dispatcher); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing order = %v, want ErrNotFound", err)
	}
}

func TestServiceListScoping(t *testing.T) {
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	svc, _, _ := fixture(before)
	for _, outlet := range []string{"OUT001", "OUT050"} {
		if _, err := svc.Create(context.Background(), CreateInput{
			OutletID:              outlet,
			RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
			Lines:                 []LineRequest{{ItemID: singleItemFor(outlet), Quantity: 1}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	dispatcher := Scope{Role: domain.RoleDispatcher}
	all, err := svc.List(context.Background(), Filter{}, dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("dispatcher sees %d orders, want 2", len(all))
	}

	store := Scope{Role: domain.RoleStoreManager, OutletID: "OUT001"}
	own, err := svc.List(context.Background(), Filter{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(own) != 1 || own[0].OutletID != "OUT001" {
		t.Fatalf("store manager sees %+v, want only OUT001", own)
	}

	if _, err := svc.List(context.Background(), Filter{OutletID: "OUT050"}, store); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-outlet filter = %v, want ErrNotFound", err)
	}
	if _, err := svc.List(context.Background(), Filter{Status: "PLANNED"}, dispatcher); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown status filter = %v, want ErrInvalid", err)
	}
}

func singleItemFor(outlet string) string {
	if outlet == "OUT050" {
		return "i-style"
	}
	return "i-amb"
}

func TestServiceConfirm(t *testing.T) {
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	svc, repo, _ := fixture(before)
	o, err := svc.Create(context.Background(), CreateInput{
		OutletID:              "OUT001",
		RequestedDeliveryDate: time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
		Lines:                 []LineRequest{{ItemID: "i-amb", Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := Scope{Role: domain.RoleDispatcher}

	confirmed, err := svc.Confirm(context.Background(), o.OrderID, dispatcher)
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if confirmed.Status != domain.OrderConfirmed {
		t.Fatalf("status = %s, want CONFIRMED", confirmed.Status)
	}
	if repo.statuses[o.OrderID] != "CONFIRMED" {
		t.Fatalf("status not persisted: %v", repo.statuses)
	}

	// Confirming again is a conflict.
	if _, err := svc.Confirm(context.Background(), o.OrderID, dispatcher); !errors.Is(err, ErrConflict) {
		t.Fatalf("re-confirm = %v, want ErrConflict", err)
	}
}

func TestServiceConfirmNotFound(t *testing.T) {
	before := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	svc, _, _ := fixture(before)
	_, err := svc.Confirm(context.Background(), "missing", Scope{Role: domain.RoleDispatcher})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestServiceCloseQueuePerBrandIdempotent(t *testing.T) {
	svc, repo, _ := fixture(time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC))
	dispatcher := Scope{Role: domain.RoleDispatcher}
	date := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)

	first, err := svc.CloseQueue(context.Background(), CloseQueueInput{
		Date: date, DepotID: "d1", Brands: []domain.Brand{domain.BrandFresh}, Actor: "u-disp",
	}, dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	if first.Closed != 1 || len(first.AlreadyClosed) != 0 {
		t.Fatalf("first close = %+v", first)
	}
	if repo.closedBy != "u-disp" {
		t.Fatalf("actor not recorded: %q", repo.closedBy)
	}

	// A repeat close of the same brand writes nothing and is not an error.
	second, err := svc.CloseQueue(context.Background(), CloseQueueInput{
		Date: date, DepotID: "d1", Brands: []domain.Brand{domain.BrandFresh},
	}, dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	if second.Closed != 0 || len(second.AlreadyClosed) != 1 || second.AlreadyClosed[0] != "FRESH" {
		t.Fatalf("repeat close = %+v", second)
	}

	// Closing all brands closes the two that were still open.
	all, err := svc.CloseQueue(context.Background(), CloseQueueInput{Date: date, DepotID: "d1"}, dispatcher)
	if err != nil {
		t.Fatal(err)
	}
	if all.Closed != 2 {
		t.Fatalf("closing all should close the 2 remaining brands, got %d", all.Closed)
	}
	closed, _ := repo.ClosedBrands(context.Background(), date, "d1")
	if len(closed) != 3 {
		t.Fatalf("closed brands = %v, want 3", closed)
	}
}

func TestServiceCloseQueueRejectsBadInput(t *testing.T) {
	svc, _, _ := fixture(time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC))
	date := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)
	dispatcher := Scope{Role: domain.RoleDispatcher}

	// Missing date.
	if _, err := svc.CloseQueue(context.Background(), CloseQueueInput{DepotID: "d1"}, dispatcher); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing date = %v, want ErrInvalid", err)
	}
	// Missing depot.
	if _, err := svc.CloseQueue(context.Background(), CloseQueueInput{Date: date}, dispatcher); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing depot = %v, want ErrInvalid", err)
	}
	// Unknown brand.
	if _, err := svc.CloseQueue(context.Background(), CloseQueueInput{Date: date, DepotID: "d1", Brands: []domain.Brand{"GROCERY"}}, dispatcher); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad brand = %v, want ErrInvalid", err)
	}
	// Non-dispatcher.
	if _, err := svc.CloseQueue(context.Background(), CloseQueueInput{Date: date, DepotID: "d1"}, Scope{Role: domain.RoleLoader, DepotID: "d1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("loader close = %v, want ErrInvalid", err)
	}
}
