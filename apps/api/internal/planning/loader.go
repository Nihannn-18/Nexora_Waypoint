package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGLoader loads planning inputs from PostgreSQL. It reads only; the engine
// performs no I/O of its own.
type PGLoader struct {
	pool *pgxpool.Pool
}

// NewPGLoader builds an input loader over the given pool.
func NewPGLoader(pool *pgxpool.Pool) *PGLoader {
	return &PGLoader{pool: pool}
}

// LoadInput assembles everything a run for one depot and date needs. Every query
// that can affect the plan has an explicit ORDER BY, so a run is reproducible.
func (l *PGLoader) LoadInput(ctx context.Context, planningDate time.Time, depotID string) (Input, error) {
	in := Input{
		PlanningDate:      planningDate,
		DepotID:           depotID,
		Outlets:           map[string]Outlet{},
		Items:             map[string]Item{},
		Travel:            map[string]Travel{},
		ServiceAllowances: map[string]int{},
		FuelUsedL:         map[string]float64{},
		FuelQuotaL:        map[string]float64{},
	}

	if err := l.loadCalendar(ctx, planningDate, &in); err != nil {
		return Input{}, err
	}
	if err := l.loadOutlets(ctx, &in); err != nil {
		return Input{}, err
	}
	if err := l.loadTravel(ctx, &in); err != nil {
		return Input{}, err
	}
	if err := l.loadServiceAllowances(ctx, &in); err != nil {
		return Input{}, err
	}
	if err := l.loadVehicles(ctx, planningDate, depotID, &in); err != nil {
		return Input{}, err
	}
	if err := l.loadOrders(ctx, planningDate, depotID, &in); err != nil {
		return Input{}, err
	}
	return in, nil
}

func (l *PGLoader) loadCalendar(ctx context.Context, d time.Time, in *Input) error {
	err := l.pool.QueryRow(ctx, `
		SELECT date, is_operating FROM calendar_day WHERE date = $1`, d).
		Scan(&in.Calendar.Date, &in.Calendar.IsOperating)
	if err != nil {
		return fmt.Errorf("load calendar day: %w", err)
	}
	return nil
}

