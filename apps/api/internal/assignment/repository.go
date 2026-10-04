package assignment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AuditSink records one audit event in the caller's transaction. It is a narrow
// interface so this package does not import internal/audit.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error
}

// pgPool is the slice of pgxpool.Pool the store uses. Narrowing it keeps the
// store testable and free of the full pool type.
type pgPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PGStore is the PostgreSQL-backed assignment store. Every write runs in a
// transaction that also writes its audit row, so a rolled-back assignment
// leaves no audit trail.
type PGStore struct {
	pool  pgPool
	audit AuditSink
}

// NewPGStore builds a store over a pool. audit may be nil in narrow tests, in
// which case no audit row is written.
func NewPGStore(p pgPool, audit AuditSink) *PGStore {
	return &PGStore{pool: p, audit: audit}
}

// ListDrivers returns active drivers, narrowed by depot when one is given.
func (s *PGStore) ListDrivers(ctx context.Context, depotID string) ([]Driver, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, COALESCE(NULLIF(display_name, ''), email), email, COALESCE(depot_id::text, '')
		FROM app_user
		WHERE role = 'DRIVER' AND is_active AND ($1 = '' OR depot_id::text = $1)
		ORDER BY COALESCE(NULLIF(display_name, ''), email), user_id`, depotID)
	if err != nil {
		return nil, fmt.Errorf("list drivers: %w", err)
	}
	defer rows.Close()
	out := make([]Driver, 0)
	for rows.Next() {
		var d Driver
		if err := rows.Scan(&d.UserID, &d.Name, &d.Email, &d.DepotID); err != nil {
			return nil, fmt.Errorf("scan driver: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListStoreManagers returns active store managers, with their current outlet.
func (s *PGStore) ListStoreManagers(ctx context.Context) ([]ManagerCandidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, COALESCE(NULLIF(display_name, ''), email), email, COALESCE(outlet_id, '')
		FROM app_user
		WHERE role = 'STORE_MANAGER' AND is_active
		ORDER BY COALESCE(NULLIF(display_name, ''), email), user_id`)
	if err != nil {
		return nil, fmt.Errorf("list store managers: %w", err)
	}
	defer rows.Close()
	out := make([]ManagerCandidate, 0)
	for rows.Next() {
		var m ManagerCandidate
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.OutletID); err != nil {
			return nil, fmt.Errorf("scan store manager: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UserByID loads one account.
func (s *PGStore) UserByID(ctx context.Context, userID string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, COALESCE(NULLIF(display_name, ''), email), email, role, is_active,
		       COALESCE(depot_id::text, ''), COALESCE(outlet_id, '')
		FROM app_user WHERE user_id = $1`, userID).
		Scan(&u.UserID, &u.Name, &u.Email, &u.Role, &u.Active, &u.DepotID, &u.OutletID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("%w: user %s", ErrNotFound, userID)
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

// VehicleDepot returns a vehicle's home depot id.
func (s *PGStore) VehicleDepot(ctx context.Context, vehicleID string) (string, error) {
	var depotID string
	err := s.pool.QueryRow(ctx, `SELECT depot_id::text FROM vehicle WHERE vehicle_id = $1`, vehicleID).Scan(&depotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: vehicle %s", ErrNotFound, vehicleID)
	}
	if err != nil {
		return "", fmt.Errorf("get vehicle depot: %w", err)
	}
	return depotID, nil
}

// OutletExists reports whether an outlet exists.
func (s *PGStore) OutletExists(ctx context.Context, outletID string) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM outlet WHERE outlet_id = $1)`, outletID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check outlet: %w", err)
	}
	return exists, nil
}

const assignmentSelect = `
	SELECT a.vehicle_id, a.assignment_date::text, a.depot_id::text,
	       u.user_id, COALESCE(NULLIF(u.display_name, ''), u.email), u.email, COALESCE(u.depot_id::text, '')
	FROM driver_vehicle_assignment a
	JOIN app_user u ON u.user_id = a.driver_id`

// AssignmentForVehicle returns the vehicle's assignment on a date.
func (s *PGStore) AssignmentForVehicle(ctx context.Context, vehicleID, date string) (VehicleAssignment, bool, error) {
	row := s.pool.QueryRow(ctx, assignmentSelect+`
		WHERE a.vehicle_id = $1 AND a.assignment_date = $2::date`, vehicleID, date)
	a, err := scanAssignment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleAssignment{}, false, nil
	}
	if err != nil {
		return VehicleAssignment{}, false, err
	}
	return a, true, nil
}

// AssignmentForDriver returns the driver's assignment on a date.
func (s *PGStore) AssignmentForDriver(ctx context.Context, driverID, date string) (VehicleAssignment, bool, error) {
	row := s.pool.QueryRow(ctx, assignmentSelect+`
		WHERE a.driver_id = $1 AND a.assignment_date = $2::date`, driverID, date)
	a, err := scanAssignment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleAssignment{}, false, nil
	}
	if err != nil {
		return VehicleAssignment{}, false, err
	}
	return a, true, nil
}

