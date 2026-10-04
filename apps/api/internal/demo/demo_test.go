package demo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/clock"
	"waypoint.lk/api/internal/domain"
)

func colombo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// manual is a base clock the test moves by hand.
type manual struct{ t time.Time }

func (m *manual) Now() time.Time { return m.t }

type fakeRepo struct {
	jumpErr  error
	resetErr error
	jumps    []Stage
	resets   int
	gotActor string
	gotDepot string
	gotTo    time.Time
}

func (f *fakeRepo) RecordClockJump(_ context.Context, actor, depotID string, stage Stage, _, to time.Time) error {
	if f.jumpErr != nil {
		return f.jumpErr
	}
	f.jumps = append(f.jumps, stage)
	f.gotActor, f.gotDepot, f.gotTo = actor, depotID, to
	return nil
}

func (f *fakeRepo) Reset(_ context.Context, actor, depotID string, clockTo time.Time) (ResetSummary, error) {
	if f.resetErr != nil {
		return ResetSummary{}, f.resetErr
	}
	f.resets++
	f.gotActor, f.gotDepot, f.gotTo = actor, depotID, clockTo
	return ResetSummary{Cleared: map[string]int64{"customer_order": 3}, DemoOrders: 85, DemoVehicleDays: 60}, nil
}

// fixture builds a demo clock at DEMO_CLOCK_START over a frozen base clock.
func fixture(t *testing.T) (*Service, *fakeRepo, *clock.Demo, time.Time) {
	t.Helper()
	loc := colombo(t)
	start := time.Date(2026, 9, 25, 15, 40, 0, 0, loc)
	clk := clock.NewDemo(start, &manual{t: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)})
	repo := &fakeRepo{}
	return NewService(clk, repo, loc, start), repo, clk, start
}

func TestStagesAreTheWalkthroughInstants(t *testing.T) {
	loc := colombo(t)
	want := map[Stage]string{
		StageBeforeCutoff: "2026-09-25T15:40:00+05:30",
		StageAfterCutoff:  "2026-09-25T16:05:00+05:30",
		StageLoading:      "2026-09-26T03:30:00+05:30",
		StageOnRoute:      "2026-09-26T05:00:00+05:30",
	}
	got := Stages()
	if len(got) != len(want) {
		t.Fatalf("Stages() = %v, want %d stages", got, len(want))
	}
	var prev time.Time
	for _, s := range got {
		at, ok := s.At(loc)
		if !ok {
			t.Fatalf("stage %s has no instant", s)
		}
		if at.Format(time.RFC3339) != want[s] {
			t.Errorf("%s at %s, want %s", s, at.Format(time.RFC3339), want[s])
		}
		if !prev.IsZero() && !at.After(prev) {
			t.Errorf("stages out of walkthrough order at %s", s)
		}
		prev = at
	}
	if _, ok := Stage("LUNCH").At(loc); ok {
		t.Error("unknown stage should not resolve")
	}
}

func TestJumpToMovesTheClockAndAuditsFirst(t *testing.T) {
	svc, repo, clk, _ := fixture(t)

	now, err := svc.JumpTo(context.Background(), "seed-dispatcher", "depot-1", " LOADING ")
	if err != nil {
		t.Fatalf("JumpTo: %v", err)
	}
	if got := now.Format(time.RFC3339); got != "2026-09-26T03:30:00+05:30" {
		t.Fatalf("now = %s, want Sat 03:30", got)
	}
	if !clk.Now().Equal(now) {
		t.Fatalf("API clock = %v, want %v", clk.Now(), now)
	}
	if len(repo.jumps) != 1 || repo.jumps[0] != StageLoading {
		t.Fatalf("audited jumps = %v, want [LOADING] (trimmed)", repo.jumps)
	}
	if repo.gotActor != "seed-dispatcher" || repo.gotDepot != "depot-1" {
		t.Fatalf("audit actor/depot = %q/%q", repo.gotActor, repo.gotDepot)
	}
}

func TestJumpToRefusesUnknownStageWithoutMoving(t *testing.T) {
	svc, repo, clk, start := fixture(t)
	if _, err := svc.JumpTo(context.Background(), "u", "", "TOMORROW"); !errors.Is(err, ErrUnknownStage) {
		t.Fatalf("err = %v, want ErrUnknownStage", err)
	}
	if !clk.Now().Equal(start) || len(repo.jumps) != 0 {
		t.Fatal("an unknown stage must neither move the clock nor be audited")
	}
}

func TestJumpToDoesNotMoveWhenAuditFails(t *testing.T) {
	svc, repo, clk, start := fixture(t)
	repo.jumpErr = errors.New("db down")
	if _, err := svc.JumpTo(context.Background(), "u", "", StageOnRoute); err == nil {
		t.Fatal("want the audit error")
	}
	if !clk.Now().Equal(start) {
		t.Fatalf("clock moved to %v despite the failed audit", clk.Now())
	}
}

