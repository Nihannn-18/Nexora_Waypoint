package routes

import (
	"context"
	"errors"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/planning"
)

// --- fakes -----------------------------------------------------------------

type fakePlanning struct {
	job       planning.Job
	proposals []planning.Proposal
	err       error
}

func (f fakePlanning) Job(context.Context, string) (planning.Job, error) { return f.job, f.err }
func (f fakePlanning) Results(context.Context, string) (planning.Job, []planning.Proposal, error) {
	return f.job, f.proposals, f.err
}

type fakeOrders struct{ facts map[string]OrderFacts }

func (f fakeOrders) LoadOrderFacts(_ context.Context, id string) (OrderFacts, error) {
	fc, ok := f.facts[id]
	if !ok {
		return OrderFacts{}, ErrNotFound
	}
	return fc, nil
}

type fakeVehicles struct{ depots map[string]string }

func (f fakeVehicles) VehicleDepot(_ context.Context, id string) (string, error) {
	d, ok := f.depots[id]
	if !ok {
		return "", ErrNotFound
	}
	return d, nil
}

type fakeRefs struct {
	outlets   map[string]OutletRef
	travel    map[string]TravelRef
	allowance int
}

func (f fakeRefs) OutletFor(_ context.Context, id string) (OutletRef, error) {
	o, ok := f.outlets[id]
	if !ok {
		return OutletRef{}, ErrNotFound
	}
	return o, nil
}
func (f fakeRefs) Travel(_ context.Context, depotID, district string) (TravelRef, error) {
	t, ok := f.travel[depotID+"|"+district]
	if !ok {
		return TravelRef{}, ErrNotFound
	}
	return t, nil
}
func (f fakeRefs) ServiceAllowance(context.Context, domain.Brand, domain.DockType) (int, error) {
	return f.allowance, nil
}

type fakeRepo struct {
	confirmed *ConfirmationPlan
	result    ConfirmationResult
	err       error
	routes    map[string]Route
	deferrals []DeferralEntry
}

func (f *fakeRepo) GetRoute(_ context.Context, id string) (Route, error) {
	r, ok := f.routes[id]
	if !ok {
		return Route{}, ErrNotFound
	}
	return r, nil
}
func (f *fakeRepo) ListRoutes(context.Context, string, string) ([]Route, error) { return nil, nil }
func (f *fakeRepo) ListDeferrals(context.Context, string) ([]DeferralEntry, error) {
	return f.deferrals, nil
}
func (f *fakeRepo) Confirm(_ context.Context, plan ConfirmationPlan) (ConfirmationResult, error) {
	if f.err != nil {
		return ConfirmationResult{}, f.err
	}
	p := plan
	f.confirmed = &p
	return f.result, nil
}

func fixture() (*Confirmation, *fakeRepo) {
	job := planning.Job{JobID: "j1", Status: planning.JobCompleted, DepotID: "d-peli",
		PlanningDate: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)}
	proposals := []planning.Proposal{
		{OrderID: "O1", Decision: "SERVE", VehicleID: "VEH014", TripNo: 1, Seq: 0},
		{OrderID: "O2", Decision: "SERVE", VehicleID: "VEH014", TripNo: 1, Seq: 1},
		{OrderID: "O3", Decision: "DEFER"},
	}
	orders := fakeOrders{facts: map[string]OrderFacts{
		"O1": {OrderID: "O1", OutletID: "OUT001", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed, TotalWeightKg: 100, TotalVolumeM3: 1},
		"O2": {OrderID: "O2", OutletID: "OUT002", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed, TotalWeightKg: 50, TotalVolumeM3: 0.5},
		"O3": {OrderID: "O3", OutletID: "OUT001", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed},
	}}
	vehicles := fakeVehicles{depots: map[string]string{"VEH014": "d-peli"}}
	refs := fakeRefs{
		outlets:   map[string]OutletRef{"OUT001": {OutletID: "OUT001", DockType: domain.DockStreet}, "OUT002": {OutletID: "OUT002", DockType: domain.DockRearDock}},
		travel:    map[string]TravelRef{"d-peli|Colombo": {DepotToDistrictKm: 12, DepotToDistrictMin: 24, InterStopKm: 4, InterStopMin: 8}},
		allowance: 15,
	}
	repo := &fakeRepo{routes: map[string]Route{}, result: ConfirmationResult{RouteIDs: []string{"r1"}, AllocatedOrders: []string{"O1", "O2"}, DeferredOrders: []string{"O3"}}}
	svc := NewConfirmation(repo, fakePlanning{job: job, proposals: proposals}, orders, vehicles, refs, fixedClock{})
	return svc, repo
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 9, 26, 6, 0, 0, 0, time.UTC) }

