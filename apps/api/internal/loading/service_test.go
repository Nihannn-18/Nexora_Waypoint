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
