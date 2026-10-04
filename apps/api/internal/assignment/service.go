package assignment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Clock reports the API's current business-time instant. An assignment date is
// defaulted from it, never from a device clock.
type Clock interface{ Now() time.Time }

// Store is the persistence surface the service needs. It is satisfied by
// *PGStore and by a fake in tests. Write methods run in a transaction that also
// records the audit row, so a rolled-back mutation leaves no audit record.
type Store interface {
	// ListDrivers returns active drivers, optionally filtered by home depot.
	ListDrivers(ctx context.Context, depotID string) ([]Driver, error)
	// ListStoreManagers returns active store managers for the manager picker.
	ListStoreManagers(ctx context.Context) ([]ManagerCandidate, error)
	// ListLoaders returns active loaders with their current depot.
	ListLoaders(ctx context.Context) ([]Loader, error)
	// UserByID loads one account, or ErrNotFound.
	UserByID(ctx context.Context, userID string) (User, error)
	// VehicleDepot returns a vehicle's home depot id, or ErrNotFound.
	VehicleDepot(ctx context.Context, vehicleID string) (string, error)
	// DepotExists reports whether a depot exists and is active.
	DepotExists(ctx context.Context, depotID string) (bool, error)
	// OutletExists reports whether an outlet exists.
	OutletExists(ctx context.Context, outletID string) (bool, error)

	// AssignmentForVehicle returns the assignment for a vehicle on a date, if any.
	AssignmentForVehicle(ctx context.Context, vehicleID, date string) (VehicleAssignment, bool, error)
	// AssignmentForDriver returns a driver's assignment on a date, if any.
	AssignmentForDriver(ctx context.Context, driverID, date string) (VehicleAssignment, bool, error)
	// ListVehicleAssignments returns every driver-vehicle assignment on a date.
	ListVehicleAssignments(ctx context.Context, date string) ([]VehicleAssignment, error)
	// AssignVehicle upserts the assignment for its vehicle+date in a transaction.
	AssignVehicle(ctx context.Context, a VehicleAssignment, actor string) (VehicleAssignment, error)
	// UnassignVehicle removes the assignment for a vehicle+date and returns what
	// was removed, or ErrNotFound.
	UnassignVehicle(ctx context.Context, vehicleID, date, actor string) (VehicleAssignment, error)

	// ManagerForOutlet returns the store manager currently responsible for an
	// outlet, if any.
	ManagerForOutlet(ctx context.Context, outletID string) (OutletManager, bool, error)
	// SetManager makes userID the manager of outletID in a transaction,
	// unassigning any other manager currently on that outlet.
	SetManager(ctx context.Context, outletID, userID, actor string) (OutletManager, error)
	// ClearManager removes the manager currently on outletID and returns them,
	// or ErrNotFound.
	ClearManager(ctx context.Context, outletID, actor string) (OutletManager, error)

	// SetLoaderDepot sets a loader's authoritative depot in a transaction.
	SetLoaderDepot(ctx context.Context, loaderID, depotID, actor string) (Loader, error)
	// ClearLoaderDepot removes a loader's depot and returns what was removed, or
	// ErrNotFound.
	ClearLoaderDepot(ctx context.Context, loaderID, actor string) (Loader, error)
}

// Service is the assignment business surface. It validates every request
// server-side before delegating the write.
type Service struct {
	store Store
	clock Clock
}

// NewService builds the service. clock may be nil in narrow tests, in which
// case a missing date is a validation error rather than defaulted.
func NewService(store Store, clock Clock) *Service {
	return &Service{store: store, clock: clock}
}

// resolveDate canonicalises a requested date, defaulting to the API clock's
// business day when the caller supplies none.
func (s *Service) resolveDate(v string) (string, error) {
	if strings.TrimSpace(v) == "" {
		if s.clock == nil {
			return "", ValidationError{Field: "date", Message: "is required"}
		}
		return s.clock.Now().Format(time.DateOnly), nil
	}
	return ParseDate(v)
}

// ListDrivers returns active drivers for the assignment picker.
func (s *Service) ListDrivers(ctx context.Context, depotID string) ([]Driver, error) {
	return s.store.ListDrivers(ctx, strings.TrimSpace(depotID))
}

// ListStoreManagers returns active store managers for the manager picker.
func (s *Service) ListStoreManagers(ctx context.Context) ([]ManagerCandidate, error) {
	return s.store.ListStoreManagers(ctx)
}

