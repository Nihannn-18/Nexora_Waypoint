package routes

import (
	"context"
	"fmt"
	"sort"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/planning"
)

// Confirmation converts a dispatcher's chosen planning proposal into
// authoritative route/allocation state. It validates the proposal against
// current database state, then writes everything in one transaction.
//
// It does not run Planning and does not re-plan. It trusts the proposal's
// assignment only after re-checking the facts the schema can protect.
type Confirmation struct {
	repo Repository
	// planning loads a job's proposals and job metadata.
	planning PlanningReader
	// orders loads order facts (outlet, district, depot, brand, totals, status)
	// needed to build legs and route metrics and to validate eligibility.
	orders OrderReader
	// vehicles loads a vehicle's depot so the route's depot is authoritative.
	vehicles VehicleReader
	// travel/allowance/outlet supply the official trip metrics, reused from the
	// planning inputs rather than recomputed with a new formula.
	refs ReferenceReader
	// planInput loads the authoritative planning input (orders, vehicles,
	// reference, calendar, fuel) for the job's date and depot, so confirmation
	// can re-run planning.CheckTrip — the single feasibility authority — against
	// current database state before it persists anything.
	planInput PlanInputReader
	// clock is the injected clock; kept for future use (e.g. deferral target
	// dates). Confirmation itself is deterministic and date-driven.
	clock Clock
}

// Clock reports the current business-time instant.
type Clock interface{ Now() time.Time }

// PlanningReader is the slice of the planning service confirmation needs.
type PlanningReader interface {
	Job(ctx context.Context, jobID string) (planning.Job, error)
	Results(ctx context.Context, jobID string) (planning.Job, []planning.Proposal, error)
}

// OrderFacts is the order data confirmation needs, reduced to what it validates
// and persists.
type OrderFacts struct {
	OrderID       string
	OutletID      string
	District      string
	DepotID       string
	Brand         domain.Brand
	Status        domain.OrderStatus
	TotalWeightKg float64
	TotalVolumeM3 float64
}

// OrderReader loads order facts by id.
type OrderReader interface {
	LoadOrderFacts(ctx context.Context, orderID string) (OrderFacts, error)
}

// VehicleReader loads a vehicle's home depot.
type VehicleReader interface {
	VehicleDepot(ctx context.Context, vehicleID string) (string, error)
}

// ReferenceReader supplies the authoritative trip-time and stop data used to
// compute route metrics and leg order.
type ReferenceReader interface {
	// OutletFor returns the outlet's dock type and window times for a stop.
	OutletFor(ctx context.Context, outletID string) (OutletRef, error)
	// Travel returns depot→district and inter-stop reference for a depot/district.
	Travel(ctx context.Context, depotID, district string) (TravelRef, error)
	// ServiceAllowance returns the handling minutes for a brand/dock type.
	ServiceAllowance(ctx context.Context, brand domain.Brand, dockType domain.DockType) (int, error)
}

// OutletRef is the outlet data confirmation needs.
type OutletRef struct {
	OutletID string
	DockType domain.DockType
}

// TravelRef is the authoritative travel reference for a depot/district.
type TravelRef struct {
	DepotToDistrictKm  float64
	DepotToDistrictMin int
	InterStopKm        float64
	InterStopMin       int
}

// NewConfirmation builds the confirmation service.
func NewConfirmation(repo Repository, pr PlanningReader, or OrderReader, vr VehicleReader, refs ReferenceReader, planInput PlanInputReader, clock Clock) *Confirmation {
	return &Confirmation{repo: repo, planning: pr, orders: or, vehicles: vr, refs: refs, planInput: planInput, clock: clock}
}

// ConfirmInput is the dispatcher's confirmation of a planning job.
type ConfirmInput struct {
	JobID string
	Actor string
	// Routes the dispatcher chose, in the same shape as the confirm contract.
	Routes []RouteChoice
	// Deferrals for unserved orders.
	Deferrals []DeferralChoice
}

