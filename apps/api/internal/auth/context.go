package auth

import (
	"context"
	"net/http"
)

// contextKey is unexported so no other package can collide with or spoof the
// identity stored on a request context.
type contextKey struct{}

var identityKey contextKey

// WithIdentity returns a copy of ctx carrying id. It is called by the auth
// middleware after a request is verified, never by handlers directly.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// IdentityFrom returns the authenticated identity stored on ctx.
//
// The second return value is false when the request never passed through the
// auth middleware, which is a wiring error rather than a client error; handlers
// should treat it as unauthenticated and let the middleware have already
// answered.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	if !ok {
		return Identity{}, false
	}
	return id, true
}

// MustIdentity returns the identity or a wiring-error sentinel. Use it only on
// routes guaranteed to sit behind RequireAuthenticated; the panic-free error
// path lets a handler return 500 for a genuine misconfiguration rather than
// silently treating the caller as anonymous.
func MustIdentity(ctx context.Context) (Identity, error) {
	id, ok := IdentityFrom(ctx)
	if !ok {
		return Identity{}, ErrNoIdentity
	}
	return id, nil
}

// HTTPRequestHeader extracts the minimal, transport-neutral header view the
// verifier consumes from a request. It copies only the two headers the (TBD)
// Better Auth bridge may need.
func HTTPRequestHeader(r *http.Request) *RequestHeader {
	return &RequestHeader{
		Authorization: r.Header.Get("Authorization"),
		Cookie:        r.Header.Get("Cookie"),
	}
}
