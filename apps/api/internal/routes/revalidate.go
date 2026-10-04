package routes

import (
	"context"
	"fmt"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/planning"
)

// PlanInputReader loads the authoritative planning input for one depot and date.
// Confirmation uses it to rebuild exactly the world the planner saw, so it can
// re-run the same validator rather than a second, weaker set of rules.
type PlanInputReader interface {
	LoadInput(ctx context.Context, planningDate time.Time, depotID string) (planning.Input, error)
}

// ConstraintViolationError reports that a confirmation was rejected because one
// or more hard constraints failed on the current database state. It carries the
// full passed/failed verdict so the handler can answer 422 with
// constraintResults, exactly as the allocation validator does.
type ConstraintViolationError struct {
	// Results is every rule's verdict for the failing route, passed and failed.
	Results []domain.ConstraintResult
	// Detail names the failing route in plain language.
	Detail string
}

func (e ConstraintViolationError) Error() string {
	if e.Detail == "" {
		return "confirmation violates a hard constraint"
	}
	return "confirmation violates a hard constraint: " + e.Detail
}

// revalidateHardConstraints re-runs the authoritative trip validator over every
// chosen route using freshly-loaded order facts and the just-loaded planning
// input. It is the single feasibility authority: the same planning.CheckTrip the
// engine calls. A route that fails any rule aborts the confirmation before any
// write, so a plan that violates a hard constraint can never be persisted.
//
// Budget accounting matches the engine: a vehicle's day budget is the sum of its
// already-confirmed trips for the day plus the routes chosen earlier in this same
// confirmation. Routes are validated in the order given, so the accumulation is
// deterministic.
func (c *Confirmation) revalidateHardConstraints(ctx context.Context, planDate string, choices []RouteChoice, facts map[string]OrderFacts, in planning.Input) error {
	vehicleByID := make(map[string]planning.Vehicle, len(in.Vehicles))
	for _, v := range in.Vehicles {
		vehicleByID[v.VehicleID] = v
	}

	budget := map[string]planning.VehicleDayBudget{}
	if err := c.seedExistingBudget(ctx, planDate, budget); err != nil {
		return err
	}

	for _, choice := range choices {
		vehicle, ok := vehicleByID[choice.VehicleID]
		if !ok {
			// The planning loader only includes vehicles at the job's depot, so
			// a missing vehicle is one the run could not have chosen. Depot was
			// already checked in buildPlan; treat as an infeasible candidate.
			return ConstraintViolationError{
				Results: []domain.ConstraintResult{{
					Code:   domain.ConstraintVehicleUnavailable,
					Passed: false,
					Detail: fmt.Sprintf("vehicle %s is not available at the planning depot for %s", choice.VehicleID, planDate),
				}},
				Detail: fmt.Sprintf("vehicle %s is not available", choice.VehicleID),
			}
		}

		orders := make([]planning.Order, 0, len(choice.OrderIDs))
		for _, orderID := range choice.OrderIDs {
			f, ok := facts[orderID]
			if !ok {
				return fmt.Errorf("%w: order %s facts missing for revalidation", ErrInvalid, orderID)
			}
			orders = append(orders, orderFromFacts(f, in, orderID))
		}

		cand := planning.Candidate{
			Vehicle:         vehicle,
			TripNo:          choice.TripNo,
			Orders:          orders,
			DayBudget:       budget[vehicle.VehicleID],
			WeeklyFuelUsedL: in.FuelUsedL[vehicle.VehicleID],
		}
		verdict := planning.CheckTrip(in, cand)
		if !verdict.OK {
			return ConstraintViolationError{
				Results: verdict.Results,
				Detail:  fmt.Sprintf("%s trip %d: %s", choice.VehicleID, choice.TripNo, verdict.Reason),
			}
		}
		budget[vehicle.VehicleID] = addPlannedTrip(budget[vehicle.VehicleID], verdict.Trip)
	}
	return nil
}

// seedExistingBudget adds the minutes of every route already confirmed for the
// planning date to its vehicle's day budget, so a second confirmation for the
// same day cannot push a vehicle past 270 / 480 minutes combined.
func (c *Confirmation) seedExistingBudget(ctx context.Context, planDate string, budget map[string]planning.VehicleDayBudget) error {
	existing, err := c.repo.ListRoutes(ctx, planDate, "")
	if err != nil {
		return err
	}
	for _, rt := range existing {
		b := budget[rt.VehicleID]
		if rt.Brand == domain.BrandFresh {
			b.FreshMinutesUsed += rt.TotalTripMin
		} else {
			b.StyleTechMinutesUsed += rt.TotalTripMin
		}
		budget[rt.VehicleID] = b
	}
	return nil
}

// addPlannedTrip mirrors the engine's budget accrual: a trip's minutes land in
// the pool its brand draws on (Fresh alone; Style and Tech shared).
func addPlannedTrip(b planning.VehicleDayBudget, t planning.PlannedTrip) planning.VehicleDayBudget {
	if t.Brand == domain.BrandFresh {
		b.FreshMinutesUsed += t.TotalTripMin
	} else {
		b.StyleTechMinutesUsed += t.TotalTripMin
	}
	return b
}

// orderFromFacts builds a planning.Order for revalidation from the freshly
// loaded database facts and the loaded outlet reference. Totals, status and
// window/access facts are current, so a change since planning (a weight change,
// a moved window) is caught by the same rules the planner applied.
func orderFromFacts(f OrderFacts, in planning.Input, orderID string) planning.Order {
	o := planning.Order{
		OrderID:               orderID,
		OutletID:              f.OutletID,
		District:              f.District,
		DepotID:               f.DepotID,
		Brand:                 f.Brand,
		RequestedDeliveryDate: in.PlanningDate,
		TotalWeightKg:         f.TotalWeightKg,
		TotalVolumeM3:         f.TotalVolumeM3,
	}
	if outlet, ok := in.Outlets[f.OutletID]; ok {
		o.Brand = outlet.Brand
		o.District = outlet.District
		o.DepotID = outlet.DepotID
		o.ParkingConstraint = outlet.ParkingConstraint
		o.WindowOpen = outlet.WindowOpen
		o.WindowClose = outlet.WindowClose
		o.MallWindowOpen = outlet.MallWindowOpen
		o.MallWindowClose = outlet.MallWindowClose
	}
	return o
}