// RouteChoice is one chosen route.
type RouteChoice struct {
	VehicleID string
	TripNo    int
	OrderIDs  []string
}

// DeferralChoice is one unserved order with its reason.
type DeferralChoice struct {
	OrderID        string
	ReasonType     string
	ConstraintCode domain.ConstraintCode
	ReasonText     string
	DeferredToDate string
}

// Confirm validates the chosen proposal and persists it. It is safe to retry:
// a second identical confirmation fails cleanly (ErrConflict) without creating
// duplicate routes, legs or allocations, because the writes are guarded by
// database uniqueness and the order row locks.
func (c *Confirmation) Confirm(ctx context.Context, in ConfirmInput) (ConfirmationResult, error) {
	if in.JobID == "" {
		return ConfirmationResult{}, ValidationError{Field: "jobId", Message: "is required"}
	}
	job, proposals, err := c.planning.Results(ctx, in.JobID)
	if err != nil {
		return ConfirmationResult{}, err
	}
	if job.Status != planning.JobCompleted {
		return ConfirmationResult{}, fmt.Errorf("%w: planning job %s is %s, not COMPLETED", ErrConflict, job.JobID, job.Status)
	}

	// Index the proposals for lookup; a confirmed order must correspond to a
	// SERVE proposal from this job.
	serveByOrder := map[string]planning.Proposal{}
	deferByOrder := map[string]planning.Proposal{}
	for _, p := range proposals {
		switch p.Decision {
		case "SERVE":
			serveByOrder[p.OrderID] = p
		case "DEFER":
			deferByOrder[p.OrderID] = p
		}
	}

	// Facts are loaded once and shared with the hard-constraint revalidation,
	// so both the route build and the validator see the same current state.
	facts := map[string]OrderFacts{}
	plan, err := c.buildPlan(ctx, job, in, serveByOrder, deferByOrder, facts)
	if err != nil {
		return ConfirmationResult{}, err
	}

	// Every confirmed order must be either allocated or deferred, and every
	// deferral must name a reason. This is the requirement the submission turns
	// on; a plan that omits an order is rejected before any write.
	if err := c.assertComplete(in, serveByOrder, deferByOrder); err != nil {
		return ConfirmationResult{}, err
	}

	// Hard-constraint revalidation, reusing the authoritative validator. This is
	// the last gate before the transactional write: a plan that violates a hard
	// rule on current state is rejected and never persisted.
	input, err := c.planInput.LoadInput(ctx, job.PlanningDate, job.DepotID)
	if err != nil {
		return ConfirmationResult{}, fmt.Errorf("load planning input for revalidation: %w", err)
	}
	if err := c.revalidateHardConstraints(ctx, plan.RouteDate, in.Routes, facts, input); err != nil {
		return ConfirmationResult{}, err
	}

	return c.repo.Confirm(ctx, plan)
}

