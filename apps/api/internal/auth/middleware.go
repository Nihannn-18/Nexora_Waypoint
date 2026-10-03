package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"waypoint.lk/api/internal/domain"
	"waypoint.lk/api/internal/httpx"
)

// Authorizer answers the reusable authorization questions any domain handler
// asks. Each method returns nil when allowed, or an error wrapping
// ErrUnauthenticated / ErrForbidden so the HTTP layer can map it consistently.
//
// Domain packages depend on this interface rather than on net/http, so scope
// rules stay unit-testable without a server. Handlers must not re-implement
// role or scope checks inline.
type Authorizer interface {
	// Authenticated reports whether an identity is present and usable.
	Authenticated(ctx context.Context) error
	// RequireRole requires an identity holding exactly role.
	RequireRole(ctx context.Context, role domain.Role) error
	// RequireAnyRole requires an identity holding one of roles.
	RequireAnyRole(ctx context.Context, roles ...domain.Role) error
	// RequireScope requires the identity to be within depotID. An empty depotID
	// means "any depot the identity may see" and only checks authentication.
	RequireScope(ctx context.Context, depotID string) error
	// RequireOutlet requires the identity to be within outletID.
	RequireOutlet(ctx context.Context, outletID string) error
}

// LocalAuthorizer enforces authorization from the identity stored in the
// request context. It holds no state and is safe for concurrent use.
type LocalAuthorizer struct{}

// Authenticated reports ErrUnauthenticated when no usable identity is present.
func (LocalAuthorizer) Authenticated(ctx context.Context) error {
	id, ok := IdentityFrom(ctx)
	if !ok || !id.Authenticated() {
		return ErrUnauthenticated
	}
	return nil
}

// RequireRole requires exactly role.
func (a LocalAuthorizer) RequireRole(ctx context.Context, role domain.Role) error {
	id, err := a.require(ctx)
	if err != nil {
		return err
	}
	if !id.HasRole(role) {
		return fmt.Errorf("%w: requires role %s", ErrForbidden, role)
	}
	return nil
}

// RequireAnyRole requires one of roles.
func (a LocalAuthorizer) RequireAnyRole(ctx context.Context, roles ...domain.Role) error {
	id, err := a.require(ctx)
	if err != nil {
		return err
	}
	if !id.HasAnyRole(roles...) {
		return fmt.Errorf("%w: requires one of %v", ErrForbidden, roles)
	}
	return nil
}

// RequireScope requires depotID to be inside the identity's scope. An empty
// depotID only requires authentication.
func (a LocalAuthorizer) RequireScope(ctx context.Context, depotID string) error {
	id, err := a.require(ctx)
	if err != nil {
		return err
	}
	if depotID == "" || id.CanAccessDepot(depotID) {
		return nil
	}
	return fmt.Errorf("%w: depot out of scope", ErrForbidden)
}

// RequireOutlet requires outletID to be inside the identity's scope.
func (a LocalAuthorizer) RequireOutlet(ctx context.Context, outletID string) error {
	id, err := a.require(ctx)
	if err != nil {
		return err
	}
	if outletID == "" || id.CanAccessOutlet(outletID) {
		return nil
	}
	return fmt.Errorf("%w: outlet out of scope", ErrForbidden)
}

func (LocalAuthorizer) require(ctx context.Context) (Identity, error) {
	id, ok := IdentityFrom(ctx)
	if !ok || !id.Authenticated() {
		return Identity{}, ErrUnauthenticated
	}
	return id, nil
}

// NewAuthorizer returns the context-backed authorizer domain handlers use.
func NewAuthorizer() Authorizer { return LocalAuthorizer{} }

// Middleware builds net/http middleware from an identity loader and an
// authorizer. The zero-value-avoiding constructor keeps the two dependencies
// explicit at the composition root (main/Router).
type Middleware struct {
	loader     *IdentityLoader
	authorizer Authorizer
}

// NewMiddleware builds auth middleware. A nil authorizer defaults to the
// context-backed LocalAuthorizer.
func NewMiddleware(loader *IdentityLoader, authorizer Authorizer) *Middleware {
	if authorizer == nil {
		authorizer = LocalAuthorizer{}
	}
	return &Middleware{loader: loader, authorizer: authorizer}
}

// RequireAuthenticated verifies the request and stores the identity on the
// context before calling next. It answers 401 with the standard error body when
// the request is not authenticated, and 500 when verification is misconfigured
// (the TBD case) so the failure is visible rather than silently 401ing.
func (m *Middleware) RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := m.loader.Load(r.Context(), HTTPRequestHeader(r))
		if err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// RequireRole verifies the request, stores the identity, then requires the
// caller's role to be exactly role.
func (m *Middleware) RequireRole(role domain.Role, next http.Handler) http.Handler {
	return m.RequireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := m.authorizer.RequireRole(r.Context(), role); err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireAnyRole verifies the request, stores the identity, then requires the
// caller's role to be one of roles.
func (m *Middleware) RequireAnyRole(roles []domain.Role, next http.Handler) http.Handler {
	return m.RequireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := m.authorizer.RequireAnyRole(r.Context(), roles...); err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireScope verifies the request, stores the identity, then requires the
// caller to be within depotID.
func (m *Middleware) RequireScope(depotID string, next http.Handler) http.Handler {
	return m.RequireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := m.authorizer.RequireScope(r.Context(), depotID); err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// writeAuthError maps an auth error to the shared error contract:
//
//	missing/invalid credentials           -> 401 UNAUTHENTICATED
//	verification mechanism not configured -> 500 INTERNAL_ERROR (fail loud)
//	authenticated but not permitted       -> 403 FORBIDDEN
//
// The message is deliberately generic; no token, session or scope value is
// echoed, so no credential material can leak to the client or the logs.
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnauthenticated), errors.Is(err, ErrInvalidSession):
		httpx.WriteErrorCode(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "Authentication required")
	case errors.Is(err, ErrForbidden):
		httpx.WriteErrorCode(w, http.StatusForbidden, httpx.CodeForbidden, "You do not have access to this resource")
	case errors.Is(err, ErrVerificationUnavailable):
		// The bridge is not wired. Fail closed and surface it as a server fault
		// so it is fixed, rather than pretending the caller is anonymous.
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Authentication is not configured")
	default:
		httpx.WriteErrorCode(w, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong on our side")
	}
}
