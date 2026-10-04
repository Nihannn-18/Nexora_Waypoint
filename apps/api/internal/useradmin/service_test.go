package useradmin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/password"
)

// fakeStore is an in-memory Store for service tests. It records what the
// service asked it to persist so the tests can assert on hashing, scope and
// reset-token handling without a database.
type fakeStore struct {
	accounts map[string]Account
	hashes   map[string]string // userID -> password hash

	created    CreateInput
	createdFor string
	createdPwd string
	updated    UpdateInput
	activeSet  *bool

	resetForUser string
	resetHash    string
	resetExpiry  time.Time
	consumedHash string
	consumedPwd  string
	consumedNow  time.Time
	consumeErr   error

	createErr      error
	emailExists    bool
	depotExists    bool
	outletDepot    string
	userIDByEmail  string
	consumeUserID  string
	createResetErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		accounts:    map[string]Account{},
		hashes:      map[string]string{},
		depotExists: true,
	}
}

func (f *fakeStore) ListAccounts(context.Context, domain.Role, *bool) ([]Account, error) {
	out := make([]Account, 0, len(f.accounts))
	for _, a := range f.accounts {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeStore) AccountByID(_ context.Context, userID string) (Account, error) {
	a, ok := f.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeStore) EmailExists(context.Context, string) (bool, error) { return f.emailExists, nil }

func (f *fakeStore) DepotExists(context.Context, string) (bool, error) { return f.depotExists, nil }

func (f *fakeStore) OutletDepot(context.Context, string) (string, error) {
	if f.outletDepot == "" {
		return "", ValidationError{Field: "outletId", Message: "is not a known outlet"}
	}
	return f.outletDepot, nil
}

func (f *fakeStore) CreateAccount(_ context.Context, in CreateInput, passwordHash, actor string) (Account, error) {
	if f.createErr != nil {
		return Account{}, f.createErr
	}
	f.created = in
	f.createdFor = actor
	f.createdPwd = passwordHash
	a := Account{
		UserID: "usr_new", Email: in.Email, DisplayName: in.DisplayName,
		Role: in.Role, DepotID: in.DepotID, OutletID: in.OutletID, Active: true,
	}
	f.accounts[a.UserID] = a
	f.hashes[a.UserID] = passwordHash
	return a, nil
}

func (f *fakeStore) UpdateAccount(_ context.Context, userID string, in UpdateInput, actor string) (Account, error) {
	f.updated = in
	a, ok := f.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	if in.DisplayName != nil {
		a.DisplayName = *in.DisplayName
	}
	if in.DepotID != nil {
		a.DepotID = *in.DepotID
	}
	if in.OutletID != nil {
		a.OutletID = *in.OutletID
	}
	f.accounts[userID] = a
	return a, nil
}

func (f *fakeStore) SetActive(_ context.Context, userID string, active bool, actor string) (Account, error) {
	f.activeSet = &active
	a, ok := f.accounts[userID]
	if !ok {
		return Account{}, ErrNotFound
	}
	a.Active = active
	f.accounts[userID] = a
	return a, nil
}

func (f *fakeStore) CreateResetToken(_ context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if f.createResetErr != nil {
		return f.createResetErr
	}
	f.resetForUser, f.resetHash, f.resetExpiry = userID, tokenHash, expiresAt
	return nil
}

func (f *fakeStore) ConsumeResetToken(_ context.Context, tokenHash, newPasswordHash string, now time.Time) (string, error) {
	if f.consumeErr != nil {
		return "", f.consumeErr
	}
	f.consumedHash, f.consumedPwd, f.consumedNow = tokenHash, newPasswordHash, now
	return f.consumeUserID, nil
}

func (f *fakeStore) UserIDByEmail(context.Context, string) (string, error) {
	return f.userIDByEmail, nil
}

// --- tests -----------------------------------------------------------------

func TestCreateAccountDriver(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, nil)

	acc, err := svc.CreateAccount(context.Background(), CreateInput{
		Email:           "Kasun.P@Waypoint.lk",
		DisplayName:     "Kasun P.",
		Role:            domain.RoleDriver,
		DepotID:         "d-peli",
		InitialPassword: "secret-pass",
	}, "seed-dispatcher")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acc.Role != domain.RoleDriver || acc.DepotID != "d-peli" || acc.OutletID != "" {
		t.Fatalf("account = %+v", acc)
	}
	if store.created.Email != "kasun.p@waypoint.lk" {
		t.Fatalf("email not normalised: %q", store.created.Email)
	}
	if store.createdPwd == "" || store.createdPwd == "secret-pass" {
		t.Fatal("initial password was not hashed before storage")
	}
	if err := password.Verify("secret-pass", store.createdPwd); err != nil {
		t.Fatalf("stored hash does not verify the plaintext: %v", err)
	}
	if store.createdFor != "seed-dispatcher" {
		t.Fatalf("actor = %q; the client-supplied actor must not be trusted", store.createdFor)
	}
}

