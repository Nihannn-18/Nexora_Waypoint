// Package useradmin owns Dispatcher account management and the self-service
// password reset flow.
//
// Account management: a Dispatcher creates operational accounts (DRIVER,
// LOADER, STORE_MANAGER), assigns the role-appropriate depot or outlet, sets an
// initial password, edits permitted profile/assignment fields, and deactivates
// an account. It is deliberately narrow:
//
//   - The Dispatcher may NOT create another DISPATCHER. There is no privileged
//     self-replication path; the seeded dispatcher (§6) is the controlled way to
//     establish dispatcher accounts.
//   - The client never chooses a role the server will accept blindly: the role
//     is validated against the allowed-operational set server-side.
//   - Role-specific fields are enforced: a DRIVER/LOADER requires a depot and
//     must not carry an outlet; a STORE_MANAGER requires a valid outlet and
//     derives its depot from the outlet record, so there is one source of truth.
//   - There is no hard delete: an account referenced by routes, assignments,
//     audit and delivery history is deactivated, never removed.
//
// Passwords: an initial password is hashed with the existing Argon2id helper
// (internal/password). The plaintext is never stored, returned, logged or
// audited; a response never contains password_hash.
//
// This package does not own authentication. It reuses app_user, the session
// table and the Go opaque-session model; it never introduces a second identity
// system.
package useradmin

import (
	"errors"
	"strings"

	"waypoint.lk/api/internal/domain"
)

// --- command types ---------------------------------------------------------

// CreateInput is a validated request to create an operational account.
//
// InitialPassword is the plaintext the Dispatcher sets. It is hashed before it
// leaves the service and is never stored on this struct after that point.
type CreateInput struct {
	Email           string
	DisplayName     string
	Role            domain.Role
	DepotID         string
	OutletID        string
	InitialPassword string
}

// UpdateInput is a request to change an account's permitted profile/assignment
// fields. Role and password are deliberately absent: role is immutable for this
// feature, and a password is changed only through the reset flow.
type UpdateInput struct {
	DisplayName *string
	DepotID     *string
	OutletID    *string
}

// ResetInput is a request to set a new password using a reset token.
type ResetInput struct {
	Token           string
	NewPassword     string
	ConfirmPassword string
}

// --- read types ------------------------------------------------------------

// Account is the Dispatcher-facing view of an app_user. It never carries
// password_hash, a session token or a reset token.
type Account struct {
	UserID      string
	Email       string
	DisplayName string
	Role        domain.Role
	DepotID     string
	OutletID    string
	Active      bool
	CreatedAt   string // RFC 3339
}

// --- sentinel errors -------------------------------------------------------

var (
	// ErrNotFound means the account does not exist.
	ErrNotFound = errors.New("useradmin: account not found")
	// ErrDuplicateEmail means an account with that email already exists.
	ErrDuplicateEmail = errors.New("useradmin: email already in use")
	// ErrInvalid means the request failed validation (wraps ValidationError).
	ErrInvalid = errors.New("useradmin: invalid input")
	// ErrInvalidResetToken means the reset token is unknown, expired, already
	// used, or belongs to an inactive account. The cases are deliberately
	// indistinguishable to the client.
	ErrInvalidResetToken = errors.New("useradmin: invalid or expired reset token")
)

// ValidationError names the field that failed validation.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e ValidationError) Unwrap() error { return ErrInvalid }

// --- role rules ------------------------------------------------------------

// operationalRoles are the only roles a Dispatcher may create. DISPATCHER is
// deliberately excluded so a dispatcher cannot replicate itself.
var operationalRoles = []domain.Role{
	domain.RoleDriver,
	domain.RoleLoader,
	domain.RoleStoreManager,
}

// RoleCreatable reports whether role may be created through account management.
func RoleCreatable(role domain.Role) bool {
	for _, r := range operationalRoles {
		if r == role {
			return true
		}
	}
	return false
}

// CreatableRoles lists the roles a Dispatcher may create, in a stable order.
func CreatableRoles() []domain.Role {
	out := make([]domain.Role, len(operationalRoles))
	copy(out, operationalRoles)
	return out
}

// --- validation ------------------------------------------------------------

// NormalizeEmail trims and lower-cases an email for storage and lookup. Lower
// casing is deliberate: an email is case-insensitive in practice, and storing a
// canonical form means UNIQUE(email) cannot admit two visually identical rows.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateEmail reports whether email has a shape we accept. It is intentionally
// simple — no attempt at full RFC 5322 — and rejects only clearly malformed
// values: no "@", empty local or domain part, no dot in the domain, whitespace.
func ValidateEmail(email string) error {
	e := NormalizeEmail(email)
	if e == "" {
		return ValidationError{Field: "email", Message: "is required"}
	}
	at := strings.LastIndex(e, "@")
	if at <= 0 || at == len(e)-1 {
		return ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	local, domainPart := e[:at], e[at+1:]
	if strings.ContainsAny(e, " \t\r\n") || local == "" || domainPart == "" {
		return ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	if !strings.Contains(domainPart, ".") || strings.HasPrefix(domainPart, ".") || strings.HasSuffix(domainPart, ".") {
		return ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	return nil
}

// validateName rejects an empty display name. A name is a presentation field,
// but the account screens and audit trail read far better with one.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ValidationError{Field: "displayName", Message: "is required"}
	}
	return nil
}

// validateCreateShape enforces role/scope shape independent of the database.
// Existence checks (is that depot/outlet real?) happen in the service, which
// has the store.
func validateCreateShape(in CreateInput) error {
	if err := ValidateEmail(in.Email); err != nil {
		return err
	}
	if err := validateName(in.DisplayName); err != nil {
		return err
	}
	if !RoleCreatable(in.Role) {
		return ValidationError{
			Field:   "role",
			Message: "must be one of DRIVER, LOADER, STORE_MANAGER",
		}
	}
	switch in.Role {
	case domain.RoleDriver, domain.RoleLoader:
		if strings.TrimSpace(in.DepotID) == "" {
			return ValidationError{Field: "depotId", Message: "is required for this role"}
		}
		if strings.TrimSpace(in.OutletID) != "" {
			return ValidationError{Field: "outletId", Message: "must not be set for this role"}
		}
	case domain.RoleStoreManager:
		if strings.TrimSpace(in.OutletID) == "" {
			return ValidationError{Field: "outletId", Message: "is required for a store manager"}
		}
		// depotId is derived from the outlet, never taken from the client, so a
		// store manager cannot be given a depot inconsistent with their outlet.
	}
	return nil
}
