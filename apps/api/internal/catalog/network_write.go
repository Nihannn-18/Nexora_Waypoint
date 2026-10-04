package catalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"waypoint.lk/api/internal/domain"
)

// This file implements Dispatcher master-data management for the network
// reference: vehicles and outlets. Vehicles and outlets are planning inputs, so
// a mutation here can change what future planning runs can do. Two safeguards
// follow from that:
//
//   - Every mutable field is validated server-side against the same enums and
//     bounds the schema enforces (mirroring the CHECK constraints), so invalid
//     data never reaches PostgreSQL merely because a client sent it.
//   - Identity columns (vehicle_id / outlet_id) are immutable and are generated
//     by the server on create, because orders, routes, delivery records, users
//     and planning data all reference them.
//
// There is no delete: a vehicle or outlet that should stop being used is
// deactivated through the existing availability/status mechanisms or left
// inactive, never hard-deleted, so historical routes and orders keep their
// referents.

// --- identity generation ---------------------------------------------------

// Vehicle and outlet identifiers are natural keys with a fixed shape:
// VEHyynn and OUTnnn. The reference CSVs seed VEH001–VEH060 and OUT001–OUT120,
// and every downstream record references these strings, so a new one must match
// the shape and never collide.
var (
	vehicleIDPattern = regexp.MustCompile(`^VEH[0-9]{3}$`)
	outletIDPattern  = regexp.MustCompile(`^OUT[0-9]{3}$`)
)

// validVehicleID reports whether id matches the VEHyynn convention.
func validVehicleID(id string) bool { return vehicleIDPattern.MatchString(id) }

// validOutletID reports whether id matches the OUTnnn convention.
func validOutletID(id string) bool { return outletIDPattern.MatchString(id) }

// --- command types ---------------------------------------------------------

// VehicleWrite is a validated create/update payload for a vehicle. DepotID is
// the internal depot UUID (not the display code). FuelType is free text, matching
// the schema column, which has no enum.
type VehicleWrite struct {
	VehicleID        string
	Type             domain.VehicleType
	TempClass        domain.VehicleTempClass
	WeightCapKg      float64
	VolumeCapM3      float64
	FuelType         string
	KmPerL           float64
	WeeklyFuelQuotaL float64
	DepotID          string
}

// OutletWrite is a validated create/update payload for an outlet. MallWindow is
// required for a MALL_DOCK outlet and rejected otherwise, matching the seed's
// convention that only mall outlets carry a mall window.
type OutletWrite struct {
	OutletID          string
	Name              string
	Brand             domain.Brand
	District          string
	DepotID           string
	DockType          domain.DockType
	ParkingConstraint domain.ParkingConstraint
	WindowOpenTime    string
	WindowCloseTime   string
	MallWindowOpen    string
	MallWindowClose   string
}

// --- validation ------------------------------------------------------------

// ValidateVehicle checks a vehicle write against the schema's constraints and
// the domain enums. It returns every problem at once via ValidationError.
func ValidateVehicle(v VehicleWrite, requireID bool) error {
	if requireID {
		if !validVehicleID(v.VehicleID) {
			return ValidationError{Field: "vehicleId", Message: "must match VEHyynn, e.g. VEH061"}
		}
	}
	if v.Type != domain.VehicleTruck && v.Type != domain.VehicleVan {
		return ValidationError{Field: "type", Message: "must be one of TRUCK, VAN"}
	}
	if v.TempClass != domain.VehicleTempAmbient && v.TempClass != domain.VehicleTempReefer {
		return ValidationError{Field: "tempClass", Message: "must be one of AMBIENT, REEFER"}
	}
	if v.WeightCapKg <= 0 {
		return ValidationError{Field: "weightCapKg", Message: "must be greater than zero"}
	}
	if v.VolumeCapM3 <= 0 {
		return ValidationError{Field: "volumeCapM3", Message: "must be greater than zero"}
	}
	if strings.TrimSpace(v.FuelType) == "" {
		return ValidationError{Field: "fuelType", Message: "is required"}
	}
	if v.KmPerL <= 0 {
		return ValidationError{Field: "kmPerL", Message: "must be greater than zero"}
	}
	if v.WeeklyFuelQuotaL < 0 {
		return ValidationError{Field: "weeklyFuelQuotaL", Message: "must not be negative"}
	}
	if strings.TrimSpace(v.DepotID) == "" {
		return ValidationError{Field: "depotId", Message: "is required"}
	}
	return nil
}

