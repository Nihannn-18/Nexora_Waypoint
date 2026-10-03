package password

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	const plain = "waypoint2026"
	hash, err := Hash(plain)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash = %q, want an argon2id PHC string", hash)
	}
	if strings.Contains(hash, plain) {
		t.Fatal("hash contains the plaintext password")
	}

	t.Run("correct password verifies", func(t *testing.T) {
		if err := Verify(plain, hash); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})

	t.Run("wrong password is rejected", func(t *testing.T) {
		if err := Verify("not-the-password", hash); !errors.Is(err, ErrMismatch) {
			t.Fatalf("err = %v, want ErrMismatch", err)
		}
	})

	t.Run("same password hashes differently (random salt)", func(t *testing.T) {
		other, err := Hash(plain)
		if err != nil {
			t.Fatalf("Hash: %v", err)
		}
		if other == hash {
			t.Fatal("two hashes of the same password are identical; salt is not random")
		}
		if err := Verify(plain, other); err != nil {
			t.Fatalf("Verify other: %v", err)
		}
	})
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"not phc":        "plaintext",
		"wrong algo":     "$bcrypt$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA",
		"too few parts":  "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA",
		"bad base64":     "$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA",
		"bad params":     "$argon2id$v=19$nope$c2FsdA$aGFzaA",
		"impossible mem": "$argon2id$v=19$m=1,t=1,p=4$c2FsdA$aGFzaA",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Verify("anything", encoded); !errors.Is(err, ErrInvalidHash) {
				t.Fatalf("err = %v, want ErrInvalidHash", err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("short"); err == nil {
		t.Fatal("Validate accepted a too-short password")
	}
	if err := Validate("longenough"); err != nil {
		t.Fatalf("Validate rejected a valid password: %v", err)
	}
}

func TestDummyHashIsUnverifiable(t *testing.T) {
	h := DummyHash()
	if h == "" {
		t.Fatal("DummyHash returned empty")
	}
	// Nobody knows the dummy plaintext, but the hash must be well-formed so the
	// verify path still does real Argon2id work.
	if err := Verify("waypoint2026", h); err == nil {
		t.Fatal("a real password verified against the dummy hash")
	}
}
