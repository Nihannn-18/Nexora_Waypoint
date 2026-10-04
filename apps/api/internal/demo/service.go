package demo

import (
	"context"
	"errors"
	"strings"
	"time"

	"waypoint.lk/api/internal/clock"
)

// ErrUnknownStage is returned for a stage name this package does not define.
var ErrUnknownStage = errors.New("unknown demo stage")

// Repository is the persistence the demo controls need. Both writes are
// audited inside their own transaction, so a jump or reset that is not on the
// trail did not happen.
type Repository interface {
	// RecordClockJump audits a clock jump before the clock moves.
	RecordClockJump(ctx context.Context, actor, depotID string, stage Stage, from, to time.Time) error
	// Reset clears operational data and re-seeds, atomically.
	Reset(ctx context.Context, actor, depotID string, clockTo time.Time) (ResetSummary, error)
}

// ResetSummary reports what a reset removed and what the seed put back.
type ResetSummary struct {
	// Cleared is rows deleted per operational table.
	Cleared map[string]int64
	// DemoOrders is the seeded scenario's order count after the reset.
	DemoOrders int
	// DemoVehicleDays is the vehicle-day availability rows written.
	DemoVehicleDays int
}

// Service moves the demo clock and resets the demo data. It holds the API's
// one Settable clock, so every package that reads "now" sees the jump.
type Service struct {
	clock clock.Settable
	repo  Repository
	loc   *time.Location
	// start is DEMO_CLOCK_START: where a reset puts the clock back.
	start time.Time
}

// NewService builds the demo service. start is the configured demo clock
// start, which a reset returns to.
func NewService(clk clock.Settable, repo Repository, loc *time.Location, start time.Time) *Service {
	return &Service{clock: clk, repo: repo, loc: loc, start: start.In(loc)}
}

// JumpTo moves the API clock to stage and returns the new "now". The jump is
// audited first; if the audit write fails the clock does not move.
func (s *Service) JumpTo(ctx context.Context, actor, depotID string, stage Stage) (time.Time, error) {
	stage = Stage(strings.TrimSpace(string(stage)))
	at, ok := stage.At(s.loc)
	if !ok {
		return time.Time{}, ErrUnknownStage
	}
	if err := s.repo.RecordClockJump(ctx, actor, depotID, stage, s.clock.Now(), at); err != nil {
		return time.Time{}, err
	}
	s.clock.Set(at)
	return s.clock.Now(), nil
}

// Reset clears the operational data, re-seeds the demo day and puts the clock
// back to DEMO_CLOCK_START. The clock only moves once the data reset commits.
func (s *Service) Reset(ctx context.Context, actor, depotID string) (ResetSummary, time.Time, error) {
	sum, err := s.repo.Reset(ctx, actor, depotID, s.start)
	if err != nil {
		return ResetSummary{}, time.Time{}, err
	}
	s.clock.Set(s.start)
	return sum, s.clock.Now(), nil
}
