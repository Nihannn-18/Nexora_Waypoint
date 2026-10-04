package delivery

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo is an in-memory Repository for service tests.
type fakeRepo struct {
	leg        LegContext
	detail     LegDetail
	routes     []DriverRoute
	routesArg  [2]string
	activeDate string
	legErr     error
	results    map[string]EventResult
	recordErr  error
	recorded   *EventInput
	status     SyncStatus
}

func (f *fakeRepo) ActiveRouteDate(_ context.Context, depot string, _ time.Time) (string, error) {
	f.routesArg = [2]string{depot, ""}
	return f.activeDate, nil
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

func (f *fakeRepo) LegDetail(context.Context, string) (LegDetail, error) {
	if f.legErr != nil {
		return LegDetail{}, f.legErr
	}
	return f.detail, nil
}

func (f *fakeRepo) DriverRoutes(_ context.Context, depot, date string) ([]DriverRoute, error) {
	f.routesArg = [2]string{depot, date}
	return f.routes, nil
}

func (f *fakeRepo) SyncStatus(context.Context, string) (SyncStatus, error) { return f.status, nil }
func (f *fakeRepo) GetEvent(context.Context, string) (Event, error)        { return Event{}, ErrNotFound }

func fixture() (*Service, *fakeRepo) {
	repo := &fakeRepo{
		leg: LegContext{LegID: "LEG1", RouteID: "R1", DepotID: "d-peli", OrderIDs: []string{"O1"}},
		detail: LegDetail{
			LegContext: LegContext{LegID: "LEG1", RouteID: "R1", DepotID: "d-peli", RouteDate: "2026-09-26", ToOutlet: "OUT014", Status: "PENDING", OrderIDs: []string{"O1"}},
			Route:      RouteSummary{RouteID: "R1", RouteDate: "2026-09-26", VehicleID: "VEH014", TripNo: 1, Brand: "FRESH", District: "Colombo", Status: "DISPATCHED"},
			Outlet:     OutletInfo{OutletID: "OUT014", Name: "Fresh Colombo 14", WindowOpen: "05:00", WindowClose: "08:00"},
			Orders:     []StopOrder{{OrderID: "O1", OrderNumber: "S1-0001", Lines: []OrderLine{{OrderItemID: "OI1", SKU: "SKU1", Name: "Milk", Quantity: 10}}}},
		},
		results: map[string]EventResult{},
	}
	return NewService(repo, nil), repo
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

func validFailed(id string) EventInput {
	return EventInput{
		LegID: "LEG1", ClientEventID: id, Outcome: OutcomeFailed,
		OccurredAt: "2026-09-26T07:42:00+05:30", CreatedOffline: true, ReasonCode: "ACCESS_BLOCKED",
	}
}

func TestReasonCodeReachesRepository(t *testing.T) {
	svc, repo := fixture()
	ctx := context.Background()

	if _, err := svc.RecordOne(ctx, "u-driver", "d-peli", validFailed("EVF1")); err != nil {
		t.Fatal(err)
	}
	if repo.recorded == nil || repo.recorded.ReasonCode != "ACCESS_BLOCKED" {
		t.Fatalf("recorded = %+v, want reasonCode ACCESS_BLOCKED", repo.recorded)
	}

	// Offline batch: the reason survives sync too.
	res, err := svc.SyncBatch(ctx, "u-driver", "d-peli", []EventInput{validFailed("EVF2")})
	if err != nil || res[0].Status != SyncAccepted {
		t.Fatalf("sync = %+v, %v", res, err)
	}
	if repo.recorded.ClientEventID != "EVF2" || repo.recorded.ReasonCode != "ACCESS_BLOCKED" {
		t.Fatalf("synced = %+v", repo.recorded)
	}

	// DELAYED needs no reason and is unchanged.
	delayed := validFailed("EVD1")
	delayed.Outcome, delayed.ReasonCode = OutcomeDelayed, ""
	if _, err := svc.RecordOne(ctx, "u-driver", "d-peli", delayed); err != nil {
		t.Fatalf("delayed: %v", err)
	}
}

func TestLegDetailScope(t *testing.T) {
	svc, repo := fixture()
	ctx := context.Background()

	got, err := svc.LegDetail(ctx, "LEG1", "d-peli")
	if err != nil || got.Outlet.Name == "" || len(got.Orders) != 1 {
		t.Fatalf("own depot = %+v, %v", got, err)
	}
	for _, depot := range []string{"d-kandy", ""} {
		if _, err := svc.LegDetail(ctx, "LEG1", depot); !errors.Is(err, ErrNotFound) {
			t.Fatalf("depot %q = %v, want ErrNotFound", depot, err)
		}
	}
	repo.legErr = ErrNotFound
	if _, err := svc.LegDetail(ctx, "NOPE", "d-peli"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing leg = %v, want ErrNotFound", err)
	}
}

func TestDriverRoutesScope(t *testing.T) {
	svc, repo := fixture()
	ctx := context.Background()

	if _, err := svc.DriverRoutes(ctx, "u-driver", "d-peli", "26 Sep"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad date = %v, want ErrInvalid", err)
	}
	if _, err := svc.DriverRoutes(ctx, "u-driver", "d-peli", "2026-09-26"); err != nil || repo.routesArg != [2]string{"d-peli", "2026-09-26"} {
		t.Fatalf("own depot: args=%v err=%v", repo.routesArg, err)
	}
	repo.routesArg = [2]string{}
	got, err := svc.DriverRoutes(ctx, "u-driver", "", "2026-09-26")
	if err != nil || len(got) != 0 || repo.routesArg != [2]string{} {
		t.Fatalf("no depot must not query: got=%v args=%v err=%v", got, repo.routesArg, err)
	}
}

// fakeAssignments resolves a single fixed vehicle for a driver, or none.
type fakeAssignments struct {
	vehicleID string
	assigned  bool
}

func (f fakeAssignments) AssignedVehicle(context.Context, string, string) (string, bool, error) {
	return f.vehicleID, f.assigned, nil
}

// TestDriverRoutesNarrowedByAssignment proves a driver with an assignment only
// sees their own vehicle's routes, and that no assignment preserves the
// depot-wide fallback (explicit choice) rather than silently guessing.
func TestDriverRoutesNarrowedByAssignment(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{
		routes: []DriverRoute{
			{RouteSummary: RouteSummary{RouteID: "R1", RouteDate: "2026-09-26", VehicleID: "VEH014", TripNo: 1, Status: "DISPATCHED"}},
			{RouteSummary: RouteSummary{RouteID: "R2", RouteDate: "2026-09-26", VehicleID: "VEH001", TripNo: 1, Status: "DISPATCHED"}},
		},
		results: map[string]EventResult{},
	}

	assigned := NewService(repo, nil).WithAssignments(fakeAssignments{vehicleID: "VEH014", assigned: true})
	got, err := assigned.DriverRoutes(ctx, "u-driver", "d-peli", "2026-09-26")
	if err != nil {
		t.Fatalf("assigned: %v", err)
	}
	if len(got) != 1 || got[0].VehicleID != "VEH014" {
		t.Fatalf("assigned narrow = %+v, want only VEH014", got)
	}

	unassigned := NewService(repo, nil).WithAssignments(fakeAssignments{})
	got, err = unassigned.DriverRoutes(ctx, "u-driver", "d-peli", "2026-09-26")
	if err != nil {
		t.Fatalf("unassigned: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unassigned = %d routes, want the depot's 2 (explicit choice)", len(got))
	}
}

func TestSyncBatchEmpty(t *testing.T) {
	svc, _ := fixture()
	if _, err := svc.SyncBatch(context.Background(), "u", "d-peli", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