func TestCreateAccountStoreManagerDerivesDepot(t *testing.T) {
	store := newFakeStore()
	store.outletDepot = "d-peli"
	svc := NewService(store, nil)

	acc, err := svc.CreateAccount(context.Background(), CreateInput{
		Email:           "ishara.s@waypoint.lk",
		DisplayName:     "Ishara S.",
		Role:            domain.RoleStoreManager,
		OutletID:        "OUT014",
		DepotID:         "d-KANDY-SPOOF", // must be ignored
		InitialPassword: "secret-pass",
	}, "seed-dispatcher")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acc.DepotID != "d-peli" {
		t.Fatalf("depot = %q; should derive from the outlet", acc.DepotID)
	}
	if store.created.DepotID != "d-peli" {
		t.Fatalf("client depot spoofed: %q", store.created.DepotID)
	}
}

func TestCreateAccountRejectsDispatcherRole(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, nil)

	_, err := svc.CreateAccount(context.Background(), CreateInput{
		Email:           "spoof@waypoint.lk",
		DisplayName:     "Spoof",
		Role:            domain.RoleDispatcher,
		DepotID:         "d-peli",
		InitialPassword: "secret-pass",
	}, "seed-dispatcher")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for a DISPATCHER role", err)
	}
}

func TestCreateAccountRejectsUnknownRole(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	_, err := svc.CreateAccount(context.Background(), CreateInput{
		Email: "x@waypoint.lk", DisplayName: "X", Role: domain.Role("ADMIN"),
		DepotID: "d-peli", InitialPassword: "secret-pass",
	}, "seed-dispatcher")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for an unknown role", err)
	}
}

