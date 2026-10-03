package notify

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	created     []New
	listed      []Notification
	unread      int
	markErr     error
	markedAll   int
	dispatchers []string
}

func (f *fakeRepo) Create(_ context.Context, n New) error {
	f.created = append(f.created, n)
	return nil
}
func (f *fakeRepo) List(context.Context, Recipient, int, int) ([]Notification, error) {
	return f.listed, nil
}
func (f *fakeRepo) UnreadCount(context.Context, Recipient) (int, error) { return f.unread, nil }
func (f *fakeRepo) MarkRead(context.Context, Recipient, string) error   { return f.markErr }
func (f *fakeRepo) MarkAllRead(context.Context, Recipient) (int, error) { return f.markedAll, nil }
func (f *fakeRepo) DispatcherUserIDs(context.Context, string) ([]string, error) {
	return f.dispatchers, nil
}

func TestServiceListValidation(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.List(context.Background(), Recipient{}, 0, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestServiceMarkReadValidation(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if err := svc.MarkRead(context.Background(), Recipient{UserID: "u1"}, "  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestCreateIdempotentPayload(t *testing.T) {
	repo := &fakeRepo{}
	if err := repo.Create(context.Background(), New{UserID: "u1", Type: TypeShortfall, Title: "t", Reference: "ref1"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.created) != 1 || repo.created[0].Reference != "ref1" {
		t.Fatalf("created = %+v", repo.created)
	}
}

func TestRecipientFrom(t *testing.T) {
	r := RecipientFrom("u1", "DISPATCHER", "OUT1")
	if r.UserID != "u1" || r.OutletID != "OUT1" {
		t.Fatalf("recipient = %+v", r)
	}
}