func TestResetRestoresTheStartClockAfterCommit(t *testing.T) {
	svc, repo, clk, start := fixture(t)
	if _, err := svc.JumpTo(context.Background(), "u", "", StageOnRoute); err != nil {
		t.Fatal(err)
	}

	sum, now, err := svc.Reset(context.Background(), "seed-dispatcher", "depot-1")
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !now.Equal(start) || !clk.Now().Equal(start) {
		t.Fatalf("clock after reset = %v, want DEMO_CLOCK_START %v", clk.Now(), start)
	}
	if !repo.gotTo.Equal(start) || repo.resets != 1 || sum.DemoOrders != 85 {
		t.Fatalf("reset not delegated as expected: %+v %+v", repo, sum)
	}
}

func TestResetLeavesTheClockWhenTheDataResetFails(t *testing.T) {
	svc, repo, clk, _ := fixture(t)
	onRoute, err := svc.JumpTo(context.Background(), "u", "", StageOnRoute)
	if err != nil {
		t.Fatal(err)
	}
	repo.resetErr = errors.New("seed failed")
	if _, _, err := svc.Reset(context.Background(), "u", ""); err == nil {
		t.Fatal("want the reset error")
	}
	if !clk.Now().Equal(onRoute) {
		t.Fatalf("clock = %v, want it left at %v after a failed reset", clk.Now(), onRoute)
	}
}

// --- HTTP ------------------------------------------------------------------

type stubVerifier struct{ id auth.Identity }

func (v stubVerifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

func demoMux(t *testing.T, role domain.Role) (http.Handler, *fakeRepo, *clock.Demo) {
	t.Helper()
	svc, repo, clk, _ := fixture(t)
	id := auth.Identity{UserID: "u1", Role: role, DepotID: "depot-1"}
	mw := auth.NewMiddleware(auth.NewIdentityLoader(stubVerifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
	mux := http.NewServeMux()
	NewHandler(svc, mw, "Asia/Colombo").RegisterRoutes(mux)
	return mux, repo, clk
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	h.ServeHTTP(rec, req)
	return rec
}

func TestSetClockEndpoint(t *testing.T) {
	h, _, clk := demoMux(t, domain.RoleDispatcher)

	rec := post(h, "/api/v1/demo/clock", `{"stage":"AFTER_CUTOFF"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body clockResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Stage != "AFTER_CUTOFF" || body.Now != "2026-09-25T16:05:00+05:30" || !body.DemoMode || body.Timezone != "Asia/Colombo" {
		t.Fatalf("body = %+v", body)
	}
	if got := clk.Now().Format(time.RFC3339); got != body.Now {
		t.Fatalf("API clock = %s, want %s", got, body.Now)
	}
}

func TestSetClockValidation(t *testing.T) {
	h, repo, _ := demoMux(t, domain.RoleDispatcher)
	for name, body := range map[string]string{
		"missing stage": `{}`,
		"empty stage":   `{"stage":"  "}`,
		"unknown stage": `{"stage":"LUNCH"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := post(h, "/api/v1/demo/clock", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), `"field":"stage"`) {
				t.Fatalf("body should name the stage field: %s", rec.Body)
			}
		})
	}
	if rec := post(h, "/api/v1/demo/clock", `{"stage":"LOADING","extra":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: status = %d, want 400", rec.Code)
	}
	if len(repo.jumps) != 0 {
		t.Fatalf("invalid requests were audited: %v", repo.jumps)
	}
}

func TestDemoEndpointsAreDispatcherOnly(t *testing.T) {
	for _, role := range []domain.Role{domain.RoleLoader, domain.RoleDriver, domain.RoleStoreManager} {
		h, repo, _ := demoMux(t, role)
		if rec := post(h, "/api/v1/demo/clock", `{"stage":"LOADING"}`); rec.Code != http.StatusForbidden {
			t.Errorf("%s clock: status = %d, want 403", role, rec.Code)
		}
		if rec := post(h, "/api/v1/demo/reset", ``); rec.Code != http.StatusForbidden {
			t.Errorf("%s reset: status = %d, want 403", role, rec.Code)
		}
		if len(repo.jumps) != 0 || repo.resets != 0 {
			t.Errorf("%s reached the repository", role)
		}
	}
}

func TestResetEndpoint(t *testing.T) {
	h, repo, clk := demoMux(t, domain.RoleDispatcher)
	post(h, "/api/v1/demo/clock", `{"stage":"ON_ROUTE"}`)

	rec := post(h, "/api/v1/demo/reset", ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body resetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Now != "2026-09-25T15:40:00+05:30" || body.DemoOrders != 85 || body.Cleared["customer_order"] != 3 {
		t.Fatalf("body = %+v", body)
	}
	if clk.Now().Format(time.RFC3339) != body.Now || repo.gotActor != "u1" || repo.gotDepot != "depot-1" {
		t.Fatal("reset did not restore the clock or pass the caller through")
	}

	repo.resetErr = errors.New("boom")
	if rec := post(h, "/api/v1/demo/reset", ``); rec.Code != http.StatusInternalServerError {
		t.Fatalf("failed reset: status = %d, want 500", rec.Code)
	}
}
