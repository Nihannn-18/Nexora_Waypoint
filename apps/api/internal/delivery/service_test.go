package delivery

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is an in-memory Repository for service tests.
type fakeRepo struct {
	leg       LegContext
	legErr    error
	results   map[string]EventResult
	recordErr error
	recorded  *EventInput
	status    SyncStatus
}

func (f *fakeRepo) LegContext(context.Context, string) (LegContext, error) {
	if f.legErr != nil {
		return LegContext{}, f.legErr
	}
	return f.leg, nil
}

func (f *fakeRepo) Record(_ context.Context, _ string, in EventInput, _ LegContext) (EventResult, error) {
	if f.recordErr != nil {
		return EventResult{}, f.recordErr
	}
	u := in
	f.recorded = &u
	if r, ok := f.results[in.ClientEventID]; ok {
		return r, nil
	}
	return EventResult{ClientEventID: in.ClientEventID, Status: SyncAccepted, ServerEventID: "E1"}, nil
}

func (f *fakeRepo) SyncStatus(context.Context, string) (SyncStatus, error) { return f.status, nil }
func (f *fakeRepo) GetEvent(context.Context, string) (Event, error)        { return Event{}, ErrNotFound }

func fixture() (*Service, *fakeRepo) {
	repo := &fakeRepo{
		leg:     LegContext{LegID: "LEG1", RouteID: "R1", DepotID: "d-peli", OrderIDs: []string{"O1"}},
		results: map[string]EventResult{},
	}
	return NewService(repo), repo
}

func TestRecordOneScope(t *testing.T) {
	svc, _ := fixture()
	ctx := context.Background()

	if _, err := svc.RecordOne(ctx, "u-driver", "d-peli", validDelivered()); err != nil {
		t.Fatalf("own depot: %v", err)
	}
	if _, err := svc.RecordOne(ctx, "u-driver", "d-kandy", validDelivered()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other depot = %v, want ErrNotFound", err)
	}
	if _, err := svc.RecordOne(ctx, "u-driver", "", validDelivered()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no depot = %v, want ErrNotFound (fail closed)", err)
	}
	bad := validDelivered()
	bad.Outcome = "PARTIAL"
	if _, err := svc.RecordOne(ctx, "u-driver", "d-peli", bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad outcome = %v, want ErrInvalid", err)
	}
}

func TestRecordOneDuplicate(t *testing.T) {
	svc, repo := fixture()
	repo.results["EV1"] = EventResult{ClientEventID: "EV1", Status: SyncDuplicate, ServerEventID: "E9"}
	res, err := svc.RecordOne(context.Background(), "u-driver", "d-peli", validDelivered())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != SyncDuplicate || res.ServerEventID != "E9" {
		t.Fatalf("res = %+v", res)
	}
}

func TestSyncBatchIndependent(t *testing.T) {
	svc, repo := fixture()
	repo.results["EV2"] = EventResult{ClientEventID: "EV2", Status: SyncDuplicate, ServerEventID: "E2"}

	good := validDelivered() // EV1 -> ACCEPTED
	dup := validDelivered()  // EV2 -> DUPLICATE
	dup.ClientEventID = "EV2"
	bad := validDelivered() // EV3 -> REJECTED
	bad.ClientEventID = "EV3"
	bad.Outcome = "PARTIAL"

	results, err := svc.SyncBatch(context.Background(), "u-driver", "d-peli", []EventInput{good, dup, bad})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].Status != SyncAccepted || results[1].Status != SyncDuplicate || results[2].Status != SyncRejected {
		t.Fatalf("statuses = %+v", results)
	}
	if results[2].Reason == "" {
		t.Fatal("rejected event should carry a reason")
	}
}

func TestSyncBatchEmpty(t *testing.T) {
	svc, _ := fixture()
	if _, err := svc.SyncBatch(context.Background(), "u", "d-peli", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