func TestCreateAccountRoleFieldRules(t *testing.T) {
	svc := NewService(newFakeStore(), nil)

	t.Run("driver requires a depot", func(t *testing.T) {
		_, err := svc.CreateAccount(context.Background(), CreateInput{
			Email: "d@waypoint.lk", DisplayName: "D", Role: domain.RoleDriver,
			InitialPassword: "secret-pass",
		}, "actor")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("driver must not carry an outlet", func(t *testing.T) {
		_, err := svc.CreateAccount(context.Background(), CreateInput{
			Email: "d@waypoint.lk", DisplayName: "D", Role: domain.RoleDriver,
			DepotID: "d-peli", OutletID: "OUT014", InitialPassword: "secret-pass",
		}, "actor")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("store manager requires an outlet", func(t *testing.T) {
		_, err := svc.CreateAccount(context.Background(), CreateInput{
			Email: "s@waypoint.lk", DisplayName: "S", Role: domain.RoleStoreManager,
			InitialPassword: "secret-pass",
		}, "actor")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})
}

func TestCreateAccountPasswordPolicy(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	_, err := svc.CreateAccount(context.Background(), CreateInput{
		Email: "d@waypoint.lk", DisplayName: "D", Role: domain.RoleDriver,
		DepotID: "d-peli", InitialPassword: "short",
	}, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for a short password", err)
	}
}

func TestCreateAccountInvalidEmail(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	for _, bad := range []string{"", "no-at", "a@b", "a b@c.d"} {
		_, err := svc.CreateAccount(context.Background(), CreateInput{
			Email: bad, DisplayName: "D", Role: domain.RoleDriver,
			DepotID: "d-peli", InitialPassword: "secret-pass",
		}, "actor")
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("email %q: err = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestCreateAccountDuplicateEmail(t *testing.T) {
	store := newFakeStore()
	store.emailExists = true
	svc := NewService(store, nil)
	_, err := svc.CreateAccount(context.Background(), CreateInput{
		Email: "dup@waypoint.lk", DisplayName: "D", Role: domain.RoleLoader,
		DepotID: "d-peli", InitialPassword: "secret-pass",
	}, "actor")
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("err = %v, want ErrDuplicateEmail", err)
	}
}

func TestCreateAccountInvalidDepot(t *testing.T) {
	store := newFakeStore()
	store.depotExists = false
	svc := NewService(store, nil)
	_, err := svc.CreateAccount(context.Background(), CreateInput{
		Email: "d@waypoint.lk", DisplayName: "D", Role: domain.RoleDriver,
		DepotID: "nope", InitialPassword: "secret-pass",
	}, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for an unknown depot", err)
	}
}

func TestCreateAccountInvalidOutlet(t *testing.T) {
	store := newFakeStore()
	store.outletDepot = "" // no such outlet
	svc := NewService(store, nil)
	_, err := svc.CreateAccount(context.Background(), CreateInput{
		Email: "s@waypoint.lk", DisplayName: "S", Role: domain.RoleStoreManager,
		OutletID: "OUT999", InitialPassword: "secret-pass",
	}, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for an unknown outlet", err)
	}
}

func TestUpdateAccountStoreManagerDerivesDepot(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleStoreManager, OutletID: "OUT001", DepotID: "d-old"}
	store.outletDepot = "d-peli"
	svc := NewService(store, nil)

	outlet := "OUT014"
	acc, err := svc.UpdateAccount(context.Background(), "u1", UpdateInput{OutletID: &outlet}, "actor")
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if acc.OutletID != "OUT014" || acc.DepotID != "d-peli" {
		t.Fatalf("account = %+v; depot should follow the outlet", acc)
	}
}

func TestUpdateAccountRejectsIndependentDepotForStoreManager(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleStoreManager, OutletID: "OUT001", DepotID: "d-peli"}
	svc := NewService(store, nil)

	depot := "d-other"
	_, err := svc.UpdateAccount(context.Background(), "u1", UpdateInput{DepotID: &depot}, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v; a store manager must not set depot independently", err)
	}
}

func TestUpdateAccountRejectsOutletForDriver(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleDriver, DepotID: "d-peli"}
	svc := NewService(store, nil)

	outlet := "OUT014"
	_, err := svc.UpdateAccount(context.Background(), "u1", UpdateInput{OutletID: &outlet}, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v; a driver must not be given an outlet", err)
	}
}

func TestUpdateAccountRejectsDispatcherRole(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleDispatcher}
	svc := NewService(store, nil)
	name := "New Name"
	_, err := svc.UpdateAccount(context.Background(), "u1", UpdateInput{DisplayName: &name}, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v; a dispatcher account is not manageable here", err)
	}
}

func TestSetActiveRejectsDispatcher(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleDispatcher, Active: true}
	svc := NewService(store, nil)
	_, err := svc.SetActive(context.Background(), "u1", false, "actor")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v; a dispatcher account must not be deactivated here", err)
	}
}

func TestSetActiveOperational(t *testing.T) {
	store := newFakeStore()
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleLoader, Active: true}
	svc := NewService(store, nil)
	acc, err := svc.SetActive(context.Background(), "u1", false, "actor")
	if err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if acc.Active {
		t.Fatal("account should be deactivated")
	}
	if store.activeSet == nil || *store.activeSet {
		t.Fatal("store was not told to deactivate")
	}
}