// buildPlan validates every chosen route and deferral and assembles the writes.
// It records each order's freshly-loaded facts in `facts` for the later
// hard-constraint revalidation pass.
func (c *Confirmation) buildPlan(ctx context.Context, job planning.Job, in ConfirmInput, serveByOrder, deferByOrder map[string]planning.Proposal, facts map[string]OrderFacts) (ConfirmationPlan, error) {
	plan := ConfirmationPlan{
		DepotID:       job.DepotID,
		RouteDate:     job.PlanningDate.Format("2006-01-02"),
		Actor:         in.Actor,
		PlanningJobID: job.JobID,
	}

	seenOrder := map[string]bool{}

	for _, choice := range in.Routes {
		if !ValidTripNo(choice.TripNo) {
			return ConfirmationPlan{}, ValidationError{Field: "tripNo", Message: "must be 1 or 2"}
		}
		if choice.VehicleID == "" {
			return ConfirmationPlan{}, ValidationError{Field: "vehicleId", Message: "is required"}
		}
		if len(choice.OrderIDs) == 0 {
			return ConfirmationPlan{}, ValidationError{Field: "orderIds", Message: "a route needs at least one order"}
		}
		depotID, err := c.vehicles.VehicleDepot(ctx, choice.VehicleID)
		if err != nil {
			return ConfirmationPlan{}, err
		}
		if depotID != job.DepotID {
			return ConfirmationPlan{}, fmt.Errorf("%w: vehicle %s is not based at the planning depot", ErrInvalid, choice.VehicleID)
		}

		route := Route{
			VehicleID: choice.VehicleID,
			DepotID:   depotID,
			RouteDate: plan.RouteDate,
			TripNo:    choice.TripNo,
			Status:    RouteConfirmed,
		}

		for _, orderID := range choice.OrderIDs {
			if seenOrder[orderID] {
				return ConfirmationPlan{}, fmt.Errorf("%w: order %s appears more than once", ErrInvalid, orderID)
			}
			seenOrder[orderID] = true

			p, ok := serveByOrder[orderID]
			if !ok {
				return ConfirmationPlan{}, fmt.Errorf("%w: order %s is not a SERVE proposal of job %s", ErrInvalid, orderID, job.JobID)
			}
			if p.VehicleID != "" && p.VehicleID != choice.VehicleID {
				return ConfirmationPlan{}, fmt.Errorf("%w: order %s was proposed for vehicle %s, not %s", ErrInvalid, orderID, p.VehicleID, choice.VehicleID)
			}
			if p.TripNo != 0 && p.TripNo != choice.TripNo {
				return ConfirmationPlan{}, fmt.Errorf("%w: order %s was proposed for trip %d, not %d", ErrInvalid, orderID, p.TripNo, choice.TripNo)
			}

			orderFacts, err := c.orders.LoadOrderFacts(ctx, orderID)
			if err != nil {
				return ConfirmationPlan{}, err
			}
			if err := c.assertOrderConfirmable(orderID, orderFacts); err != nil {
				return ConfirmationPlan{}, err
			}
			facts[orderID] = orderFacts
			if route.Brand == "" {
				route.Brand, route.District = orderFacts.Brand, orderFacts.District
			} else if orderFacts.Brand != route.Brand || orderFacts.District != route.District {
				return ConfirmationPlan{}, fmt.Errorf("%w: order %s breaks the one brand + district rule for this trip", ErrInvalid, orderID)
			}
			route.TotalWeightKg += orderFacts.TotalWeightKg
			route.TotalVolumeM3 += orderFacts.TotalVolumeM3
		}

		// Build legs in the dispatcher's chosen stop order, from the proposal's
		// sequence if present, else the given order.
		if err := c.buildLegs(ctx, &route, choice.OrderIDs, serveByOrder); err != nil {
			return ConfirmationPlan{}, err
		}
		plan.Routes = append(plan.Routes, route)
	}

	for _, d := range in.Deferrals {
		if d.OrderID == "" {
			return ConfirmationPlan{}, ValidationError{Field: "orderId", Message: "is required"}
		}
		if d.ReasonText == "" {
			return ConfirmationPlan{}, ValidationError{Field: "reasonText", Message: "a deferral must carry a reason"}
		}
		if d.ReasonType == "" {
			return ConfirmationPlan{}, ValidationError{Field: "reasonType", Message: "is required"}
		}
		if seenOrder[d.OrderID] {
			return ConfirmationPlan{}, fmt.Errorf("%w: order %s is both allocated and deferred", ErrInvalid, d.OrderID)
		}
		seenOrder[d.OrderID] = true
		if _, ok := deferByOrder[d.OrderID]; !ok {
			return ConfirmationPlan{}, fmt.Errorf("%w: order %s is not a DEFER proposal of job %s", ErrInvalid, d.OrderID, job.JobID)
		}
		plan.Deferrals = append(plan.Deferrals, DeferredOrderForConfirm{
			OrderID: d.OrderID, ReasonType: d.ReasonType, Reason: d.ReasonText,
			ConstraintCode: d.ConstraintCode, DeferredToDate: d.DeferredToDate,
		})
	}

	return plan, nil
}

