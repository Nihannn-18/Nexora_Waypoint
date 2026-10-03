package audit

import (
	"context"
	"errors"
	"testing"

	"waypoint.lk/api/internal/domain"
)

type fakeRepo struct {
	records []Record
	filter  Filter
	err     error
}

func (f *fakeRepo) List(_ context.Context, filter Filter) ([]Record, error) {
	f.filter = filter
	return f.records, f.err
}

func (f *fakeRepo) WithTx(_ context.Context, fn func(context.Context, Recorder) error) error {
	return fn(context.Background(), nopRecorder{})
}

type nopRecorder struct{}

func (nopRecorder) Record(context.Context, Event) error { return nil }

func TestServiceListDispatcherOnly(t *testing.T) {
	svc := NewService(&fakeRepo{})
	ctx := context.Background()

	if _, err := svc.List(ctx, Identity{Role: domain.RoleLoader}, Filter{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("loader err = %v, want ErrInvalid", err)
	}
	if _, err := svc.List(ctx, Identity{Role: domain.RoleDriver}, Filter{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("driver err = %v, want ErrInvalid", err)
	}
	if _, err := svc.List(ctx, Identity{Role: domain.RoleStoreManager}, Filter{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("store err = %v, want ErrInvalid", err)
	}
	if _, err := svc.List(ctx, Identity{Role: domain.RoleDispatcher}, Filter{}); err != nil {
		t.Fatalf("dispatcher err = %v, want nil", err)
	}
}

func TestServiceListLimitClamp(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	ctx := context.Background()

	_, _ = svc.List(ctx, Identity{Role: domain.RoleDispatcher}, Filter{})
	if repo.filter.Limit != defaultLimit {
		t.Fatalf("default limit = %d, want %d", repo.filter.Limit, defaultLimit)
	}
	_, _ = svc.List(ctx, Identity{Role: domain.RoleDispatcher}, Filter{Limit: 10000})
	if repo.filter.Limit != maxLimit {
		t.Fatalf("clamped limit = %d, want %d", repo.filter.Limit, maxLimit)
	}
	_, _ = svc.List(ctx, Identity{Role: domain.RoleDispatcher}, Filter{Limit: 10, Offset: -5})
	if repo.filter.Offset != 0 {
		t.Fatalf("negative offset = %d, want 0", repo.filter.Offset)
	}
}

func TestServiceListDateRange(t *testing.T) {
	svc := NewService(&fakeRepo{})
	ctx := context.Background()
	from := mustTime(t, "2026-09-26T00:00:00Z")
	to := mustTime(t, "2026-09-25T00:00:00Z")
	if _, err := svc.List(ctx, Identity{Role: domain.RoleDispatcher}, Filter{From: from, To: to}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("to<from err = %v, want ErrInvalid", err)
	}
}