func TestListAccountsExcludesDispatchers(t *testing.T) {
	store := newFakeStore()
	store.accounts["d1"] = Account{UserID: "d1", Role: domain.RoleDispatcher, Active: true}
	store.accounts["u1"] = Account{UserID: "u1", Role: domain.RoleDriver, Active: true}
	svc := NewService(store, nil)

	got, err := svc.ListAccounts(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(got) != 1 || got[0].UserID != "u1" {
		t.Fatalf("accounts = %+v; a dispatcher account must not be listed", got)
	}
}

func TestListAccountsRejectsDispatcherFilter(t *testing.T) {
	svc := NewService(newFakeStore(), nil)
	if _, err := svc.ListAccounts(context.Background(), domain.RoleDispatcher, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid for a DISPATCHER role filter", err)
	}
}

func TestForgotPasswordUnknownEmailIsGeneric(t *testing.T) {
	store := newFakeStore() // userIDByEmail returns ""
	svc := NewService(store, nil)
	raw, matched, err := svc.ForgotPassword(context.Background(), "nobody@waypoint.lk")
	if err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	if matched || raw != "" {
		t.Fatalf("matched=%v raw=%q; an unknown email must not mint a token", matched, raw)
	}
}

func TestForgotPasswordStoresOnlyHash(t *testing.T) {
	store := newFakeStore()
	store.userIDByEmail = "u1"
	now := time.Date(2026, 9, 25, 15, 40, 0, 0, time.UTC)
	svc := NewService(store, fakeClock{now})

	raw, matched, err := svc.ForgotPassword(context.Background(), "kasun.p@waypoint.lk")
	if err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	if !matched || raw == "" {
		t.Fatalf("matched=%v raw=%q", matched, raw)
	}
	if store.resetHash == HashResetToken(raw) {
		// expected
	} else {
		t.Fatalf("stored hash %q is not HashResetToken(raw)", store.resetHash)
	}
	if store.resetHash == raw {
		t.Fatal("raw reset token was stored instead of its hash")
	}
	if store.resetForUser != "u1" {
		t.Fatalf("token stored for %q, want u1", store.resetForUser)
	}
	if !store.resetExpiry.After(now) {
		t.Fatalf("expiry %v is not in the future", store.resetExpiry)
	}
	if store.resetExpiry.Sub(now) > resetTokenTTL+time.Minute {
		t.Fatalf("expiry %v is longer than the TTL", store.resetExpiry.Sub(now))
	}
}

func TestResetPassword(t *testing.T) {
	store := newFakeStore()
	store.consumeUserID = "u1"
	svc := NewService(store, nil)

	err := svc.ResetPassword(context.Background(), ResetInput{
		Token: "raw-token", NewPassword: "new-secret", ConfirmPassword: "new-secret",
	})
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if store.consumedHash != HashResetToken("raw-token") {
		t.Fatalf("consumed hash = %q, want HashResetToken(token)", store.consumedHash)
	}
	if store.consumedPwd == "" || store.consumedPwd == "new-secret" {
		t.Fatal("new password was not hashed")
	}
	if err := password.Verify("new-secret", store.consumedPwd); err != nil {
		t.Fatalf("stored hash does not verify the new password: %v", err)
	}
}

// TestResetPasswordUsesInjectedClock pins the regression where token expiry was
// compared against the database's real-time now() while the token had been
// minted from the injected business clock. Under DEMO_MODE (a past seeded day)
// that made every freshly minted token appear already expired. The store must
// receive the same instant the service minted with, so the two ends of the
// comparison live in one time domain.
func TestResetPasswordUsesInjectedClock(t *testing.T) {
	store := newFakeStore()
	store.consumeUserID = "u1"
	businessNow := time.Date(2026, 9, 25, 15, 40, 0, 0, time.UTC)
	svc := NewService(store, fakeClock{businessNow})

	if err := svc.ResetPassword(context.Background(), ResetInput{
		Token: "raw-token", NewPassword: "new-secret", ConfirmPassword: "new-secret",
	}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if !store.consumedNow.Equal(businessNow) {
		t.Fatalf("consume used %v, want the injected business clock %v", store.consumedNow, businessNow)
	}
}

func TestResetPasswordValidation(t *testing.T) {
	svc := NewService(newFakeStore(), nil)

	cases := []struct {
		name string
		in   ResetInput
	}{
		{"empty token", ResetInput{Token: "", NewPassword: "new-secret", ConfirmPassword: "new-secret"}},
		{"short password", ResetInput{Token: "t", NewPassword: "short", ConfirmPassword: "short"}},
		{"mismatch", ResetInput{Token: "t", NewPassword: "new-secret", ConfirmPassword: "different"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.ResetPassword(context.Background(), tc.in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestResetPasswordInvalidTokenPropagates(t *testing.T) {
	store := newFakeStore()
	store.consumeErr = ErrInvalidResetToken
	svc := NewService(store, nil)
	err := svc.ResetPassword(context.Background(), ResetInput{
		Token: "t", NewPassword: "new-secret", ConfirmPassword: "new-secret",
	})
	if !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("err = %v, want ErrInvalidResetToken", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Foo.Bar@Example.LK "); got != "foo.bar@example.lk" {
		t.Fatalf("NormalizeEmail = %q", got)
	}
}

func TestHashResetTokenIsDeterministicAndHex(t *testing.T) {
	h := HashResetToken("abc")
	if len(h) != 64 || strings.ToLower(h) != h {
		t.Fatalf("hash = %q, want a 64-char lowercase hex string", h)
	}
	if HashResetToken("abc") != h {
		t.Fatal("hash is not deterministic")
	}
}

// fakeClock is a deterministic Clock.
type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }
