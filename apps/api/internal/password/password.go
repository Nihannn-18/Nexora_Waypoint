// Package password hashes and verifies account passwords with Argon2id.
//
// Argon2id is the memory-hard password hashing function recommended by OWASP;
// it ships in golang.org/x/crypto, so no new third-party dependency is added.
// The encoded form is the standard PHC string, which carries its own salt and
// parameters so parameters can change over time without breaking old hashes:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<base64-salt>$<base64-hash>
//
// Nothing here logs or returns the plaintext password.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Cost parameters. These are OWASP's minimum recommendation for Argon2id
// (19 MiB, t=2, p=1): strong enough for real use, cheap enough that the four
// demo logins and the test suite stay fast.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	argonKeyLen    = 32
	saltLen        = 16
)

// MinLength is the shortest password the application sets. It is enforced when
// seeding credentials, not when verifying, so an existing password is never
// locked out by a policy change.
const MinLength = 8

// Errors returned by Verify. Callers treat any non-nil error as "not matching"
// and must not distinguish the cases to a client.
var (
	ErrInvalidHash = errors.New("password: invalid hash format")
	ErrMismatch    = errors.New("password: does not match")
)

// Validate reports whether plain is acceptable as a new password.
func Validate(plain string) error {
	if len([]rune(plain)) < MinLength {
		return fmt.Errorf("password must be at least %d characters", MinLength)
	}
	return nil
}

// Hash returns the encoded Argon2id hash of plain.
func Hash(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)

	b64 := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(key),
	), nil
}

// Verify reports whether plain matches the encoded hash. The comparison is
// constant-time. Any parse failure is ErrInvalidHash, never a panic.
func Verify(plain, encoded string) error {
	parts := strings.Split(encoded, "$")
	// Split of "$argon2id$v=19$m=..,t=..,p=..$salt$hash" yields a leading "".
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return ErrInvalidHash
	}

	var memory, timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return ErrInvalidHash
	}
	if memory < 8*uint32(threads) {
		return ErrInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return ErrInvalidHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return ErrInvalidHash
	}

	got := argon2.IDKey([]byte(plain), salt, timeCost, memory, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// DummyHash returns a valid hash of a value nobody can log in with. Login calls
// it so an unknown email still pays the Argon2id cost, which keeps the response
// time close to a wrong-password attempt and blunts account enumeration.
func DummyHash() string {
	dummyOnce.Do(func() {
		// A random, unguessable value; the result is never verifiable by a caller
		// because the value is discarded.
		var seed [32]byte
		_, _ = rand.Read(seed[:])
		dummyHash, _ = Hash(base64.RawStdEncoding.EncodeToString(seed[:]))
	})
	return dummyHash
}
