package planning

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/domain"
)

// Job status values, matching the planning_job CHECK constraint and
// PLANNING_JOB_STATUSES in libs/shared-types.
const (
	JobQueued    = "QUEUED"
	JobRunning   = "RUNNING"
	JobCompleted = "COMPLETED"
	JobFailed    = "FAILED"
)

// Job is the persisted planning job. A job is metadata about a run; it never
// authorises service. The results it produces are proposals only.
type Job struct {
	JobID        string
	PlanningDate time.Time
	DepotID      string
	Status       string
	RequestedBy  string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	ErrorMessage string
}

// Proposal is one row of a job's output, mirroring a planning_result row.
type Proposal struct {
	OrderID           string
	Decision          string // SERVE or DEFER
	VehicleID         string
	TripNo            int
	Seq               int
	TripMinutes       float64
	Explanation       string
	Constraint        domain.ConstraintCode
	ConstraintResults []domain.ConstraintResult
}

// Repository persists planning jobs and their proposed results, and loads the
// inputs a run needs. It writes only planning_job/planning_result — never
// route, route_leg or allocation, which stay the dispatcher's to confirm.
type Repository interface {
	CreateJob(ctx context.Context, job Job) (Job, error)
	UpdateJobStatus(ctx context.Context, jobID, status string, errMsg string) (Job, error)
	GetJob(ctx context.Context, jobID string) (Job, error)
	SaveProposals(ctx context.Context, jobID string, proposals []Proposal) error
	LoadProposals(ctx context.Context, jobID string) ([]Proposal, error)
}

// Loader loads the planning inputs from the database. It is separate from the
// Repository so the engine stays pure and the run's data access is one clear
// seam.
type Loader interface {
	LoadInput(ctx context.Context, planningDate time.Time, depotID string) (Input, error)
}

// PGRepository is the PostgreSQL-backed Repository.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a planning repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

// CreateJob inserts a QUEUED job and returns it with its generated id.
func (r *PGRepository) CreateJob(ctx context.Context, job Job) (Job, error) {
	var out Job
	err := r.pool.QueryRow(ctx, `
		INSERT INTO planning_job (planning_date, depot_id, status, requested_by)
		VALUES ($1, $2, $3, $4)
		RETURNING job_id, planning_date, depot_id, status,
		          COALESCE(requested_by, ''), created_at, started_at, completed_at,
		          COALESCE(error_message, '')`,
		job.PlanningDate, job.DepotID, JobQueued, nullable(job.RequestedBy)).
		Scan(&out.JobID, &out.PlanningDate, &out.DepotID, &out.Status, &out.RequestedBy,
			&out.CreatedAt, &out.StartedAt, &out.CompletedAt, &out.ErrorMessage)
	if err != nil {
		return Job{}, fmt.Errorf("create planning job: %w", err)
	}
	return out, nil
}

// UpdateJobStatus moves a job to a new status, stamping started/completed.
func (r *PGRepository) UpdateJobStatus(ctx context.Context, jobID, status string, errMsg string) (Job, error) {
	var out Job
	var started, completed *time.Time
	switch status {
	case JobRunning:
		now := time.Now()
		started = &now
	case JobCompleted, JobFailed:
		now := time.Now()
		completed = &now
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE planning_job
		SET status = $2,
		    started_at = COALESCE($3, started_at),
		    completed_at = COALESCE($4, completed_at),
		    error_message = NULLIF($5, '')
		WHERE job_id = $1
		RETURNING job_id, planning_date, depot_id, status,
		          COALESCE(requested_by, ''), created_at, started_at, completed_at,
		          COALESCE(error_message, '')`,
		jobID, status, started, completed, errMsg).
		Scan(&out.JobID, &out.PlanningDate, &out.DepotID, &out.Status, &out.RequestedBy,
			&out.CreatedAt, &out.StartedAt, &out.CompletedAt, &out.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("update planning job: %w", err)
	}
	return out, nil
}

// GetJob loads one job, or ErrJobNotFound.
func (r *PGRepository) GetJob(ctx context.Context, jobID string) (Job, error) {
	var out Job
	err := r.pool.QueryRow(ctx, `
		SELECT job_id, planning_date, depot_id, status, COALESCE(requested_by, ''),
		       created_at, started_at, completed_at, COALESCE(error_message, '')
		FROM planning_job WHERE job_id::text = $1`, jobID).
		Scan(&out.JobID, &out.PlanningDate, &out.DepotID, &out.Status, &out.RequestedBy,
			&out.CreatedAt, &out.StartedAt, &out.CompletedAt, &out.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("get planning job: %w", err)
	}
	return out, nil
}

// SaveProposals writes a job's results in one transaction. Existing rows for
// the job are replaced, so re-running a job id is idempotent.
func (r *PGRepository) SaveProposals(ctx context.Context, jobID string, proposals []Proposal) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin proposals tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM planning_result WHERE job_id = $1`, jobID); err != nil {
		return fmt.Errorf("clear planning results: %w", err)
	}
	for _, p := range proposals {
		var vehicleID *string
		var tripNo, seq *int
		if p.Decision == "SERVE" {
			vehicleID = &p.VehicleID
			tripNo = &p.TripNo
			seq = &p.Seq
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO planning_result (
				job_id, order_id, decision, vehicle_id, trip_no, seq,
				trip_minutes, explanation, constraint_results
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			jobID, p.OrderID, dbDecision(p.Decision), vehicleID, tripNo, seq,
			p.TripMinutes, p.Explanation, p.ConstraintResults)
		if err != nil {
			return fmt.Errorf("insert planning result for %s: %w", p.OrderID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit proposals: %w", err)
	}
	return nil
}

// LoadProposals reads a job's results in a deterministic order.
func (r *PGRepository) LoadProposals(ctx context.Context, jobID string) ([]Proposal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT order_id, decision, COALESCE(vehicle_id, ''), COALESCE(trip_no, 0),
		       COALESCE(seq, 0), COALESCE(trip_minutes, 0), COALESCE(explanation, ''),
		       COALESCE(constraint_results, '[]'::jsonb)
		FROM planning_result WHERE job_id = $1
		ORDER BY decision, order_id`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load planning results: %w", err)
	}
	defer rows.Close()

	out := make([]Proposal, 0)
	for rows.Next() {
		var p Proposal
		var decision string
		if err := rows.Scan(&p.OrderID, &decision, &p.VehicleID, &p.TripNo,
			&p.Seq, &p.TripMinutes, &p.Explanation, &p.ConstraintResults); err != nil {
			return nil, fmt.Errorf("scan planning result: %w", err)
		}
		p.Decision = apiDecision(decision)
		// The binding constraint is the first failed rule, as the engine wrote it.
		for _, cr := range p.ConstraintResults {
			if !cr.Passed {
				p.Constraint = cr.Code
				break
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning results: %w", err)
	}
	return out, nil
}

// dbDecision maps the API/proposal vocabulary (shared-types PLANNING_DECISIONS:
// SERVE/DEFER) to the persisted planning_result CHECK values
// (ALLOCATED/DEFERRED). The two layers use different words for the same fact;
// this is the single place they are reconciled.
func dbDecision(api string) string {
	if api == "SERVE" {
		return "ALLOCATED"
	}
	return "DEFERRED"
}

// apiDecision is the inverse of dbDecision.
func apiDecision(db string) string {
	if db == "ALLOCATED" {
		return "SERVE"
	}
	return "DEFER"
}

// ErrJobNotFound means no planning job matches the id.
var ErrJobNotFound = errors.New("planning: job not found")

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