// ListLoaders returns active loaders with their current depot, for the loader
// assignment screen.
func (s *Service) ListLoaders(ctx context.Context) ([]Loader, error) {
	return s.store.ListLoaders(ctx)
}

// ListVehicleAssignments returns the driver-vehicle assignments on a date,
// defaulting to the API clock's business day. It is the read the Dispatcher
// assignments board uses to show the whole day at once.
func (s *Service) ListVehicleAssignments(ctx context.Context, date string) ([]VehicleAssignment, error) {
	d, err := s.resolveDate(date)
	if err != nil {
		return nil, err
	}
	return s.store.ListVehicleAssignments(ctx, d)
}

// DriverAssignment returns the assignment for a specific driver, if any. It is
// used by the driver's own read endpoint and by the Dispatcher screens.
func (s *Service) DriverAssignment(ctx context.Context, driverID, date string) (VehicleAssignment, bool, error) {
	d, err := s.resolveDate(date)
	if err != nil {
		return VehicleAssignment{}, false, err
	}
	return s.store.AssignmentForDriver(ctx, strings.TrimSpace(driverID), d)
}

// VehicleAssignment returns the assignment for a specific vehicle on a date.
func (s *Service) VehicleAssignment(ctx context.Context, vehicleID, date string) (VehicleAssignment, bool, error) {
	d, err := s.resolveDate(date)
	if err != nil {
		return VehicleAssignment{}, false, err
	}
	return s.store.AssignmentForVehicle(ctx, strings.TrimSpace(vehicleID), d)
}

// AssignDriver validates and records a driver-to-vehicle assignment.
//
// Rules enforced here (and backed by the table's unique constraints):
//   - the target is an active DRIVER with a home depot;
//   - the driver's depot matches the vehicle's depot;
//   - the driver is not already committed to another vehicle that date.
//
// Reassigning the same vehicle+date to a different driver is allowed (that is a
// "change driver"); the previous driver's history for other dates is untouched.
func (s *Service) AssignDriver(ctx context.Context, in AssignDriverInput, actor string) (VehicleAssignment, error) {
	vehicleID := strings.TrimSpace(in.VehicleID)
	driverID := strings.TrimSpace(in.DriverID)
	if vehicleID == "" {
		return VehicleAssignment{}, ValidationError{Field: "vehicleId", Message: "is required"}
	}
	if driverID == "" {
		return VehicleAssignment{}, ValidationError{Field: "driverId", Message: "is required"}
	}
	date, err := ParseDate(in.Date)
	if err != nil {
		return VehicleAssignment{}, err
	}

	vehicleDepot, err := s.store.VehicleDepot(ctx, vehicleID)
	if err != nil {
		return VehicleAssignment{}, err
	}
	user, err := s.store.UserByID(ctx, driverID)
	if err != nil {
		return VehicleAssignment{}, err
	}
	if err := validateDriverTarget(user, driverID); err != nil {
		return VehicleAssignment{}, err
	}
	if user.DepotID != vehicleDepot {
		return VehicleAssignment{}, ValidationError{
			Field:   "driverId",
			Message: fmt.Sprintf("driver %s is not in this vehicle's depot", driverID),
		}
	}

	// One driver cannot be on two vehicles the same date. Changing this vehicle's
	// driver is fine; moving the driver to a second vehicle is not.
	if existing, ok, err := s.store.AssignmentForDriver(ctx, driverID, date); err != nil {
		return VehicleAssignment{}, err
	} else if ok && existing.VehicleID != vehicleID {
		return VehicleAssignment{}, fmt.Errorf(
			"%w: driver %s is already assigned to %s on %s", ErrConflict, driverID, existing.VehicleID, date)
	}

	return s.store.AssignVehicle(ctx, VehicleAssignment{
		VehicleID:      vehicleID,
		Driver:         Driver{UserID: user.UserID, Name: user.Name, Email: user.Email, DepotID: user.DepotID},
		AssignmentDate: date,
		DepotID:        vehicleDepot,
	}, actor)
}

// UnassignDriver removes a vehicle's driver for a date and returns what was
// removed.
func (s *Service) UnassignDriver(ctx context.Context, vehicleID, date, actor string) (VehicleAssignment, error) {
	vehicleID = strings.TrimSpace(vehicleID)
	if vehicleID == "" {
		return VehicleAssignment{}, ValidationError{Field: "vehicleId", Message: "is required"}
	}
	d, err := ParseDate(date)
	if err != nil {
		return VehicleAssignment{}, err
	}
	return s.store.UnassignVehicle(ctx, vehicleID, d, actor)
}

