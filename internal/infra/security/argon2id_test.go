package security

import (
	"strings"
	"testing"
)

func TestArgon2IDHasherHashAndVerify(t *testing.T) {
	hasher := NewArgon2IDHasher(DefaultArgon2IDConfig())

	hash, err := hasher.Hash("StrongPassw0rd!")
	if err != nil {
		t.Fatalf("hash returned error: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("expected argon2id hash format, got %q", hash)
	}

	ok, err := hasher.Verify("StrongPassw0rd!", hash)
	if err != nil {
		t.Fatalf("verify returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify successfully")
	}

	ok, err = hasher.Verify("WrongPassw0rd!", hash)
	if err != nil {
		t.Fatalf("verify returned error for wrong password: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password verification to fail")
	}
}
