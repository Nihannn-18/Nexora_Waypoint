package useradmin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/password"
)

// Store is the persistence surface the service needs. It is satisfied by
// *PGStore and by a fake in tests. Write methods run in a transaction that also
// records the audit row, so a rolled-back mutation leaves no audit trail.
type Store interface {
	// ListAccounts returns accounts, optionally filtered by role and active
	// state, newest first.
	ListAccounts(ctx context.Context, role domain.Role, active *bool) ([]Account, error)
	// AccountByID loads one account, or ErrNotFound.
	AccountByID(ctx context.Context, userID string) (Account, error)
	// EmailExists reports whether an account already uses the email.
	EmailExists(ctx context.Context, email string) (bool, error)
	// DepotExists reports whether a depot exists and is active.
	DepotExists(ctx context.Context, depotID string) (bool, error)
	// OutletDepot returns the depot id of an existing outlet, or ErrNotFound.
	OutletDepot(ctx context.Context, outletID string) (string, error)

	// CreateAccount inserts an account and audits it in one transaction.
	CreateAccount(ctx context.Context, in CreateInput, passwordHash, actor string) (Account, error)
	// UpdateAccount updates the permitted fields and audits the change.
	UpdateAccount(ctx context.Context, userID string, in UpdateInput, actor string) (Account, error)
	// SetActive activates or deactivates an account, revokes its sessions when
	// deactivating, and audits the change, in one transaction.
	SetActive(ctx context.Context, userID string, active bool, actor string) (Account, error)

	// CreateResetToken stores the hash of a reset token for a user, superseding
	// any outstanding token for that user, in one transaction.
	CreateResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	// ConsumeResetToken atomically validates a token hash (unused, unexpired,
	// active user), sets the new password hash, marks the token used and revokes
	// the user's sessions, in one transaction. It returns the user id.
	//
	// `now` is the caller's injected business instant and is compared against
	// `expires_at` in the same time domain the token was minted in. It must not
	// be left to the database clock: under DEMO_MODE the business clock is the
	// seeded (past) day, and mixing it with a real-time now() would make every
	// freshly minted token look expired (CLAUDE.md §2 rule 8).
	ConsumeResetToken(ctx context.Context, tokenHash, newPasswordHash string, now time.Time) (string, error)
	// UserIDByEmail returns the active user id for an email, or "" when none.
	UserIDByEmail(ctx context.Context, email string) (string, error)
}

// Clock reports the API's current business-time instant.
type Clock interface{ Now() time.Time }

// Service is the account-management and password-reset business surface.
type Service struct {
	store Store
	clock Clock
}

// NewService builds the service. clock may be nil in narrow tests, in which
// case reset-token expiry uses the wall clock.
func NewService(store Store, clock Clock) *Service {
	return &Service{store: store, clock: clock}
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock.Now()
	}
	return time.Now()
}

// resetTokenTTL is how long a password-reset link stays valid. Short by design:
// long enough for a store manager to open the link, short enough that a
// leaked link is not a lasting credential.
const resetTokenTTL = 30 * time.Minute

// CreateAccount validates a role/scope assignment against real reference data,
// hashes the initial password with Argon2id, and persists the account.
//
// The role must be one a dispatcher may create; a driver/loader must name an
// existing active depot; a store manager must name an existing outlet, and the
// account's depot is derived from that outlet so there is a single source of
// truth. A duplicate email is ErrDuplicateEmail (mapped to 409).
func (s *Service) CreateAccount(ctx context.Context, in CreateInput, actor string) (Account, error) {
	in.Email = NormalizeEmail(in.Email)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.DepotID = strings.TrimSpace(in.DepotID)
	in.OutletID = strings.TrimSpace(in.OutletID)

	if err := validateCreateShape(in); err != nil {
		return Account{}, err
	}
	if err := password.Validate(in.InitialPassword); err != nil {
		return Account{}, ValidationError{Field: "initialPassword", Message: err.Error()}
	}

	// Resolve scope against the store, not the client's word.
	switch in.Role {
	case domain.RoleDriver, domain.RoleLoader:
		ok, err := s.store.DepotExists(ctx, in.DepotID)
		if err != nil {
			return Account{}, err
		}
		if !ok {
			return Account{}, ValidationError{Field: "depotId", Message: "is not a known depot"}
		}
	case domain.RoleStoreManager:
		depotID, err := s.store.OutletDepot(ctx, in.OutletID)
		if err != nil {
			return Account{}, err
		}
		// The outlet is the authority for depot scope; ignore any client depot.
		in.DepotID = depotID
	}

	exists, err := s.store.EmailExists(ctx, in.Email)
	if err != nil {
		return Account{}, err
	}
	if exists {
		return Account{}, ErrDuplicateEmail
	}

	hash, err := password.Hash(in.InitialPassword)
	if err != nil {
		return Account{}, fmt.Errorf("hash initial password: %w", err)
	}
	return s.store.CreateAccount(ctx, in, hash, actor)
}

