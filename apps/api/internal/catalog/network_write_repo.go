package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Vehicle/outlet persistence. Each mutation is a single transaction that writes
// the row and its audit record together. Identity is immutable on update: the
// UPDATE statements never touch the primary key, so a historical route or order
// that references the id keeps its referent.

// GetVehicle reads one vehicle and its status on the API clock's date.
func (w *PGNetworkWriter) GetVehicle(ctx context.Context, vehicleID string) (VehicleDetail, error) {
	var v VehicleDetail
	err := w.pool.QueryRow(ctx, `
		SELECT vehicle_id, type, temp, weight_cap_kg::float8, volume_cap_m3::float8,
		       fuel_type, km_per_l::float8, weekly_fuel_quota_l::float8, depot_id::text
		FROM vehicle WHERE vehicle_id = $1`, vehicleID).
		Scan(&v.VehicleID, &v.Type, &v.TempClass, &v.WeightCapKg, &v.VolumeCapM3,
			&v.FuelType, &v.KmPerL, &v.WeeklyFuelQuotaL, &v.DepotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleDetail{}, fmt.Errorf("%w: vehicle %s", ErrNotFound, vehicleID)
	}
	if err != nil {
		return VehicleDetail{}, fmt.Errorf("get vehicle: %w", err)
	}
	// A vehicle with no availability row for a date is available (the same
	// convention the planning loader and the read endpoint use).
	if err := w.pool.QueryRow(ctx, `
		SELECT COALESCE(status, 'AVAILABLE')
		FROM vehicle v
		LEFT JOIN vehicle_daily_availability a
		  ON a.vehicle_id = v.vehicle_id AND a.date = CURRENT_DATE
		WHERE v.vehicle_id = $1`, vehicleID).Scan(&v.Status); err != nil {
		return VehicleDetail{}, fmt.Errorf("get vehicle status: %w", err)
	}
	return v, nil
}