func confirmInput() ConfirmInput {
	return ConfirmInput{
		JobID: "j1", Actor: "u-disp",
		Routes:    []RouteChoice{{VehicleID: "VEH014", TripNo: 1, OrderIDs: []string{"O1", "O2"}}},
		Deferrals: []DeferralChoice{{OrderID: "O3", ReasonType: "CONSTRAINT", ConstraintCode: domain.ConstraintFreshTimeBudget, ReasonText: "no budget"}},
	}
}

// --- tests -----------------------------------------------------------------

func TestConfirmBuildsRoutesLegsAndMetrics(t *testing.T) {
	svc, repo := fixture()
	res, err := svc.Confirm(context.Background(), confirmInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RouteIDs) != 1 {
		t.Fatalf("route ids = %v", res.RouteIDs)
	}
	plan := repo.confirmed
	if plan == nil || len(plan.Routes) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	route := plan.Routes[0]
	if route.Status != RouteConfirmed || route.Brand != domain.BrandFresh || route.District != "Colombo" {
		t.Fatalf("route = %+v", route)
	}
	if route.TotalWeightKg != 150 || route.TotalVolumeM3 != 1.5 {
		t.Fatalf("totals = %v kg, %v m3", route.TotalWeightKg, route.TotalVolumeM3)
	}
	// Official formula: outbound 24 + inter-stop 8*1 + handling 15*2 = 62.
	if route.OutboundMin != 24 || route.InterStopMin != 8 || route.HandlingMin != 30 || route.TotalTripMin != 62 {
		t.Fatalf("trip metrics = %+v", route)
	}
	if route.DistanceKm != 16 {
		t.Fatalf("distance = %v, want 16", route.DistanceKm)
	}
	if len(route.Legs) != 2 || route.Legs[0].FromPoint != "DEPOT" || route.Legs[1].FromPoint != "OUT001" {
		t.Fatalf("legs = %+v", route.Legs)
	}
	if len(plan.Deferrals) != 1 || plan.Deferrals[0].OrderID != "O3" {
		t.Fatalf("deferrals = %+v", plan.Deferrals)
	}
}

func TestConfirmRejectsIncompletePlan(t *testing.T) {
	svc, _ := fixture()
	in := confirmInput()
	in.Routes[0].OrderIDs = []string{"O1"} // O2 omitted, not deferred
	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid (an order neither allocated nor deferred)", err)
	}
}

func TestConfirmRejectsBadJobStatus(t *testing.T) {
	svc, repo := fixture()
	svc.planning = fakePlanning{job: planning.Job{JobID: "j1", Status: planning.JobRunning, DepotID: "d-peli", PlanningDate: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)}}
	_, err := svc.Confirm(context.Background(), confirmInput())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	_ = repo
}

func TestConfirmRejectsOrderNotInProposal(t *testing.T) {
	svc, _ := fixture()
	in := confirmInput()
	in.Routes[0].OrderIDs = []string{"O1", "GHOST"}
	in.Deferrals = nil
	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestConfirmRejectsDeferralWithoutReason(t *testing.T) {
	svc, _ := fixture()
	in := confirmInput()
	in.Deferrals[0].ReasonText = ""
	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestConfirmRejectsWrongVehicle(t *testing.T) {
	svc, _ := fixture()
	in := confirmInput()
	in.Routes[0].VehicleID = "VEH999" // not at the depot
	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestConfirmRejectsNextConfirmedStatus(t *testing.T) {
	svc, _ := fixture()
	svc.orders = fakeOrders{facts: map[string]OrderFacts{
		"O1": {OrderID: "O1", OutletID: "OUT001", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderReceived},
		"O2": {OrderID: "O2", OutletID: "OUT002", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed},
		"O3": {OrderID: "O3", OutletID: "OUT001", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed},
	}}
	_, err := svc.Confirm(context.Background(), confirmInput())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestConfirmRejectsMixedBrandDistrict(t *testing.T) {
	svc, _ := fixture()
	svc.orders = fakeOrders{facts: map[string]OrderFacts{
		"O1": {OrderID: "O1", OutletID: "OUT001", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed},
		"O2": {OrderID: "O2", OutletID: "OUT002", District: "Gampaha", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed},
		"O3": {OrderID: "O3", OutletID: "OUT001", District: "Colombo", DepotID: "d-peli", Brand: domain.BrandFresh, Status: domain.OrderConfirmed},
	}}
	_, err := svc.Confirm(context.Background(), confirmInput())
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestConfirmRejectsOrderAllocatedAndDeferred(t *testing.T) {
	svc, _ := fixture()
	in := confirmInput()
	in.Deferrals = append(in.Deferrals, DeferralChoice{OrderID: "O1", ReasonType: "CONSTRAINT", ReasonText: "x"})
	_, err := svc.Confirm(context.Background(), in)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