// ListAccounts returns the operational accounts the Dispatcher user screen
// manages. role and active are optional filters; an empty role and nil active
// mean "no filter".
//
// Only roles a dispatcher may create are ever returned: a DISPATCHER account is
// neither created nor managed through this surface, so it must not appear in
// the list (and ?role=DISPATCHER is rejected rather than silently returning a
// privileged account).
func (s *Service) ListAccounts(ctx context.Context, role domain.Role, active *bool) ([]Account, error) {
	if role != "" && !RoleCreatable(role) {
		return nil, ValidationError{Field: "role", Message: "must be one of DRIVER, LOADER, STORE_MANAGER"}
	}
	accounts, err := s.store.ListAccounts(ctx, role, active)
	if err != nil {
		return nil, err
	}
	operational := make([]Account, 0, len(accounts))
	for _, a := range accounts {
		if RoleCreatable(a.Role) {
			operational = append(operational, a)
		}
	}
	return operational, nil
}

// Account returns one account.
func (s *Service) Account(ctx context.Context, userID string) (Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Account{}, ValidationError{Field: "userId", Message: "is required"}
	}
	return s.store.AccountByID(ctx, userID)
}

// UpdateAccount changes permitted profile/assignment fields. Role and password
// cannot be changed here.
//
// A store manager's depot always tracks its outlet, so a depot cannot be set
// independently for that role; drivers/loaders are re-validated against the
// depot they are moved to. Clearing a driver/loader depot is refused — the role
// requires one.
func (s *Service) UpdateAccount(ctx context.Context, userID string, in UpdateInput, actor string) (Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Account{}, ValidationError{Field: "userId", Message: "is required"}
	}
	current, err := s.store.AccountByID(ctx, userID)
	if err != nil {
		return Account{}, err
	}

	// Normalise the patch and enforce role-specific scope rules against the
	// account's immutable role.
	resolved := current
	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if err := validateName(name); err != nil {
			return Account{}, err
		}
		in.DisplayName = &name
	}
	switch current.Role {
	case domain.RoleDriver, domain.RoleLoader:
		if in.DepotID != nil {
			depot := strings.TrimSpace(*in.DepotID)
			if depot == "" {
				return Account{}, ValidationError{Field: "depotId", Message: "is required for this role"}
			}
			ok, err := s.store.DepotExists(ctx, depot)
			if err != nil {
				return Account{}, err
			}
			if !ok {
				return Account{}, ValidationError{Field: "depotId", Message: "is not a known depot"}
			}
			in.DepotID = &depot
			resolved.DepotID = depot
		}
		if in.OutletID != nil && strings.TrimSpace(*in.OutletID) != "" {
			return Account{}, ValidationError{Field: "outletId", Message: "must not be set for this role"}
		}
		in.OutletID = nil
	case domain.RoleStoreManager:
		if in.OutletID != nil {
			outlet := strings.TrimSpace(*in.OutletID)
			if outlet == "" {
				return Account{}, ValidationError{Field: "outletId", Message: "is required for a store manager"}
			}
			depotID, err := s.store.OutletDepot(ctx, outlet)
			if err != nil {
				return Account{}, err
			}
			in.OutletID = &outlet
			resolved.OutletID = outlet
			resolved.DepotID = depotID
			// A store manager's depot follows the outlet; ignore any client depot.
			in.DepotID = &depotID
		} else if in.DepotID != nil {
			// Reject an attempt to set depot independently for this role.
			return Account{}, ValidationError{Field: "depotId", Message: "is derived from the store manager's outlet"}
		}
	default:
		// A dispatcher account is not editable through this surface.
		return Account{}, ValidationError{Field: "role", Message: "this account is not manageable here"}
	}

	return s.store.UpdateAccount(ctx, userID, in, actor)
}