// CreateVehicle inserts a vehicle and audits the creation in one transaction.
func (w *PGNetworkWriter) CreateVehicle(ctx context.Context, v VehicleWrite, actor string) (VehicleDetail, error) {
	if !validVehicleID(v.VehicleID) {
		return VehicleDetail{}, ValidationError{Field: "vehicleId", Message: "must match VEHyynn"}
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return VehicleDetail{}, fmt.Errorf("begin create vehicle: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vehicle WHERE vehicle_id = $1)`, v.VehicleID).Scan(&exists); err != nil {
		return VehicleDetail{}, fmt.Errorf("check vehicle identity: %w", err)
	}
	if exists {
		return VehicleDetail{}, fmt.Errorf("%w: vehicle %s", ErrDuplicate, v.VehicleID)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO vehicle (vehicle_id, type, temp, weight_cap_kg, volume_cap_m3,
		                     fuel_type, km_per_l, weekly_fuel_quota_l, depot_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		v.VehicleID, string(v.Type), string(v.TempClass), v.WeightCapKg, v.VolumeCapM3,
		strings.TrimSpace(v.FuelType), v.KmPerL, v.WeeklyFuelQuotaL, v.DepotID); err != nil {
		return VehicleDetail{}, fmt.Errorf("insert vehicle: %w", err)
	}

	if err := w.recordAudit(ctx, tx, "VEHICLE_CREATED", "VEHICLE", v.VehicleID, actor, v.DepotID, "", map[string]any{
		"type": string(v.Type), "tempClass": string(v.TempClass),
		"weightCapKg": v.WeightCapKg, "volumeCapM3": v.VolumeCapM3,
		"fuelType": strings.TrimSpace(v.FuelType), "kmPerL": v.KmPerL,
		"weeklyFuelQuotaL": v.WeeklyFuelQuotaL,
	}); err != nil {
		return VehicleDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return VehicleDetail{}, fmt.Errorf("commit create vehicle: %w", err)
	}
	return w.GetVehicle(ctx, v.VehicleID)
}

// UpdateVehicle updates the mutable fields of a vehicle (never its id or depot)
// and audits the change in one transaction, recording a compact before/after.
func (w *PGNetworkWriter) UpdateVehicle(ctx context.Context, v VehicleWrite, actor string) (VehicleDetail, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return VehicleDetail{}, fmt.Errorf("begin update vehicle: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := scanVehicleForUpdate(ctx, tx, v.VehicleID)
	if err != nil {
		return VehicleDetail{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vehicle
		SET type = $2, temp = $3, weight_cap_kg = $4, volume_cap_m3 = $5,
		    fuel_type = $6, km_per_l = $7, weekly_fuel_quota_l = $8, depot_id = $9
		WHERE vehicle_id = $1`,
		v.VehicleID, string(v.Type), string(v.TempClass), v.WeightCapKg, v.VolumeCapM3,
		strings.TrimSpace(v.FuelType), v.KmPerL, v.WeeklyFuelQuotaL, v.DepotID); err != nil {
		return VehicleDetail{}, fmt.Errorf("update vehicle: %w", err)
	}

	if err := w.recordAudit(ctx, tx, "VEHICLE_UPDATED", "VEHICLE", v.VehicleID, actor, v.DepotID, "", map[string]any{
		"before": vehicleAuditDetail(before),
		"after": vehicleAuditDetail(VehicleWrite{
			VehicleID: v.VehicleID, Type: v.Type, TempClass: v.TempClass,
			WeightCapKg: v.WeightCapKg, VolumeCapM3: v.VolumeCapM3,
			FuelType: strings.TrimSpace(v.FuelType), KmPerL: v.KmPerL,
			WeeklyFuelQuotaL: v.WeeklyFuelQuotaL, DepotID: v.DepotID,
		}),
	}); err != nil {
		return VehicleDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return VehicleDetail{}, fmt.Errorf("commit update vehicle: %w", err)
	}
	return w.GetVehicle(ctx, v.VehicleID)
}

// GetOutlet reads one outlet.
func (w *PGNetworkWriter) GetOutlet(ctx context.Context, outletID string) (OutletDetail, error) {
	var o OutletDetail
	var mallOpen, mallClose *string
	err := w.pool.QueryRow(ctx, `
		SELECT outlet_id, name, brand, district, depot_id::text, dock_type, parking_constraint,
		       to_char(window_open_time, 'HH24:MI'), to_char(window_close_time, 'HH24:MI'),
		       to_char(mall_window_open, 'HH24:MI'), to_char(mall_window_close, 'HH24:MI')
		FROM outlet WHERE outlet_id = $1`, outletID).
		Scan(&o.OutletID, &o.Name, &o.Brand, &o.District, &o.DepotID, &o.DockType,
			&o.ParkingConstraint, &o.WindowOpenTime, &o.WindowCloseTime, &mallOpen, &mallClose)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutletDetail{}, fmt.Errorf("%w: outlet %s", ErrNotFound, outletID)
	}
	if err != nil {
		return OutletDetail{}, fmt.Errorf("get outlet: %w", err)
	}
	if mallOpen != nil {
		o.MallWindowOpen = *mallOpen
	}
	if mallClose != nil {
		o.MallWindowClose = *mallClose
	}
	return o, nil
}

// CreateOutlet inserts an outlet and audits the creation in one transaction.
func (w *PGNetworkWriter) CreateOutlet(ctx context.Context, o OutletWrite, actor string) (OutletDetail, error) {
	if !validOutletID(o.OutletID) {
		return OutletDetail{}, ValidationError{Field: "outletId", Message: "must match OUTnnn"}
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return OutletDetail{}, fmt.Errorf("begin create outlet: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM outlet WHERE outlet_id = $1)`, o.OutletID).Scan(&exists); err != nil {
		return OutletDetail{}, fmt.Errorf("check outlet identity: %w", err)
	}
	if exists {
		return OutletDetail{}, fmt.Errorf("%w: outlet %s", ErrDuplicate, o.OutletID)
	}

	mallOpen, mallClose := mallWindowValues(o)
	if _, err := tx.Exec(ctx, `
		INSERT INTO outlet (outlet_id, name, brand, district, depot_id, dock_type,
		                    parking_constraint, mall_window_open, mall_window_close,
		                    window_open_time, window_close_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		o.OutletID, strings.TrimSpace(o.Name), string(o.Brand), strings.TrimSpace(o.District),
		o.DepotID, string(o.DockType), string(o.ParkingConstraint),
		mallOpen, mallClose, o.WindowOpenTime, o.WindowCloseTime); err != nil {
		return OutletDetail{}, fmt.Errorf("insert outlet: %w", err)
	}

	if err := w.recordAudit(ctx, tx, "OUTLET_CREATED", "OUTLET", o.OutletID, actor, o.DepotID, o.OutletID, map[string]any{
		"name": o.Name, "brand": string(o.Brand), "district": o.District,
		"dockType": string(o.DockType), "parkingConstraint": string(o.ParkingConstraint),
		"windowOpenTime": o.WindowOpenTime, "windowCloseTime": o.WindowCloseTime,
	}); err != nil {
		return OutletDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return OutletDetail{}, fmt.Errorf("commit create outlet: %w", err)
	}
	return w.GetOutlet(ctx, o.OutletID)
}

// UpdateOutlet updates the mutable fields of an outlet (never its id) and audits
// the change in one transaction.
func (w *PGNetworkWriter) UpdateOutlet(ctx context.Context, o OutletWrite, actor string) (OutletDetail, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return OutletDetail{}, fmt.Errorf("begin update outlet: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := scanOutletForUpdate(ctx, tx, o.OutletID)
	if err != nil {
		return OutletDetail{}, err
	}

	mallOpen, mallClose := mallWindowValues(o)
	if _, err := tx.Exec(ctx, `
		UPDATE outlet
		SET name = $2, brand = $3, district = $4, depot_id = $5, dock_type = $6,
		    parking_constraint = $7, mall_window_open = $8, mall_window_close = $9,
		    window_open_time = $10, window_close_time = $11
		WHERE outlet_id = $1`,
		o.OutletID, strings.TrimSpace(o.Name), string(o.Brand), strings.TrimSpace(o.District),
		o.DepotID, string(o.DockType), string(o.ParkingConstraint),
		mallOpen, mallClose, o.WindowOpenTime, o.WindowCloseTime); err != nil {
		return OutletDetail{}, fmt.Errorf("update outlet: %w", err)
	}

	if err := w.recordAudit(ctx, tx, "OUTLET_UPDATED", "OUTLET", o.OutletID, actor, o.DepotID, o.OutletID, map[string]any{
		"before": outletAuditDetail(before),
		"after": outletAuditDetail(OutletWrite{
			OutletID: o.OutletID, Name: strings.TrimSpace(o.Name), Brand: o.Brand,
			District: strings.TrimSpace(o.District), DepotID: o.DepotID,
			DockType: o.DockType, ParkingConstraint: o.ParkingConstraint,
			WindowOpenTime: o.WindowOpenTime, WindowCloseTime: o.WindowCloseTime,
			MallWindowOpen: o.MallWindowOpen, MallWindowClose: o.MallWindowClose,
		}),
	}); err != nil {
		return OutletDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return OutletDetail{}, fmt.Errorf("commit update outlet: %w", err)
	}
	return w.GetOutlet(ctx, o.OutletID)
}

// recordAudit writes an audit row in the caller's transaction when a sink is
// configured.
func (w *PGNetworkWriter) recordAudit(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID string, detail map[string]any) error {
	if w.audit == nil {
		return nil
	}
	if err := w.audit.RecordTx(ctx, tx, action, entityType, entityID, actor, depotID, outletID, "SUCCESS", detail); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

// mallWindowValues returns the mall window as nullable strings: both empty for a
// non-mall outlet, both set otherwise (validation has already checked pairing).
func mallWindowValues(o OutletWrite) (*string, *string) {
	if strings.TrimSpace(o.MallWindowOpen) == "" {
		return nil, nil
	}
	return &o.MallWindowOpen, &o.MallWindowClose
}

// scanVehicleForUpdate reads the current mutable vehicle fields inside the tx.
func scanVehicleForUpdate(ctx context.Context, tx pgx.Tx, vehicleID string) (VehicleWrite, error) {
	var v VehicleWrite
	err := tx.QueryRow(ctx, `
		SELECT vehicle_id, type, temp, weight_cap_kg::float8, volume_cap_m3::float8,
		       fuel_type, km_per_l::float8, weekly_fuel_quota_l::float8, depot_id::text
		FROM vehicle WHERE vehicle_id = $1`, vehicleID).
		Scan(&v.VehicleID, &v.Type, &v.TempClass, &v.WeightCapKg, &v.VolumeCapM3,
			&v.FuelType, &v.KmPerL, &v.WeeklyFuelQuotaL, &v.DepotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleWrite{}, fmt.Errorf("%w: vehicle %s", ErrNotFound, vehicleID)
	}
	if err != nil {
		return VehicleWrite{}, fmt.Errorf("load vehicle for update: %w", err)
	}
	return v, nil
}

// scanOutletForUpdate reads the current mutable outlet fields inside the tx.
func scanOutletForUpdate(ctx context.Context, tx pgx.Tx, outletID string) (OutletWrite, error) {
	var o OutletWrite
	var mallOpen, mallClose *string
	err := tx.QueryRow(ctx, `
		SELECT outlet_id, name, brand, district, depot_id::text, dock_type, parking_constraint,
		       to_char(window_open_time, 'HH24:MI'), to_char(window_close_time, 'HH24:MI'),
		       to_char(mall_window_open, 'HH24:MI'), to_char(mall_window_close, 'HH24:MI')
		FROM outlet WHERE outlet_id = $1`, outletID).
		Scan(&o.OutletID, &o.Name, &o.Brand, &o.District, &o.DepotID, &o.DockType,
			&o.ParkingConstraint, &o.WindowOpenTime, &o.WindowCloseTime, &mallOpen, &mallClose)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutletWrite{}, fmt.Errorf("%w: outlet %s", ErrNotFound, outletID)
	}
	if err != nil {
		return OutletWrite{}, fmt.Errorf("load outlet for update: %w", err)
	}
	if mallOpen != nil {
		o.MallWindowOpen = *mallOpen
	}
	if mallClose != nil {
		o.MallWindowClose = *mallClose
	}
	return o, nil
}

// vehicleAuditDetail reduces a vehicle to the facts worth auditing.
func vehicleAuditDetail(v VehicleWrite) map[string]any {
	return map[string]any{
		"type": string(v.Type), "tempClass": string(v.TempClass),
		"weightCapKg": v.WeightCapKg, "volumeCapM3": v.VolumeCapM3,
		"fuelType": v.FuelType, "kmPerL": v.KmPerL,
		"weeklyFuelQuotaL": v.WeeklyFuelQuotaL, "depotId": v.DepotID,
	}
}

// outletAuditDetail reduces an outlet to the facts worth auditing.
func outletAuditDetail(o OutletWrite) map[string]any {
	return map[string]any{
		"name": o.Name, "brand": string(o.Brand), "district": o.District,
		"depotId": o.DepotID, "dockType": string(o.DockType),
		"parkingConstraint": string(o.ParkingConstraint),
		"windowOpenTime":    o.WindowOpenTime, "windowCloseTime": o.WindowCloseTime,
		"mallWindowOpen": o.MallWindowOpen, "mallWindowClose": o.MallWindowClose,
	}
}