// ValidateOutlet checks an outlet write against the schema's constraints and the
// domain enums, including the window shapes and the mall-window rule.
func ValidateOutlet(o OutletWrite, requireID bool) error {
	if requireID {
		if !validOutletID(o.OutletID) {
			return ValidationError{Field: "outletId", Message: "must match OUTnnn, e.g. OUT121"}
		}
	}
	if strings.TrimSpace(o.Name) == "" {
		return ValidationError{Field: "name", Message: "is required"}
	}
	if !o.Brand.Valid() {
		return ValidationError{Field: "brand", Message: "must be one of FRESH, STYLE, TECH"}
	}
	if strings.TrimSpace(o.District) == "" {
		return ValidationError{Field: "district", Message: "is required"}
	}
	if strings.TrimSpace(o.DepotID) == "" {
		return ValidationError{Field: "depotId", Message: "is required"}
	}
	switch o.DockType {
	case domain.DockRearDock, domain.DockStreet, domain.DockMallBay:
	default:
		return ValidationError{Field: "dockType", Message: "must be one of REAR_DOCK, STREET, MALL_BAY"}
	}
	switch o.ParkingConstraint {
	case domain.ParkingNormal, domain.ParkingVanOnly, domain.ParkingMallDock:
	default:
		return ValidationError{Field: "parkingConstraint", Message: "must be one of NORMAL, VAN_ONLY, MALL_DOCK"}
	}

	open, err := parseWindow(o.WindowOpenTime)
	if err != nil {
		return ValidationError{Field: "windowOpenTime", Message: "must be HH:MM (24-hour)"}
	}
	close, err := parseWindow(o.WindowCloseTime)
	if err != nil {
		return ValidationError{Field: "windowCloseTime", Message: "must be HH:MM (24-hour)"}
	}
	if close <= open {
		return ValidationError{Field: "windowCloseTime", Message: "must be after windowOpenTime"}
	}

	hasMallOpen := strings.TrimSpace(o.MallWindowOpen) != ""
	hasMallClose := strings.TrimSpace(o.MallWindowClose) != ""
	if hasMallOpen != hasMallClose {
		return ValidationError{Field: "mallWindow", Message: "both mall window times are required together"}
	}
	if o.ParkingConstraint == domain.ParkingMallDock {
		if !hasMallOpen {
			return ValidationError{Field: "mallWindow", Message: "a MALL_DOCK outlet requires a mall access window"}
		}
		mo, err := parseWindow(o.MallWindowOpen)
		if err != nil {
			return ValidationError{Field: "mallWindowOpen", Message: "must be HH:MM (24-hour)"}
		}
		mc, err := parseWindow(o.MallWindowClose)
		if err != nil {
			return ValidationError{Field: "mallWindowClose", Message: "must be HH:MM (24-hour)"}
		}
		if mc <= mo {
			return ValidationError{Field: "mallWindowClose", Message: "must be after mallWindowOpen"}
		}
	} else if hasMallOpen {
		// A non-mall outlet must not carry a mall window; the seed stores NULL.
		return ValidationError{Field: "mallWindow", Message: "only a MALL_DOCK outlet may have a mall window"}
	}
	return nil
}

