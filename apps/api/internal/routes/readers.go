package routes

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"waypoint.lk/api/internal/domain"
)

// PGReaders wraps the read-only queries confirmation needs. It is one type so
// the composition root wires a single dependency; each method is a narrow reader
// the service declares as an interface.
type PGReaders struct {
	pool *pgxpool.Pool
}

// NewPGReaders builds the readers over the given pool.
func NewPGReaders(pool *pgxpool.Pool) *PGReaders { return &PGReaders{pool: pool} }

// LoadOrderFacts implements OrderReader.
func (r *PGReaders) LoadOrderFacts(ctx context.Context, orderID string) (OrderFacts, error) {
	var f OrderFacts
	err := r.pool.QueryRow(ctx, `
		SELECT o.order_id, o.outlet_id, ou.district, ou.depot_id, o.brand, o.status,
		       o.total_weight_kg, o.total_volume_m3
		FROM customer_order o
		JOIN outlet ou ON ou.outlet_id = o.outlet_id
		WHERE o.order_id = $1`, orderID).
		Scan(&f.OrderID, &f.OutletID, &f.District, &f.DepotID, &f.Brand, &f.Status,
			&f.TotalWeightKg, &f.TotalVolumeM3)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderFacts{}, fmt.Errorf("%w: order %s", ErrNotFound, orderID)
	}
	if err != nil {
		return OrderFacts{}, fmt.Errorf("load order facts: %w", err)
	}
	return f, nil
}

// VehicleDepot implements VehicleReader.
func (r *PGReaders) VehicleDepot(ctx context.Context, vehicleID string) (string, error) {
	var depotID string
	err := r.pool.QueryRow(ctx, `SELECT depot_id FROM vehicle WHERE vehicle_id = $1`, vehicleID).Scan(&depotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: vehicle %s", ErrNotFound, vehicleID)
	}
	if err != nil {
		return "", fmt.Errorf("load vehicle depot: %w", err)
	}
	return depotID, nil
}

// OutletFor implements ReferenceReader.
func (r *PGReaders) OutletFor(ctx context.Context, outletID string) (OutletRef, error) {
	var o OutletRef
	err := r.pool.QueryRow(ctx, `SELECT outlet_id, dock_type FROM outlet WHERE outlet_id = $1`, outletID).
		Scan(&o.OutletID, &o.DockType)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutletRef{}, fmt.Errorf("%w: outlet %s", ErrNotFound, outletID)
	}
	if err != nil {
		return OutletRef{}, fmt.Errorf("load outlet: %w", err)
	}
	return o, nil
}

// Travel implements ReferenceReader.
func (r *PGReaders) Travel(ctx context.Context, depotID, district string) (TravelRef, error) {
	var t TravelRef
	err := r.pool.QueryRow(ctx, `
		SELECT depot_to_district_km, depot_to_district_freeflow_min, inter_stop_km, inter_stop_freeflow_min
		FROM district_travel WHERE depot_id = $1 AND district = $2`, depotID, district).
		Scan(&t.DepotToDistrictKm, &t.DepotToDistrictMin, &t.InterStopKm, &t.InterStopMin)
	if errors.Is(err, pgx.ErrNoRows) {
		return TravelRef{}, fmt.Errorf("%w: no travel reference for %s/%s", ErrNotFound, depotID, district)
	}
	if err != nil {
		return TravelRef{}, fmt.Errorf("load travel: %w", err)
	}
	return t, nil
}

// ServiceAllowance implements ReferenceReader.
func (r *PGReaders) ServiceAllowance(ctx context.Context, brand domain.Brand, dockType domain.DockType) (int, error) {
	var mins int
	err := r.pool.QueryRow(ctx, `
		SELECT service_allowance_min FROM service_allowance WHERE brand = $1 AND dock_type = $2`,
		string(brand), string(dockType)).Scan(&mins)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: no service allowance for %s/%s", ErrNotFound, brand, dockType)
	}
	if err != nil {
		return 0, fmt.Errorf("load service allowance: %w", err)
	}
	return mins, nil
}
