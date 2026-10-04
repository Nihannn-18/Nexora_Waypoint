package loading

import (
	"context"
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// fakeRepo is an in-memory Repository for service tests.
type fakeRepo struct {
	loading   RouteLoading
	recorded  *[]LineUpdate
	recordErr error
	loadErr   error
	summaries []RouteSummary
	listedFor struct{ depotID, date string }
}

func (f *fakeRepo) RoutesForDepot(_ context.Context, depotID, date string) ([]RouteSummary, error) {
	f.listedFor.depotID, f.listedFor.date = depotID, date
	return f.summaries, nil
}

func (f *fakeRepo) RouteLoading(context.Context, string) (RouteLoading, error) {
	if f.loadErr != nil {
		return RouteLoading{}, f.loadErr
	}
	return f.loading, nil
}

func (f *fakeRepo) RecordShortfalls(_ context.Context, _ string, _ string, updates []LineUpdate) (RouteLoading, error) {
	if f.recordErr != nil {
		return RouteLoading{}, f.recordErr
	}
	u := updates
	f.recorded = &u
	return f.loading, nil
}

func serviceFixture() (*Service, *fakeRepo) {
	repo := &fakeRepo{loading: RouteLoading{
		RouteID: "R1", DepotID: "d-peli", Status: "CONFIRMED",
		Lines: []Line{{OrderItemID: "OI1", OrderedQty: 10}},
	}}
	return NewService(repo), repo
}

func TestServicePickingListScope(t *testing.T) {
	svc, _ := serviceFixture()

	if _, err := svc.PickingList(context.Background(), "R1", "d-peli"); err != nil {
		t.Fatalf("own depot: %v", err)
	}
	if _, err := svc.PickingList(context.Background(), "R1", "d-kandy"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other depot = %v, want ErrNotFound", err)
	}
	if _, err := svc.PickingList(context.Background(), "R1", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no depot = %v, want ErrNotFound (fail closed)", err)
	}
	if _, err := svc.PickingList(context.Background(), "", "d-peli"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty route = %v, want ErrInvalid", err)
	}
}

func TestServiceRecordShortfalls(t *testing.T) {
	svc, repo := serviceFixture()
	ctx := context.Background()

	t.Run("valid submission", func(t *testing.T) {
		_, err := svc.RecordShortfalls(ctx, "R1", "d-peli", "u-loader",
			[]LineUpdate{{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2}})
		if err != nil {
			t.Fatal(err)
		}
		if repo.recorded == nil {
			t.Fatal("repository was not called")
		}
	})

	t.Run("empty submission rejected", func(t *testing.T) {
		if _, err := svc.RecordShortfalls(ctx, "R1", "d-peli", "u", nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("duplicate line rejected", func(t *testing.T) {
		_, err := svc.RecordShortfalls(ctx, "R1", "d-peli", "u", []LineUpdate{
			{OrderItemID: "OI1", LoadedQty: 5, MissingQty: 5},
			{OrderItemID: "OI1", LoadedQty: 5, MissingQty: 5},
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("negative rejected", func(t *testing.T) {
		if _, err := svc.RecordShortfalls(ctx, "R1", "d-peli", "u", []LineUpdate{{OrderItemID: "OI1", LoadedQty: -1}}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("cross-depot rejected", func(t *testing.T) {
		if _, err := svc.RecordShortfalls(ctx, "R1", "d-kandy", "u", []LineUpdate{{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2}}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("bad photo rejected", func(t *testing.T) {
		_, err := svc.RecordShortfalls(ctx, "R1", "d-peli", "u", []LineUpdate{{OrderItemID: "OI1", LoadedQty: 8, MissingQty: 2, PhotoRef: "pod/x/y"}})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})
}

var _ = domain.RoleLoader

// A loader with no depot must see nothing rather than every depot's work: the
// loader endpoints fail closed, and a route list is the one place where a
// missing scope would otherwise leak the whole network.
func TestServiceRoutesWithoutDepotReturnsNothing(t *testing.T) {
	svc, repo := serviceFixture()
	repo.summaries = []RouteSummary{{RouteID: "R1"}}

	got, err := svc.Routes(context.Background(), "", "2026-09-26")
	if err != nil {
		t.Fatalf("Routes() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Routes() returned %d routes for an unscoped caller, want 0", len(got))
	}
	if repo.listedFor.depotID != "" {
		t.Errorf("repository was queried for depot %q; it should not be reached", repo.listedFor.depotID)
	}
}

func TestServiceRoutesRejectsBadDate(t *testing.T) {
	svc, _ := serviceFixture()
	for _, date := range []string{"", "26-09-2026", "2026-9-26", "tomorrow"} {
		if _, err := svc.Routes(context.Background(), "d-peli", date); !errors.Is(err, ErrInvalid) {
			t.Errorf("Routes(date=%q) error = %v, want ErrInvalid", date, err)
		}
	}
}

func TestServiceRoutesPassesScopeToRepository(t *testing.T) {
	svc, repo := serviceFixture()
	repo.summaries = []RouteSummary{{RouteID: "R1", Lines: 2, LinesComplete: 2}}

	got, err := svc.Routes(context.Background(), "d-peli", "2026-09-26")
	if err != nil {
		t.Fatalf("Routes() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Routes() returned %d routes, want 1", len(got))
	}
	if repo.listedFor.depotID != "d-peli" || repo.listedFor.date != "2026-09-26" {
		t.Errorf("repository queried for (%q, %q), want (d-peli, 2026-09-26)",
			repo.listedFor.depotID, repo.listedFor.date)
	}
}