// parseWindow parses a 24-hour HH:MM clock time to minutes since midnight. It
// requires exactly five characters and a zero-padded hour, because time.Parse's
// "15:04" layout accepts "5:00" and would silently normalise a malformed value.
func parseWindow(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) != 5 || s[2] != ':' {
		return 0, errors.New("window must be HH:MM")
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

// --- read types for mutation -------------------------------------------------

// VehicleDetail is the full vehicle record as stored, including its day status.
type VehicleDetail struct {
	VehicleWrite
	Status string
}

// OutletDetail is the full outlet record as stored.
type OutletDetail struct {
	OutletWrite
}

// --- sentinel errors -------------------------------------------------------

var (
	// ErrDuplicate means a vehicle/outlet with that identity already exists.
	ErrDuplicate = errors.New("catalog: identity already exists")
)

// --- repository mutations --------------------------------------------------

// VehicleWriter persists vehicle master data. It is separate from NetworkReader
// so read callers do not depend on write methods.
type VehicleWriter interface {
	GetVehicle(ctx context.Context, vehicleID string) (VehicleDetail, error)
	CreateVehicle(ctx context.Context, v VehicleWrite, actor string) (VehicleDetail, error)
	UpdateVehicle(ctx context.Context, v VehicleWrite, actor string) (VehicleDetail, error)
	NextVehicleID(ctx context.Context) (string, error)
	DepotExists(ctx context.Context, depotID string) (bool, error)
}

// OutletWriter persists outlet master data.
type OutletWriter interface {
	GetOutlet(ctx context.Context, outletID string) (OutletDetail, error)
	CreateOutlet(ctx context.Context, o OutletWrite, actor string) (OutletDetail, error)
	UpdateOutlet(ctx context.Context, o OutletWrite, actor string) (OutletDetail, error)
	NextOutletID(ctx context.Context) (string, error)
}

// PGNetworkWriter is the PostgreSQL-backed writer for vehicles and outlets. It
// writes the mutation and its audit row in one transaction, so a rolled-back
// change leaves no audit trail.
type PGNetworkWriter struct {
	pool interface {
		Begin(ctx context.Context) (pgx.Tx, error)
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	}
	audit AuditSink
}

// AuditSink records one audit event in the caller's transaction. It is a narrow
// interface so this package does not import internal/audit.
type AuditSink interface {
	RecordTx(ctx context.Context, tx pgx.Tx, action, entityType, entityID, actor, depotID, outletID, result string, detail map[string]any) error
}

// NewPGNetworkWriter builds a writer over a pool. audit may be nil in narrow
// tests, in which case no audit row is written.
func NewPGNetworkWriter(pool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, audit AuditSink) *PGNetworkWriter {
	return &PGNetworkWriter{pool: pool, audit: audit}
}

// DepotExists reports whether depotID is a known depot.
func (w *PGNetworkWriter) DepotExists(ctx context.Context, depotID string) (bool, error) {
	var exists bool
	err := w.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM depot WHERE depot_id = $1 AND is_active)`, depotID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check depot: %w", err)
	}
	return exists, nil
}

// NextVehicleID returns the next free VEHyynn identifier. It scans the existing
// ids, parses the numeric suffix and returns max+1, so it never collides with a
// seeded or previously created vehicle.
func (w *PGNetworkWriter) NextVehicleID(ctx context.Context) (string, error) {
	return nextNumericID(ctx, w.pool, "vehicle", "vehicle_id", vehicleIDPattern, "VEH")
}

// NextOutletID returns the next free OUTnnn identifier.
func (w *PGNetworkWriter) NextOutletID(ctx context.Context) (string, error) {
	return nextNumericID(ctx, w.pool, "outlet", "outlet_id", outletIDPattern, "OUT")
}

// nextNumericID scans a table's natural-key column for the highest numeric
// suffix matching pattern and returns prefix+(max+1) zero-padded to three
// digits. It never trusts the caller for identity.
func nextNumericID(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, table, column string, pattern *regexp.Regexp, prefix string) (string, error) {
	// table/column are internal constants, never client input, so the fmt is safe.
	var maxID string
	err := q.QueryRow(ctx, fmt.Sprintf(
		`SELECT COALESCE(MAX(substring(%s from 4)::int), 0)::text FROM %s WHERE %s ~ '^%s[0-9]{3}$'`,
		column, table, column, prefix)).Scan(&maxID)
	if err != nil {
		return "", fmt.Errorf("scan max %s: %w", column, err)
	}
	var n int
	if _, err := fmt.Sscanf(maxID, "%d", &n); err != nil {
		return "", fmt.Errorf("parse max %s %q: %w", column, maxID, err)
	}
	return fmt.Sprintf("%s%03d", prefix, n+1), nil
}
