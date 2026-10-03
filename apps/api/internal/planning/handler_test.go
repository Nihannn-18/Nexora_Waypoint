package planning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// --- fakes -----------------------------------------------------------------

type fakeRepo struct {
	jobs      map[string]Job
	proposals map[string][]Proposal
	seq       int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{jobs: map[string]Job{}, proposals: map[string][]Proposal{}}
}

func (f *fakeRepo) CreateJob(_ context.Context, job Job) (Job, error) {
	f.seq++
	job.JobID = fmt.Sprintf("PLAN-%04d", f.seq)
	job.Status = JobQueued
	job.CreatedAt = time.Now()
	f.jobs[job.JobID] = job
	return job, nil
}

func (f *fakeRepo) UpdateJobStatus(_ context.Context, jobID, status, errMsg string) (Job, error) {
	job, ok := f.jobs[jobID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	job.Status = status
	job.ErrorMessage = errMsg
	if status == JobCompleted || status == JobFailed {
		now := time.Now()
		job.CompletedAt = &now
	}
	f.jobs[jobID] = job
	return job, nil
}

func (f *fakeRepo) GetJob(_ context.Context, jobID string) (Job, error) {
	job, ok := f.jobs[jobID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	return job, nil
}

func (f *fakeRepo) SaveProposals(_ context.Context, jobID string, p []Proposal) error {
	f.proposals[jobID] = p
	return nil
}

func (f *fakeRepo) LoadProposals(_ context.Context, jobID string) ([]Proposal, error) {
	return f.proposals[jobID], nil
}

type fakeLoader struct {
	in  Input
	err error
}

func (f fakeLoader) LoadInput(context.Context, time.Time, string) (Input, error) {
	return f.in, f.err
}

// --- middleware / helpers --------------------------------------------------

type verifier struct{ id auth.Identity }

func (v verifier) Verify(context.Context, *auth.RequestHeader) (auth.Identity, error) {
	return v.id, nil
}

func middlewareFor(id auth.Identity) *auth.Middleware {
	return auth.NewMiddleware(auth.NewIdentityLoader(verifier{id: id}, auth.NewStaticUserStore(id)), auth.NewAuthorizer())
}

func dispatcher() auth.Identity {
	return auth.Identity{UserID: "u-disp", Role: domain.RoleDispatcher, DepotID: "d-peli"}
}

func storeManager() auth.Identity {
	return auth.Identity{UserID: "u-store", Role: domain.RoleStoreManager, OutletID: "OUT001"}
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf)))
	return rec
}

func getReq(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// --- tests -----------------------------------------------------------------

func TestServiceSuggestProducesProposals(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fakeLoader{in: engineInput()}, New())

	job, err := svc.Suggest(context.Background(), date(2026, 9, 26), "d-peli", "u-disp")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobCompleted {
		t.Fatalf("status = %s, want COMPLETED", job.Status)
	}
	props := repo.proposals[job.JobID]
	if len(props) == 0 {
		t.Fatal("expected proposals")
	}
	served, deferred := 0, 0
	for _, p := range props {
		switch p.Decision {
		case "SERVE":
			served++
		case "DEFER":
			deferred++
			if p.Constraint == "" {
				t.Fatalf("deferred order %s has no constraint", p.OrderID)
			}
		}
	}
	if served == 0 {
		t.Fatal("expected served proposals")
	}
	_ = deferred
}

func TestServiceLoadFailureMarksJobFailed(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fakeLoader{err: fmt.Errorf("boom")}, New())
	if _, err := svc.Suggest(context.Background(), date(2026, 9, 26), "d-peli", "u-disp"); err == nil {
		t.Fatal("expected an error")
	}
	for _, j := range repo.jobs {
		if j.Status != JobFailed {
			t.Fatalf("job status = %s, want FAILED", j.Status)
		}
	}
}

func TestServiceJobNotFound(t *testing.T) {
	svc := NewService(newFakeRepo(), fakeLoader{}, New())
	if _, err := svc.Job(context.Background(), "nope"); err != ErrJobNotFound {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestHandlerSuggestAndPoll(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fakeLoader{in: engineInput()}, New())
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(dispatcher())).RegisterRoutes(mux)

	t.Run("suggest returns 202 with a job id", func(t *testing.T) {
		rec := postJSON(t, mux, "/api/v1/allocations/suggest", map[string]any{
			"planningDate": "2026-09-26", "depotId": "d-peli",
		})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202 (body=%s)", rec.Code, rec.Body)
		}
		var body suggestResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.JobID == "" {
			t.Fatal("no jobId")
		}

		// Poll status.
		got := getReq(mux, "/api/v1/planning-jobs/"+body.JobID)
		if got.Code != http.StatusOK {
			t.Fatalf("job status = %d, want 200", got.Code)
		}
		// Results.
		res := getReq(mux, "/api/v1/planning-jobs/"+body.JobID+"/results")
		if res.Code != http.StatusOK {
			t.Fatalf("results status = %d, want 200", res.Code)
		}
		var results resultsResponse
		if err := json.Unmarshal(res.Body.Bytes(), &results); err != nil {
			t.Fatal(err)
		}
		if len(results.Proposals) == 0 {
			t.Fatal("expected proposals")
		}
	})

	t.Run("bad date is a 400", func(t *testing.T) {
		rec := postJSON(t, mux, "/api/v1/allocations/suggest", map[string]any{
			"planningDate": "26/09/2026", "depotId": "d-peli",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown job is 404", func(t *testing.T) {
		got := getReq(mux, "/api/v1/planning-jobs/nope")
		if got.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got.Code)
		}
	})
}

func TestHandlerRequiresDispatcher(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fakeLoader{in: engineInput()}, New())

	// A store manager must be forbidden.
	mux := http.NewServeMux()
	NewHandler(svc, middlewareFor(storeManager())).RegisterRoutes(mux)
	rec := postJSON(t, mux, "/api/v1/allocations/suggest", map[string]any{"planningDate": "2026-09-26", "depotId": "d-peli"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("store manager status = %d, want 403", rec.Code)
	}

	// The fail-closed verifier must refuse rather than serve.
	mux2 := http.NewServeMux()
	failClosed := auth.NewMiddleware(auth.NewIdentityLoader(auth.SessionTokenVerifier{}, nil), auth.NewAuthorizer())
	NewHandler(svc, failClosed).RegisterRoutes(mux2)
	if got := getReq(mux2, "/api/v1/planning-jobs/x"); got.Code == http.StatusOK {
		t.Fatal("planning served an unverifiable request")
	}
}

// ensure httpx is referenced (error shape) even if assertions change.
var _ = httpx.CodeNotFound