// AssignedVehicle returns the vehicle a driver is assigned to on a date, or
// ("", false, nil) when there is no assignment. It is the narrow read the driver
// workflow uses to resolve the driver's own run.
func (s *PGStore) AssignedVehicle(ctx context.Context, driverID, date string) (string, bool, error) {
	a, ok, err := s.AssignmentForDriver(ctx, driverID, date)
	if err != nil || !ok {
		return "", false, err
	}
	return a.VehicleID, true, nil
}

// AssignVehicle upserts the assignment for its vehicle+date and audits it.
func (s *PGStore) AssignVehicle(ctx context.Context, a VehicleAssignment, actor string) (VehicleAssignment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return VehicleAssignment{}, fmt.Errorf("begin assign driver: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The previous driver on this vehicle/date, if any, so the audit records a
	// change rather than just an insert.
	var previousDriver *string
	var prev string
	err = tx.QueryRow(ctx, `
		SELECT driver_id FROM driver_vehicle_assignment
		WHERE vehicle_id = $1 AND assignment_date = $2::date FOR UPDATE`, a.VehicleID, a.AssignmentDate).Scan(&prev)
	if err == nil {
		previousDriver = &prev
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return VehicleAssignment{}, fmt.Errorf("load prior assignment: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO driver_vehicle_assignment (driver_id, vehicle_id, assignment_date, depot_id, assigned_by)
		VALUES ($1, $2, $3::date, $4, $5)
		ON CONFLICT (vehicle_id, assignment_date) DO UPDATE
		SET driver_id = EXCLUDED.driver_id, assigned_by = EXCLUDED.assigned_by, created_at = now()`,
		a.Driver.UserID, a.VehicleID, a.AssignmentDate, a.DepotID, actor); err != nil {
		if isUniqueViolation(err) {
			return VehicleAssignment{}, fmt.Errorf("%w: driver or vehicle already assigned on %s", ErrConflict, a.AssignmentDate)
		}
		return VehicleAssignment{}, fmt.Errorf("assign vehicle: %w", err)
	}

	detail := map[string]any{"driverId": a.Driver.UserID, "date": a.AssignmentDate}
	if previousDriver != nil && *previousDriver != a.Driver.UserID {
		detail["previousDriverId"] = *previousDriver
	}
	if err := s.recordAudit(ctx, tx, "DRIVER_ASSIGNED", "DRIVER_VEHICLE_ASSIGNMENT", a.VehicleID, actor, a.DepotID, "", detail); err != nil {
		return VehicleAssignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return VehicleAssignment{}, fmt.Errorf("commit assign driver: %w", err)
	}
	return a, nil
}

// UnassignVehicle removes the assignment for a vehicle+date and audits it.
func (s *PGStore) UnassignVehicle(ctx context.Context, vehicleID, date, actor string) (VehicleAssignment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return VehicleAssignment{}, fmt.Errorf("begin unassign driver: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var a VehicleAssignment
	err = tx.QueryRow(ctx, `
		SELECT a.vehicle_id, a.assignment_date::text, a.depot_id::text,
		       u.user_id, COALESCE(NULLIF(u.display_name, ''), u.email), u.email, COALESCE(u.depot_id::text, '')
		FROM driver_vehicle_assignment a
		JOIN app_user u ON u.user_id = a.driver_id
		WHERE a.vehicle_id = $1 AND a.assignment_date = $2::date
		FOR UPDATE OF a`, vehicleID, date).
		Scan(&a.VehicleID, &a.AssignmentDate, &a.DepotID, &a.Driver.UserID, &a.Driver.Name, &a.Driver.Email, &a.Driver.DepotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return VehicleAssignment{}, fmt.Errorf("%w: no assignment for %s on %s", ErrNotFound, vehicleID, date)
	}
	if err != nil {
		return VehicleAssignment{}, fmt.Errorf("load assignment to remove: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM driver_vehicle_assignment WHERE vehicle_id = $1 AND assignment_date = $2::date`,
		vehicleID, date); err != nil {
		return VehicleAssignment{}, fmt.Errorf("unassign vehicle: %w", err)
	}
	if err := s.recordAudit(ctx, tx, "DRIVER_UNASSIGNED", "DRIVER_VEHICLE_ASSIGNMENT", vehicleID, actor, a.DepotID, "", map[string]any{
		"driverId": a.Driver.UserID, "date": a.AssignmentDate,
	}); err != nil {
		return VehicleAssignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return VehicleAssignment{}, fmt.Errorf("commit unassign driver: %w", err)
	}
	return a, nil
}

// ManagerForOutlet returns the store manager responsible for an outlet.
func (s *PGStore) ManagerForOutlet(ctx context.Context, outletID string) (OutletManager, bool, error) {
	var m OutletManager
	err := s.pool.QueryRow(ctx, `
		SELECT outlet_id, user_id, COALESCE(NULLIF(display_name, ''), email), email, COALESCE(depot_id::text, '')
		FROM app_user
		WHERE role = 'STORE_MANAGER' AND outlet_id = $1
		ORDER BY user_id
		LIMIT 1`, outletID).
		Scan(&m.OutletID, &m.UserID, &m.Name, &m.Email, &m.DepotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutletManager{}, false, nil
	}
	if err != nil {
		return OutletManager{}, false, fmt.Errorf("get outlet manager: %w", err)
	}
	return m, true, nil
}

// SetManager makes userID the outlet's manager, unassigning any other manager
// on it in the same transaction, and audits both changes.
func (s *PGStore) SetManager(ctx context.Context, outletID, userID, actor string) (OutletManager, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OutletManager{}, fmt.Errorf("begin assign manager: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// An outlet holds one manager: release whoever else is on it.
	var replacedID, replacedName string
	err = tx.QueryRow(ctx, `
		SELECT user_id, COALESCE(NULLIF(display_name, ''), email)
		FROM app_user
		WHERE role = 'STORE_MANAGER' AND outlet_id = $1 AND user_id <> $2
		ORDER BY user_id LIMIT 1
		FOR UPDATE`, outletID, userID).Scan(&replacedID, &replacedName)
	switch {
	case err == nil:
		if _, err := tx.Exec(ctx, `
			UPDATE app_user SET outlet_id = NULL
			WHERE user_id = $1 AND role = 'STORE_MANAGER'`, replacedID); err != nil {
			return OutletManager{}, fmt.Errorf("release previous manager: %w", err)
		}
		if err := s.recordAudit(ctx, tx, "MANAGER_UNASSIGNED", "OUTLET_MANAGER", outletID, actor, "", outletID, map[string]any{
			"userId": replacedID, "replacedBy": userID,
		}); err != nil {
			return OutletManager{}, err
		}
	case errors.Is(err, pgx.ErrNoRows):
		// No other manager; nothing to release.
	default:
		return OutletManager{}, fmt.Errorf("load current manager: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE app_user SET outlet_id = $1
		WHERE user_id = $2 AND role = 'STORE_MANAGER' AND is_active`, outletID, userID)
	if err != nil {
		return OutletManager{}, fmt.Errorf("set outlet manager: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return OutletManager{}, fmt.Errorf("%w: user %s", ErrNotFound, userID)
	}

	if err := s.recordAudit(ctx, tx, "MANAGER_ASSIGNED", "OUTLET_MANAGER", outletID, actor, "", outletID, map[string]any{
		"userId": userID,
	}); err != nil {
		return OutletManager{}, err
	}

	var m OutletManager
	if err := tx.QueryRow(ctx, `
		SELECT outlet_id, user_id, COALESCE(NULLIF(display_name, ''), email), email, COALESCE(depot_id::text, '')
		FROM app_user WHERE user_id = $1`, userID).
		Scan(&m.OutletID, &m.UserID, &m.Name, &m.Email, &m.DepotID); err != nil {
		return OutletManager{}, fmt.Errorf("read assigned manager: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return OutletManager{}, fmt.Errorf("commit assign manager: %w", err)
	}
	return m, nil
}

// ClearManager removes the store manager currently on an outlet and audits it.
func (s *PGStore) ClearManager(ctx context.Context, outletID, actor string) (OutletManager, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OutletManager{}, fmt.Errorf("begin unassign manager: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var m OutletManager
	err = tx.QueryRow(ctx, `
		SELECT outlet_id, user_id, COALESCE(NULLIF(display_name, ''), email), email, COALESCE(depot_id::text, '')
		FROM app_user
		WHERE role = 'STORE_MANAGER' AND outlet_id = $1
		ORDER BY user_id LIMIT 1
		FOR UPDATE`, outletID).
		Scan(&m.OutletID, &m.UserID, &m.Name, &m.Email, &m.DepotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutletManager{}, fmt.Errorf("%w: outlet %s has no manager", ErrNotFound, outletID)
	}
	if err != nil {
		return OutletManager{}, fmt.Errorf("load manager to remove: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE app_user SET outlet_id = NULL
		WHERE user_id = $1 AND role = 'STORE_MANAGER'`, m.UserID); err != nil {
		return OutletManager{}, fmt.Errorf("clear outlet manager: %w", err)
	}
	if err := s.recordAudit(ctx, tx, "MANAGER_UNASSIGNED", "OUTLET_MANAGER", outletID, actor, "", outletID, map[string]any{
		"userId": m.UserID,
	}); err != nil {
		return OutletManager{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OutletManager{}, fmt.Errorf("commit unassign manager: %w", err)
	}
	return m, nil
}

// scanAssignment reads one driver-vehicle assignment row.
func scanAssignment(row pgx.Row) (VehicleAssignment, error) {
	var a VehicleAssignment
	if err := row.Scan(&a.VehicleID, &a.AssignmentDate, &a.DepotID,
		&a.Driver.UserID, &a.Driver.Name, &a.Driver.Email, &a.Driver.DepotID); err != nil {
		return VehicleAssignment{}, err
	}
	return a, nil
}

// recordAudit writes an audit row in the caller's transaction when a sink is
// configured.
func (s *PGStore) recordAudit(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID string, detail map[string]any) error {
	if s.audit == nil {
		return nil
	}
	if err := s.audit.RecordTx(ctx, tx, action, entityType, entityID, actor, depotID, outletID, "SUCCESS", detail); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