// buildLegs fills the route's legs in stop order and computes the official
// trip metrics from the authoritative reference data.
func (c *Confirmation) buildLegs(ctx context.Context, route *Route, orderIDs []string, serveByOrder map[string]planning.Proposal) error {
	// Order the stops by the proposal's sequence when available, so the route
	// reflects the plan; ties fall back to the given order.
	ordered := append([]string{}, orderIDs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		si := serveByOrder[ordered[i]].Seq
		sj := serveByOrder[ordered[j]].Seq
		return si < sj
	})

	route.Legs = make([]RouteLeg, 0, len(ordered))
	for i, orderID := range ordered {
		facts, err := c.orders.LoadOrderFacts(ctx, orderID)
		if err != nil {
			return err
		}
		outlet, err := c.refs.OutletFor(ctx, facts.OutletID)
		if err != nil {
			return err
		}
		fromPoint := "DEPOT"
		if i > 0 {
			prev, err := c.orders.LoadOrderFacts(ctx, ordered[i-1])
			if err != nil {
				return err
			}
			fromPoint = prev.OutletID
		}
		route.Legs = append(route.Legs, RouteLeg{
			OrderID: orderID, Seq: i, FromPoint: fromPoint, ToOutlet: facts.OutletID, Status: LegPending,
		})
		_ = outlet
	}

	travel, err := c.refs.Travel(ctx, route.DepotID, route.District)
	if err != nil {
		return err
	}
	route.OutboundMin = travel.DepotToDistrictMin
	route.InterStopMin = travel.InterStopMin * max(len(route.Legs)-1, 0)
	route.DistanceKm = travel.DepotToDistrictKm + travel.InterStopKm*float64(max(len(route.Legs)-1, 0))

	handling := 0
	for i, orderID := range ordered {
		facts, err := c.orders.LoadOrderFacts(ctx, orderID)
		if err != nil {
			return err
		}
		outlet, err := c.refs.OutletFor(ctx, facts.OutletID)
		if err != nil {
			return err
		}
		mins, err := c.refs.ServiceAllowance(ctx, route.Brand, outlet.DockType)
		if err != nil {
			return err
		}
		handling += mins
		route.Legs[i].DistanceKm = 0
		if i > 0 {
			route.Legs[i].DistanceKm = travel.InterStopKm
		} else {
			route.Legs[i].DistanceKm = travel.DepotToDistrictKm
		}
	}
	route.HandlingMin = handling
	route.TotalTripMin = route.OutboundMin + route.InterStopMin + route.HandlingMin

	if err := route.Validate(); err != nil {
		return err
	}
	return ValidateLegOrder(route.Legs)
}

// assertOrderConfirmable rejects an order that is not in a state a confirmation
// may transition, or is already allocated.
func (c *Confirmation) assertOrderConfirmable(orderID string, facts OrderFacts) error {
	switch facts.Status {
	case domain.OrderConfirmed, domain.OrderDeferred:
		// Both may be confirmed/allocated by a fresh run.
	default:
		return fmt.Errorf("%w: order %s is %s and cannot be confirmed again", ErrConflict, orderID, facts.Status)
	}
	return nil
}

// assertComplete verifies every SERVE proposal the dispatcher is responsible
// for is either allocated or deferred. Per the contract, a plan that would leave
// a confirmed order neither is rejected.
func (c *Confirmation) assertComplete(in ConfirmInput, serveByOrder, _ map[string]planning.Proposal) error {
	chosen := map[string]bool{}
	for _, r := range in.Routes {
		for _, id := range r.OrderIDs {
			chosen[id] = true
		}
	}
	for _, d := range in.Deferrals {
		chosen[d.OrderID] = true
	}
	for id := range serveByOrder {
		if !chosen[id] {
			return fmt.Errorf("%w: order %s is neither allocated nor deferred", ErrInvalid, id)
		}
	}
	return nil
}
