package secrets

import (
	"bytes"
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	key, err := DeriveKey("dev-only-replace-with-openssl-rand-base64-48-chars-min")
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("webhook-token-example")
	env, err := Encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Ciphertext) == 0 || len(env.Nonce) == 0 {
		t.Fatal("empty ciphertext/nonce")
	}
	if bytes.Contains(env.Ciphertext, plain) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	got, err := Decrypt(key, env)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestDeriveKeyRequiresLength(t *testing.T) {
	if _, err := DeriveKey("too-short"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	k1, _ := DeriveKey("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	k2, _ := DeriveKey("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	env, err := Encrypt(k1, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(k2, env); err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}