func (l *PGLoader) loadOutlets(ctx context.Context, in *Input) error {
	rows, err := l.pool.Query(ctx, `
		SELECT outlet_id, brand, district, depot_id, dock_type, parking_constraint,
		       COALESCE(to_char(mall_window_open, 'HH24:MI'), ''),
		       COALESCE(to_char(mall_window_close, 'HH24:MI'), ''),
		       to_char(window_open_time, 'HH24:MI'),
		       to_char(window_close_time, 'HH24:MI')
		FROM outlet ORDER BY outlet_id`)
	if err != nil {
		return fmt.Errorf("load outlets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var o Outlet
		var depotUUID string
		if err := rows.Scan(&o.OutletID, &o.Brand, &o.District, &depotUUID, &o.DockType,
			&o.ParkingConstraint, &o.MallWindowOpen, &o.MallWindowClose, &o.WindowOpen, &o.WindowClose); err != nil {
			return fmt.Errorf("scan outlet: %w", err)
		}
		o.DepotID = depotUUID
		in.Outlets[o.OutletID] = o
	}
	return rows.Err()
}

func (l *PGLoader) loadTravel(ctx context.Context, in *Input) error {
	rows, err := l.pool.Query(ctx, `
		SELECT district, depot_id, depot_to_district_km, depot_to_district_freeflow_min,
		       inter_stop_km, inter_stop_freeflow_min
		FROM district_travel ORDER BY depot_id, district`)
	if err != nil {
		return fmt.Errorf("load district travel: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t Travel
		var depotUUID string
		if err := rows.Scan(&t.District, &depotUUID, &t.DepotToDistrictKm,
			&t.DepotToDistrictMin, &t.InterStopKm, &t.InterStopMin); err != nil {
			return fmt.Errorf("scan district travel: %w", err)
		}
		t.DepotID = depotUUID
		in.Travel[depotUUID+"|"+t.District] = t
	}
	return rows.Err()
}

func (l *PGLoader) loadServiceAllowances(ctx context.Context, in *Input) error {
	rows, err := l.pool.Query(ctx, `
		SELECT brand, dock_type, service_allowance_min
		FROM service_allowance ORDER BY brand, dock_type`)
	if err != nil {
		return fmt.Errorf("load service allowances: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var brand, dock string
		var mins int
		if err := rows.Scan(&brand, &dock, &mins); err != nil {
			return fmt.Errorf("scan service allowance: %w", err)
		}
		in.ServiceAllowances[brand+"|"+dock] = mins
	}
	return rows.Err()
}

func (l *PGLoader) loadVehicles(ctx context.Context, planningDate time.Time, depotID string, in *Input) error {
	// Availability defaults to available when no row exists for the date, then
	// is overridden by the explicit vehicle_daily_availability row. A vehicle is
	// included only when its home depot matches.
	rows, err := l.pool.Query(ctx, `
		SELECT v.vehicle_id, v.type, v.temp, v.weight_cap_kg, v.volume_cap_m3, v.km_per_l,
		       COALESCE(a.available, TRUE)
		FROM vehicle v
		LEFT JOIN vehicle_daily_availability a ON a.vehicle_id = v.vehicle_id AND a.date = $1
		WHERE v.depot_id = $2
		ORDER BY v.vehicle_id`, planningDate, depotID)
	if err != nil {
		return fmt.Errorf("load vehicles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v Vehicle
		if err := rows.Scan(&v.VehicleID, &v.Type, &v.TempClass, &v.WeightCapKg,
			&v.VolumeCapM3, &v.KmPerL, &v.Available); err != nil {
			return fmt.Errorf("scan vehicle: %w", err)
		}
		v.DepotID = depotID
		in.Vehicles = append(in.Vehicles, v)
	}
	return rows.Err()
}

func (l *PGLoader) loadOrders(ctx context.Context, planningDate time.Time, depotID string, in *Input) error {
	// Eligible orders: CONFIRMED, requested for the planning date, at an outlet
	// in this depot. Ordered by outlet then order number for determinism.
	rows, err := l.pool.Query(ctx, `
		SELECT o.order_id, o.order_number, o.outlet_id, o.brand, ou.district, ou.depot_id,
		       o.total_weight_kg, o.total_volume_m3, o.temp_requirement, ou.parking_constraint,
		       to_char(ou.window_open_time, 'HH24:MI'), to_char(ou.window_close_time, 'HH24:MI'),
		       COALESCE(to_char(ou.mall_window_open, 'HH24:MI'), ''),
		       COALESCE(to_char(ou.mall_window_close, 'HH24:MI'), ''),
		       o.deferred_yesterday, COALESCE(o.days_since_last_served, 0)
		FROM customer_order o
		JOIN outlet ou ON ou.outlet_id = o.outlet_id
		WHERE o.status = 'CONFIRMED'
		  AND o.requested_delivery_date = $1
		  AND ou.depot_id = $2
		ORDER BY o.outlet_id, o.order_number, o.order_id`, planningDate, depotID)
	if err != nil {
		return fmt.Errorf("load orders: %w", err)
	}
	defer rows.Close()

	seq := 0
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.OrderID, &o.OrderNumber, &o.OutletID, &o.Brand, &o.District,
			&o.DepotID, &o.TotalWeightKg, &o.TotalVolumeM3, &o.TempRequirement,
			&o.ParkingConstraint, &o.WindowOpen, &o.WindowClose, &o.MallWindowOpen,
			&o.MallWindowClose, &o.DeferredYesterday, &o.DaysSinceLastServed); err != nil {
			return fmt.Errorf("scan order: %w", err)
		}
		o.RequestedDeliveryDate = planningDate
		o.Sequence = seq
		seq++
		in.Orders = append(in.Orders, o)
	}
	return rows.Err()
}
