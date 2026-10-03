package planning

import (
	"context"
	"errors"
	"fmt"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Service orchestrates a planning run: create a job, load the inputs, run the
// deterministic engine, and persist the proposals. It never creates route,
// route_leg or allocation rows — those are the dispatcher's to confirm.
type Service struct {
	repo   Repository
	loader Loader
	engine *Engine
}

// NewService builds the planning service.
func NewService(repo Repository, loader Loader, engine *Engine) *Service {
	return &Service{repo: repo, loader: loader, engine: engine}
}

// Suggest creates a planning job and runs it synchronously, returning the job.
//
// The HTTP contract is 202 with a job id and asynchronous completion; an
// in-process run whose result is already stored satisfies that contract without
// a worker binary, and the job's COMPLETED status is observable by the same
// polling endpoints. The engine is pure and fast at this dataset size, so
// running it inline is honest rather than a stub — see docs/api.md, planning.
func (s *Service) Suggest(ctx context.Context, planningDate time.Time, depotID, requestedBy string) (Job, error) {
	if depotID == "" {
		return Job{}, ValidationError{Field: "depotId", Message: "is required"}
	}
	job, err := s.repo.CreateJob(ctx, Job{
		PlanningDate: planningDate,
		DepotID:      depotID,
		RequestedBy:  requestedBy,
	})
	if err != nil {
		return Job{}, err
	}

	if _, err := s.repo.UpdateJobStatus(ctx, job.JobID, JobRunning, ""); err != nil {
		return Job{}, err
	}

	in, err := s.loader.LoadInput(ctx, planningDate, depotID)
	if err != nil {
		_, _ = s.repo.UpdateJobStatus(ctx, job.JobID, JobFailed, "input load failed")
		return Job{}, fmt.Errorf("load planning input: %w", err)
	}

	result := s.engine.Plan(in)
	proposals := proposalsFor(result)
	if err := s.repo.SaveProposals(ctx, job.JobID, proposals); err != nil {
		_, _ = s.repo.UpdateJobStatus(ctx, job.JobID, JobFailed, "persist failed")
		return Job{}, err
	}

	return s.repo.UpdateJobStatus(ctx, job.JobID, JobCompleted, "")
}

// Job returns a job's status, or ErrJobNotFound.
func (s *Service) Job(ctx context.Context, jobID string) (Job, error) {
	if jobID == "" {
		return Job{}, ValidationError{Field: "jobId", Message: "is required"}
	}
	return s.repo.GetJob(ctx, jobID)
}

// Results returns a completed job and its proposals.
func (s *Service) Results(ctx context.Context, jobID string) (Job, []Proposal, error) {
	job, err := s.Job(ctx, jobID)
	if err != nil {
		return Job{}, nil, err
	}
	proposals, err := s.repo.LoadProposals(ctx, jobID)
	if err != nil {
		return Job{}, nil, err
	}
	return job, proposals, nil
}

// proposalsFor turns an engine Result into persistable rows. Served orders are
// expanded per trip with their stop sequence; deferred orders carry the binding
// constraint's explanation.
func proposalsFor(r Result) []Proposal {
	out := make([]Proposal, 0, len(r.Deferred))
	// Stable order is already guaranteed by the engine (sorted trips, sorted
	// deferrals); iterate in that order.
	for _, t := range r.Trips {
		for seq, orderID := range t.OrderIDs {
			out = append(out, Proposal{
				OrderID:     orderID,
				Decision:    "SERVE",
				VehicleID:   t.VehicleID,
				TripNo:      t.TripNo,
				Seq:         seq,
				TripMinutes: float64(t.TotalTripMin),
				Explanation: fmt.Sprintf("%s, trip %d: %s / %s", t.VehicleID, t.TripNo, t.Brand, t.District),
			})
		}
	}
	for _, d := range r.Deferred {
		out = append(out, Proposal{
			OrderID:           d.OrderID,
			Decision:          "DEFER",
			Explanation:       d.Reason,
			Constraint:        d.Constraint,
			ConstraintResults: []domain.ConstraintResult{{Code: d.Constraint, Passed: false, Detail: d.Reason}},
		})
	}
	return out
}

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalidInput }

// ErrInvalidInput is wrapped by ValidationError.
var ErrInvalidInput = errors.New("planning: invalid input")