// OutletManager returns the manager responsible for an outlet, if any.
func (s *Service) OutletManager(ctx context.Context, outletID string) (OutletManager, bool, error) {
	outletID = strings.TrimSpace(outletID)
	if outletID == "" {
		return OutletManager{}, false, ValidationError{Field: "outletId", Message: "is required"}
	}
	if ok, err := s.store.OutletExists(ctx, outletID); err != nil {
		return OutletManager{}, false, err
	} else if !ok {
		return OutletManager{}, false, fmt.Errorf("%w: outlet %s", ErrNotFound, outletID)
	}
	return s.store.ManagerForOutlet(ctx, outletID)
}

// AssignManager makes an active STORE_MANAGER the manager of an outlet. Any
// other manager currently on the outlet is unassigned in the same transaction,
// so an outlet has exactly one manager. Moving a manager to a new outlet frees
// their previous outlet.
func (s *Service) AssignManager(ctx context.Context, in AssignManagerInput, actor string) (OutletManager, error) {
	outletID := strings.TrimSpace(in.OutletID)
	userID := strings.TrimSpace(in.UserID)
	if outletID == "" {
		return OutletManager{}, ValidationError{Field: "outletId", Message: "is required"}
	}
	if userID == "" {
		return OutletManager{}, ValidationError{Field: "userId", Message: "is required"}
	}
	if ok, err := s.store.OutletExists(ctx, outletID); err != nil {
		return OutletManager{}, err
	} else if !ok {
		return OutletManager{}, fmt.Errorf("%w: outlet %s", ErrNotFound, outletID)
	}
	user, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return OutletManager{}, err
	}
	if !user.Active {
		return OutletManager{}, ValidationError{Field: "userId", Message: fmt.Sprintf("%s is not active", userID)}
	}
	if user.Role != domain.RoleStoreManager {
		return OutletManager{}, ValidationError{Field: "userId", Message: fmt.Sprintf("%s is not a store manager", userID)}
	}
	return s.store.SetManager(ctx, outletID, userID, actor)
}

// UnassignManager removes an outlet's manager and returns the removed account.
func (s *Service) UnassignManager(ctx context.Context, outletID, actor string) (OutletManager, error) {
	outletID = strings.TrimSpace(outletID)
	if outletID == "" {
		return OutletManager{}, ValidationError{Field: "outletId", Message: "is required"}
	}
	return s.store.ClearManager(ctx, outletID, actor)
}

// AssignLoader sets a loader's operational depot. The loader's depot is
// app_user.depot_id — the authoritative representation — so this updates that
// single field in a transaction rather than creating a second relationship.
//
// The target is validated to be an active LOADER and the depot to exist, both
// against server state; a client cannot point a loader at a depot that is not
// real, nor move another role this way.
func (s *Service) AssignLoader(ctx context.Context, in AssignLoaderInput, actor string) (Loader, error) {
	loaderID := strings.TrimSpace(in.LoaderID)
	depotID := strings.TrimSpace(in.DepotID)
	if loaderID == "" {
		return Loader{}, ValidationError{Field: "loaderId", Message: "is required"}
	}
	if depotID == "" {
		return Loader{}, ValidationError{Field: "depotId", Message: "is required"}
	}
	user, err := s.store.UserByID(ctx, loaderID)
	if err != nil {
		return Loader{}, err
	}
	if !user.Active {
		return Loader{}, ValidationError{Field: "loaderId", Message: fmt.Sprintf("%s is not active", loaderID)}
	}
	if user.Role != domain.RoleLoader {
		return Loader{}, ValidationError{Field: "loaderId", Message: fmt.Sprintf("%s is not a loader", loaderID)}
	}
	if ok, err := s.store.DepotExists(ctx, depotID); err != nil {
		return Loader{}, err
	} else if !ok {
		return Loader{}, fmt.Errorf("%w: depot %s", ErrNotFound, depotID)
	}
	return s.store.SetLoaderDepot(ctx, loaderID, depotID, actor)
}

// UnassignLoader removes a loader's depot and returns the former assignment.
func (s *Service) UnassignLoader(ctx context.Context, loaderID, actor string) (Loader, error) {
	loaderID = strings.TrimSpace(loaderID)
	if loaderID == "" {
		return Loader{}, ValidationError{Field: "loaderId", Message: "is required"}
	}
	return s.store.ClearLoaderDepot(ctx, loaderID, actor)
}

// IsNotFound reports whether err is the not-found sentinel.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsConflict reports whether err is the conflict sentinel.
func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }
