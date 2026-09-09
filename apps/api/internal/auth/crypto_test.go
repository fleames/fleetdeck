package auth_test

import (
	"testing"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/auth"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPassword(hash, "correct-horse-battery") {
		t.Fatal("expected password to verify")
	}
	if auth.VerifyPassword(hash, "wrong-password-here") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestTokenHashStable(t *testing.T) {
	token, err := auth.NewToken(16)
	if err != nil {
		t.Fatal(err)
	}
	a := auth.HashToken(token)
	b := auth.HashToken(token)
	if a != b || a == "" {
		t.Fatalf("hash mismatch %q vs %q", a, b)
	}
}
