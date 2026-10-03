package auth

import (
	"context"
	"net/url"
	"strings"
)

// SessionVerifier is the persistence seam behind Better Auth verification.
//
// Better Auth, in the Next.js app, owns the session. This interface is
// implemented by the database-backed store (internal/authstore), which looks the
// session token up in Better Auth's `session` table, checks expiry, and maps the
// Better Auth user to the application authorization record by email. Keeping it
// an interface lets the verifier be unit-tested with a fake and keeps net/http
// and pgx out of this package's signature.
type SessionVerifier interface {
	// VerifySession resolves an active Better Auth session token into the
	// application user id (app_user.user_id) and the account email.
	//
	// It returns ErrInvalidSession when the token is unknown or expired, and
	// ErrForbidden when the session is valid but has no active app_user mapping.
	VerifySession(ctx context.Context, token string) (userID, email, name string, err error)
}

// BetterAuthSessionVerifier verifies a request against the real Better Auth
// session store. It replaces the fail-closed SessionTokenVerifier in production
// wiring; the stub remains for tests of the unconfigured case.
//
// It reads only the Authorization header (Bearer session token), with the
// Better Auth session cookie as a secondary path for server-side callers. It
// never trusts X-User-ID, X-Role, X-Depot-ID or X-Outlet-ID: identity is
// derived solely from the server-verified session.
type BetterAuthSessionVerifier struct {
	sessions SessionVerifier
}

// NewBetterAuthSessionVerifier builds a verifier over a session store.
func NewBetterAuthSessionVerifier(sessions SessionVerifier) *BetterAuthSessionVerifier {
	return &BetterAuthSessionVerifier{sessions: sessions}
}

// sessionCookieName is Better Auth's default session cookie. The bearer plugin
// accepts the same token in the Authorization header, which is what the web and
// Driver clients send.
const sessionCookieName = "better-auth.session_token"

// Verify parses the session token from the request and resolves it.
func (v *BetterAuthSessionVerifier) Verify(ctx context.Context, h *RequestHeader) (Identity, error) {
	if v == nil || v.sessions == nil {
		return Identity{}, ErrVerificationUnavailable
	}
	raw, ok := sessionToken(h)
	if !ok {
		return Identity{}, ErrUnauthenticated
	}
	userID, email, name, err := v.sessions.VerifySession(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	if userID == "" {
		return Identity{}, ErrInvalidSession
	}
	return Identity{UserID: userID, Email: email, Name: name}, nil
}

// sessionToken extracts the Better Auth session token from a request. It
// prefers the Authorization header (Bearer) and falls back to the session
// cookie.
func sessionToken(h *RequestHeader) (string, bool) {
	if h == nil {
		return "", false
	}
	if auth := strings.TrimSpace(h.Authorization); auth != "" {
		const scheme = "bearer "
		if len(auth) >= len(scheme) && strings.EqualFold(auth[:len(scheme)], scheme) {
			token := normalizeSessionToken(strings.TrimSpace(auth[len(scheme):]))
			return token, token != ""
		}
	}
	if value := cookieValue(h.Cookie, sessionCookieName); value != "" {
		token := normalizeSessionToken(value)
		return token, token != ""
	}
	return "", false
}

// normalizeSessionToken reduces the credential to the value Better Auth stores
// in session.token.
//
// Better Auth's session cookie is `serializeSignedCookie(session.token, secret)`
// — the token plus an HMAC signature — and its bearer plugin explicitly takes
// `token.split(".")[0]` as the session token before the session-table lookup.
// The session table stores the unsigned token, so we mirror that exact split.
// A token with no signature (Bearer RFC 6750 "b64token") is used as-is.
//
// The signature itself is not re-checked here: the authoritative check is the
// server-side session lookup, which only succeeds for a token Better Auth
// minted and has not expired — exactly how Better Auth validates its own cookie.
func normalizeSessionToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// The plugin URL-decodes a cookie-style value before splitting.
	if strings.Contains(raw, "%") {
		if decoded, err := url.QueryUnescape(raw); err == nil {
			raw = decoded
		}
	}
	// `token.signature` -> `token`. Only split when a token part remains.
	if i := strings.IndexByte(raw, '.'); i > 0 {
		return raw[:i]
	}
	if i := strings.IndexByte(raw, '.'); i == 0 {
		return ""
	}
	return raw
}

// cookieValue returns the value of a named cookie from a raw Cookie header.
func cookieValue(header, name string) string {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		if strings.TrimSpace(part[:eq]) == name {
			return strings.TrimSpace(part[eq+1:])
		}
	}
	return ""
}
