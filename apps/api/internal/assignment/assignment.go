// Package assignment owns the Dispatcher's operational assignments: which driver
// is on which vehicle for an operating date, and which store manager is
// responsible for an outlet.
//
// These are not reference data. A driver-to-vehicle assignment is the link the
// schema was missing between a driver and a run, so the driver cockpit can
// resolve "which run is mine?" server-side instead of asking the driver to pick
// from every depot route. An outlet's manager is the existing authoritative
// link on app_user.outlet_id, exposed here as a focused Dispatcher workflow
// rather than a second relationship table.
//
// Every mutation is Dispatcher-only, validated server-side (role, active,
// depot compatibility, one-driver/vehicle-per-date) and audited in the same
// transaction.
package assignment

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"waypoint.lk/api/internal/domain"
)

// Driver is the read view of a DRIVER account used by the assignment screens.
// It carries only display identity — never a credential.
type Driver struct {
	UserID  string
	Name    string
	Email   string
	DepotID string
}

// User is a minimal account view for validating an assignment target. It never
// carries a password hash or session material.
type User struct {
	UserID   string
	Name     string
	Email    string
	Role     domain.Role
	Active   bool
	DepotID  string
	OutletID string
}

// VehicleAssignment is one driver on one vehicle for one operating date.
type VehicleAssignment struct {
	VehicleID      string
	Driver         Driver
	AssignmentDate string // YYYY-MM-DD
	DepotID        string
}

// OutletManager is the store manager responsible for an outlet.
type OutletManager struct {
	OutletID string
	UserID   string
	Name     string
	Email    string
	DepotID  string
}

// ManagerCandidate is an active STORE_MANAGER as the assignment picker shows
// it. OutletID is empty when the manager is currently unassigned.
type ManagerCandidate struct {
	UserID   string
	Name     string
	Email    string
	OutletID string
}

// AssignDriverInput is a validated-by-the-service request to put a driver on a
// vehicle for a date.
type AssignDriverInput struct {
	VehicleID string
	DriverID  string
	Date      string
}

// AssignManagerInput is a request to make a store manager responsible for an
// outlet.
type AssignManagerInput struct {
	OutletID string
	UserID   string
}

// --- errors ----------------------------------------------------------------

// ErrNotFound means the referenced vehicle, outlet, user or assignment does not
// exist.
var ErrNotFound = errors.New("assignment: not found")

// ErrConflict means the assignment collides with an existing one (a driver or
// vehicle already committed for the date).
var ErrConflict = errors.New("assignment: conflict")

// ErrInvalid is the sentinel every ValidationError unwraps to.
var ErrInvalid = errors.New("assignment: invalid input")

// ValidationError names the field that failed validation. It is mapped to the
// shared 400/422 field-error contract at the HTTP boundary.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// --- validation helpers ----------------------------------------------------

// ParseDate validates an operating-date string and returns it canonicalised as
// YYYY-MM-DD.
func ParseDate(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", ValidationError{Field: "date", Message: "is required"}
	}
	d, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return "", ValidationError{Field: "date", Message: "must be YYYY-MM-DD"}
	}
	return d.Format(time.DateOnly), nil
}

// validateDriverTarget checks the account is an active driver with a home depot.
func validateDriverTarget(u User, userID string) error {
	if !u.Active {
		return ValidationError{Field: "driverId", Message: fmt.Sprintf("driver %s is not active", userID)}
	}
	if u.Role != domain.RoleDriver {
		return ValidationError{Field: "driverId", Message: fmt.Sprintf("%s is not a driver", userID)}
	}
	if strings.TrimSpace(u.DepotID) == "" {
		return ValidationError{Field: "driverId", Message: fmt.Sprintf("driver %s has no home depot", userID)}
	}
	return nil
}
