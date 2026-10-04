package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Driver-facing reads for the R-01 cockpit and the stop screen. Every field is
// a stored fact; nothing (ETA, distance, GPS) is derived here. PlannedArrival is
// route_leg.planned_arrival as confirmation wrote it, empty when unset.

// RouteSummary is one route's header as a driver sees it.
type RouteSummary struct {
	RouteID   string
	RouteDate string
	VehicleID string
	TripNo    int
	Brand     string
	District  string
	Status    string
}

// OutletInfo is the stop's outlet and its delivery windows ("HH:MM").
type OutletInfo struct {
	OutletID          string
	Name              string
	District          string
	DockType          string
	ParkingConstraint string
	WindowOpen        string
	WindowClose       string
	MallWindowOpen    string
	MallWindowClose   string
}

// OrderLine is one order line on a stop.
type OrderLine struct {
	OrderItemID string
	SKU         string
	Name        string
	Quantity    int
}

// StopOrder is one order delivered at a stop, totals from the order row.
type StopOrder struct {
	OrderID         string
	OrderNumber     string
	TempRequirement string
	TotalUnits      int
	TotalWeightKg   float64
	TotalVolumeM3   float64
	Lines           []OrderLine
}

// LegDetail is GET /legs/{id}: the leg context plus everything the stop screen shows.
type LegDetail struct {
	LegContext
	Seq            int
	PlannedArrival string
	Route          RouteSummary
	Outlet         OutletInfo
	Orders         []StopOrder
}

// RouteStop is one stop in a driver route listing.
type RouteStop struct {
	LegID          string
	Seq            int
	OutletID       string
	OutletName     string
	WindowOpen     string
	WindowClose    string
	PlannedArrival string
	Status         string
}

// DriverRoute is one route with its stops in sequence.
type DriverRoute struct {
	RouteSummary
	Stops []RouteStop
}

