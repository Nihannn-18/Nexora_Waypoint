package planning

import (
	"time"

	"waypoint.lk/api/internal/domain"
)

// This file defines the engine's input and output vocabulary. It is deliberately
// separate from the persistence models in internal/orders and internal/catalog:
// the engine is pure and unit-testable, so it takes plain loaded values and
// returns proposals. The repository layer (job.go) adapts database rows into
// these types and writes the results back as planning_result rows.

// Order is one confirmed order available to the planning run, flattened to what
// feasibility needs. It carries the whole-order totals (never per-line), because
// an order is never split across trips.
type Order struct {
	OrderID     string
	OrderNumber string
	OutletID    string
	Brand       domain.Brand
	// District is denormalised from the outlet so grouping does not re-join.
	District string
	// DepotID is the outlet's depot; vehicles must match it.
	DepotID string
	// RequestedDeliveryDate is the day being planned (date-only).
	RequestedDeliveryDate time.Time
	// Totals are the whole order's demands. Neither is ever split.
	TotalWeightKg float64
	TotalVolumeM3 float64
	// TempRequirement is the strongest requirement across the order's lines.
	TempRequirement domain.TempRequirement
	// ParkingConstraint is the outlet's access rule; VAN_ONLY forces a van.
	ParkingConstraint domain.ParkingConstraint
	// WindowOpen/Close are the outlet's delivery window as "HH:mm".
	WindowOpen  string
	WindowClose string
	// MallWindowOpen/Close are set only for MALL_DOCK outlets, as "HH:mm".
	MallWindowOpen  string
	MallWindowClose string
	// Fairness history, read per order and aggregated per outlet by the policy
	// stage. Carried here so the ranking can honour it without a re-query.
	DeferredYesterday   bool
	DaysSinceLastServed int
	// Sequence is the stable input position, used only as a final tie-break.
	Sequence int
}

// Item is a catalogue item a line references, reduced to dimensions. Present so
// the engine can re-derive per-line needs if an order ever needs line-level
// checks; totals on Order are authoritative for capacity.
type Item struct {
	ItemID                 string
	Brand                  domain.Brand
	UnitWeightKg           float64
	UnitVolumeM3           float64
	TemperatureRequirement domain.TempRequirement
}

// Outlet is the planning view of an outlet.
type Outlet struct {
	OutletID          string
	Brand             domain.Brand
	District          string
	DepotID           string
	DockType          domain.DockType
	ParkingConstraint domain.ParkingConstraint
	MallWindowOpen    string
	MallWindowClose   string
	WindowOpen        string
	WindowClose       string
}

// Vehicle is a fleet vehicle, reduced to its planning capabilities.
type Vehicle struct {
	VehicleID string
	Type      domain.VehicleType
	TempClass domain.VehicleTempClass
	// WeightCapKg and VolumeCapM3 are per-trip capacities.
	WeightCapKg float64
	VolumeCapM3 float64
	// KmPerL is the vehicle's fuel efficiency, used only when the weekly quota
	// is modelled.
	KmPerL float64
	// WeeklyFuelQuotaL is the vehicle's ISO-week fuel allowance
	// (vehicle.weekly_fuel_quota_l). A positive value means the fuel quota is
	// enforced for this vehicle.
	WeeklyFuelQuotaL float64
	DepotID          string
	// Available is false when the vehicle is in the workshop / broken down on
	// the planning date. Only available vehicles may be used.
	Available bool
}

// CanCarryTemp reports whether the vehicle may carry the requirement: a reefer
// may carry anything; an ambient vehicle may carry only ambient.
func (v Vehicle) CanCarryTemp(req domain.TempRequirement) bool {
	if !req.RequiresReefer() {
		return true
	}
	return v.TempClass == domain.VehicleTempReefer
}

// Travel is the district_travel reference for a (depot, district) pair.
type Travel struct {
	District           string
	DepotID            string
	DepotToDistrictKm  float64
	DepotToDistrictMin int
	InterStopKm        float64
	InterStopMin       int
}

// ServiceAllowance is the handling time for a (brand, dock type) combination.
type ServiceAllowance struct {
	Brand    domain.Brand
	DockType domain.DockType
	Minutes  int
}

// CalendarDay is the operating-day fact for the planning date.
type CalendarDay struct {
	Date        time.Time
	IsOperating bool
}

// Input is everything one planning run needs. Every field is loaded by the
// caller; the engine performs no I/O, so a run is reproducible from an Input.
type Input struct {
	PlanningDate time.Time
	DepotID      string
	Orders       []Order
	// Items, Outlets, Vehicles, Travel and ServiceAllowances are lookup tables
	// keyed by their natural identifiers.
	Items             map[string]Item
	Outlets           map[string]Outlet
	Vehicles          []Vehicle
	Travel            map[string]Travel // key: depotID+"|"+district
	ServiceAllowances map[string]int    // key: brand+"|"+dockType
	Calendar          CalendarDay
	// FuelUsedL is the vehicle's ISO-week fuel already consumed, keyed by
	// vehicle id. Absent means zero. Used only when the quota is modelled.
	FuelUsedL map[string]float64
	// FuelQuotaL is the vehicle's weekly quota, keyed by vehicle id. A zero or
	// absent entry means the quota is not modelled for that vehicle, so the
	// fuel constraint is not applied to it.
	FuelQuotaL map[string]float64
}

// PlannedTrip is one feasible proposed trip: a vehicle running one brand and one
// district, with the orders it would serve in stop order.
type PlannedTrip struct {
	VehicleID string
	TripNo    int // 1 or 2
	Brand     domain.Brand
	District  string
	DepotID   string
	OrderIDs  []string
	// Metrics, computed with the official formula (planning.CalculateTripTime).
	WeightKg     float64
	VolumeM3     float64
	OutboundMin  int
	InterStopMin int
	HandlingMin  int
	TotalTripMin int
	// LastArrival is the latest planned arrival across the trip's stops, "HH:mm".
	LastArrival string
}

// DeferredOrder is an order the run could not serve, with the binding rule.
type DeferredOrder struct {
	OrderID     string
	OrderNumber string
	OutletID    string
	Brand       domain.Brand
	District    string
	// Constraint is the hard rule that left no feasible slot.
	Constraint domain.ConstraintCode
	// Reason is a plain-language explanation for the store manager.
	Reason string
}

// Result is the engine's full output: the proposed trips and the orders it could
// not place. It is a proposal, never an allocation.
type Result struct {
	Trips    []PlannedTrip
	Deferred []DeferredOrder
}
