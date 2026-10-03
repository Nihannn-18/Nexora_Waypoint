package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// sessionTokenBytes is the entropy of a session token: 256 bits, base64url
// encoded to 43 characters. Large enough that guessing is infeasible.
const sessionTokenBytes = 32

// NewSessionToken returns a fresh opaque session token and its lookup hash.
//
// The raw token is returned to the client exactly once, at login. Only the hash
// is persisted, so a database read cannot reconstruct a usable token. The token
// is random bytes, never a predictable id, timestamp or user id.
func NewSessionToken() (raw, hash string, err error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("auth: read session token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashSessionToken(raw), nil
}

// HashSessionToken returns the hex SHA-256 of a raw session token. It is the
// value stored in session.token_hash and the value looked up on each request.
func HashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// SessionValidator resolves a raw session token to the owning app_user id.
//
// It is implemented by the database-backed store (internal/authstore). The
// raw token never crosses this boundary beyond the call; the store hashes it
// and looks up the session row, requiring a live, unexpired session for an
// active user.
type SessionValidator interface {
	// ValidateToken returns the app_user user id for an active, unexpired
	// session, or ErrInvalidSession when the token is unknown or expired.
	ValidateToken(ctx context.Context, rawToken string) (userID string, err error)
}

// OpaqueSessionVerifier implements RequestVerifier over database-backed,
// server-side opaque sessions. It is the production verifier behind the
// existing auth middleware; SessionTokenVerifier remains only as the
// explicitly-unconfigured, fail-closed placeholder.
type OpaqueSessionVerifier struct {
	sessions SessionValidator
}

// NewOpaqueSessionVerifier builds a verifier over a session store.
func NewOpaqueSessionVerifier(sessions SessionValidator) *OpaqueSessionVerifier {
	return &OpaqueSessionVerifier{sessions: sessions}
}

// Verify extracts the bearer token and resolves it. Missing or malformed
// credentials are ErrUnauthenticated; an unknown or expired session is
// ErrInvalidSession. Only the Authorization header establishes identity — never
// X-User-ID, X-Role, X-Depot-ID or X-Outlet-ID, which are not read at all.
func (v *OpaqueSessionVerifier) Verify(ctx context.Context, h *RequestHeader) (Identity, error) {
	if v == nil || v.sessions == nil {
		return Identity{}, ErrVerificationUnavailable
	}
	if h == nil {
		return Identity{}, ErrUnauthenticated
	}
	raw, ok := BearerToken(h.Authorization)
	if !ok {
		return Identity{}, ErrUnauthenticated
	}
	userID, err := v.sessions.ValidateToken(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	if userID == "" {
		return Identity{}, ErrInvalidSession
	}
	return Identity{UserID: userID}, nil
}

const bearerScheme = "bearer "

// BearerToken extracts the token from an "Authorization: Bearer <token>"
// header, case-insensitively. It returns false for a missing or malformed
// header or an empty token.
func BearerToken(authorization string) (string, bool) {
	value := strings.TrimSpace(authorization)
	if len(value) < len(bearerScheme) || !strings.EqualFold(value[:len(bearerScheme)], bearerScheme) {
		return "", false
	}
	token := strings.TrimSpace(value[len(bearerScheme):])
	if token == "" {
		return "", false
	}
	return token, true
}