// SetActive activates or deactivates an operational account. A dispatcher
// account is never deactivated here — that would remove the only role able to
// manage accounts. Deactivation revokes the account's sessions so a live token
// cannot outlive it.
func (s *Service) SetActive(ctx context.Context, userID string, active bool, actor string) (Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Account{}, ValidationError{Field: "userId", Message: "is required"}
	}
	current, err := s.store.AccountByID(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	if !RoleCreatable(current.Role) {
		return Account{}, ValidationError{Field: "role", Message: "this account is not manageable here"}
	}
	return s.store.SetActive(ctx, userID, active, actor)
}

// --- password reset --------------------------------------------------------

// resetTokenBytes is the entropy of a reset token: 256 bits, base64url encoded.
const resetTokenBytes = 32

// NewResetToken returns a raw reset token and its lookup hash. The raw token is
// returned to the caller once; only the hash is persisted.
func NewResetToken() (raw, hash string, err error) {
	buf := make([]byte, resetTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate reset token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashResetToken(raw), nil
}

// HashResetToken returns the hex SHA-256 of a raw reset token. It is the value
// stored in password_reset_token.token_hash.
func HashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ForgotPassword requests a reset for email. It always succeeds from the
// caller's point of view: an unknown email produces no token but the same
// generic result, so the endpoint cannot be used to enumerate accounts.
//
// The returned raw token is non-empty only when the email matched an active
// account. Callers must NOT return it from the production HTTP endpoint; the
// handler logs it only under the explicit DEMO_MODE flag (see handler.go).
func (s *Service) ForgotPassword(ctx context.Context, email string) (rawToken string, matched bool, err error) {
	email = NormalizeEmail(email)
	if err := ValidateEmail(email); err != nil {
		return "", false, err
	}
	userID, err := s.store.UserIDByEmail(ctx, email)
	if err != nil {
		return "", false, err
	}
	if userID == "" {
		// No account: report success without minting a token.
		return "", false, nil
	}
	raw, hash, err := NewResetToken()
	if err != nil {
		return "", false, err
	}
	if err := s.store.CreateResetToken(ctx, userID, hash, s.now().Add(resetTokenTTL)); err != nil {
		return "", false, err
	}
	return raw, true, nil
}

// ResetPassword consumes a reset token and sets a new password. It validates the
// token (unknown, expired, used, inactive user all collapse to
// ErrInvalidResetToken), enforces the password policy, hashes the new password
// with Argon2id, and lets the store update the credential, mark the token used
// and revoke the user's sessions in one transaction.
func (s *Service) ResetPassword(ctx context.Context, in ResetInput) error {
	token := strings.TrimSpace(in.Token)
	if token == "" {
		return ValidationError{Field: "token", Message: "is required"}
	}
	if err := password.Validate(in.NewPassword); err != nil {
		return ValidationError{Field: "newPassword", Message: err.Error()}
	}
	if in.NewPassword != in.ConfirmPassword {
		return ValidationError{Field: "confirmPassword", Message: "does not match the new password"}
	}
	hash, err := password.Hash(in.NewPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	if _, err := s.store.ConsumeResetToken(ctx, HashResetToken(token), hash, s.now()); err != nil {
		return err
	}
	return nil
}

// IsNotFound reports whether err is the not-found sentinel.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsDuplicateEmail reports whether err is the duplicate-email sentinel.
func IsDuplicateEmail(err error) bool { return errors.Is(err, ErrDuplicateEmail) }

// IsInvalidResetToken reports whether err is the invalid-reset-token sentinel.
func IsInvalidResetToken(err error) bool { return errors.Is(err, ErrInvalidResetToken) }