func rfc3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// LegDetail implements Repository.
func (r *PGRepository) LegDetail(ctx context.Context, legID string) (LegDetail, error) {
	var d LegDetail
	var planned *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT leg.leg_id, leg.route_id, route.depot_id, route.route_date::text, leg.to_outlet, leg.status,
		       leg.seq, leg.planned_arrival,
		       route.vehicle_id, route.trip_no, route.brand, route.district, route.status,
		       o.name, o.district, o.dock_type, o.parking_constraint,
		       to_char(o.window_open_time, 'HH24:MI'), to_char(o.window_close_time, 'HH24:MI'),
		       COALESCE(to_char(o.mall_window_open, 'HH24:MI'), ''), COALESCE(to_char(o.mall_window_close, 'HH24:MI'), '')
		FROM route_leg leg
		JOIN route ON route.route_id = leg.route_id
		JOIN outlet o ON o.outlet_id = leg.to_outlet
		WHERE leg.leg_id::text = $1`, legID).
		Scan(&d.LegID, &d.RouteID, &d.DepotID, &d.RouteDate, &d.ToOutlet, &d.Status,
			&d.Seq, &planned,
			&d.Route.VehicleID, &d.Route.TripNo, &d.Route.Brand, &d.Route.District, &d.Route.Status,
			&d.Outlet.Name, &d.Outlet.District, &d.Outlet.DockType, &d.Outlet.ParkingConstraint,
			&d.Outlet.WindowOpen, &d.Outlet.WindowClose, &d.Outlet.MallWindowOpen, &d.Outlet.MallWindowClose)
	if errors.Is(err, pgx.ErrNoRows) {
		return LegDetail{}, fmt.Errorf("%w: leg %s", ErrNotFound, legID)
	}
	if err != nil {
		return LegDetail{}, fmt.Errorf("load leg detail: %w", err)
	}
	d.PlannedArrival = rfc3339(planned)
	d.Route.RouteID, d.Route.RouteDate = d.RouteID, d.RouteDate
	d.Outlet.OutletID = d.ToOutlet

	rows, err := r.pool.Query(ctx, `
		SELECT o.order_id, o.order_number, o.temp_requirement, o.total_units,
		       o.total_weight_kg::float8, o.total_volume_m3::float8,
		       oi.order_item_id, i.sku, i.name, oi.quantity
		FROM route_leg leg
		JOIN customer_order o ON o.order_id = leg.order_id
		JOIN order_item oi ON oi.order_id = o.order_id
		JOIN item i ON i.item_id = oi.item_id
		WHERE leg.leg_id = $1
		ORDER BY o.order_number, i.sku`, legID)
	if err != nil {
		return LegDetail{}, fmt.Errorf("load leg orders: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var o StopOrder
		var l OrderLine
		if err := rows.Scan(&o.OrderID, &o.OrderNumber, &o.TempRequirement, &o.TotalUnits,
			&o.TotalWeightKg, &o.TotalVolumeM3, &l.OrderItemID, &l.SKU, &l.Name, &l.Quantity); err != nil {
			return LegDetail{}, fmt.Errorf("scan leg order: %w", err)
		}
		if n := len(d.Orders); n == 0 || d.Orders[n-1].OrderID != o.OrderID {
			d.Orders = append(d.Orders, o)
			d.OrderIDs = append(d.OrderIDs, o.OrderID)
		}
		last := &d.Orders[len(d.Orders)-1]
		last.Lines = append(last.Lines, l)
	}
	return d, rows.Err()
}

// ActiveRouteDate implements Repository. It resolves the depot's current run
// deterministically: the earliest non-draft/cancelled route date on or after
// today, else the most recent one. This is what lets the driver cockpit open on
// the planned run without the phone or the browser inventing a date.
func (r *PGRepository) ActiveRouteDate(ctx context.Context, depotID string, today time.Time) (string, error) {
	var date string
	err := r.pool.QueryRow(ctx, `
		SELECT d::text FROM (
			SELECT route_date AS d, 0 AS pref
			FROM route
			WHERE depot_id = $1 AND status NOT IN ('DRAFT', 'CANCELLED') AND route_date >= $2::date
			UNION ALL
			SELECT MAX(route_date) AS d, 1 AS pref
			FROM route
			WHERE depot_id = $1 AND status NOT IN ('DRAFT', 'CANCELLED')
		) runs
		WHERE d IS NOT NULL
		ORDER BY pref, d
		LIMIT 1`, depotID, today.Format("2006-01-02")).Scan(&date)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve active driver route date: %w", err)
	}
	return date, nil
}

// DriverRoutes implements Repository: the depot's live routes on a date, each
// with its stops in sequence. Draft and cancelled routes are not driveable.
func (r *PGRepository) DriverRoutes(ctx context.Context, depotID, date string) ([]DriverRoute, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT route.route_id, route.route_date::text, route.vehicle_id, route.trip_no,
		       route.brand, route.district, route.status,
		       leg.leg_id, leg.seq, leg.to_outlet, o.name,
		       to_char(o.window_open_time, 'HH24:MI'), to_char(o.window_close_time, 'HH24:MI'),
		       leg.planned_arrival, leg.status
		FROM route
		JOIN route_leg leg ON leg.route_id = route.route_id
		JOIN outlet o ON o.outlet_id = leg.to_outlet
		WHERE route.depot_id = $1 AND route.route_date = $2::date
		  AND route.status NOT IN ('DRAFT', 'CANCELLED')
		ORDER BY route.vehicle_id, route.trip_no, leg.seq`, depotID, date)
	if err != nil {
		return nil, fmt.Errorf("load driver routes: %w", err)
	}
	defer rows.Close()
	out := []DriverRoute{}
	for rows.Next() {
		var rt RouteSummary
		var s RouteStop
		var planned *time.Time
		if err := rows.Scan(&rt.RouteID, &rt.RouteDate, &rt.VehicleID, &rt.TripNo,
			&rt.Brand, &rt.District, &rt.Status,
			&s.LegID, &s.Seq, &s.OutletID, &s.OutletName, &s.WindowOpen, &s.WindowClose,
			&planned, &s.Status); err != nil {
			return nil, fmt.Errorf("scan driver route: %w", err)
		}
		s.PlannedArrival = rfc3339(planned)
		if n := len(out); n == 0 || out[n-1].RouteID != rt.RouteID {
			out = append(out, DriverRoute{RouteSummary: rt})
		}
		last := &out[len(out)-1]
		last.Stops = append(last.Stops, s)
	}
	return out, rows.Err()
}
