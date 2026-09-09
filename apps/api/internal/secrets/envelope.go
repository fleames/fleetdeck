// Package secrets provides application-level envelope encryption for the
// reserved `secrets` table. The key is derived from SESSION_SECRET.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	KeyVersionV1 = 1
	hkdfInfo     = "fleetdeck-secrets-v1"
)

// Envelope holds ciphertext + nonce for AES-GCM storage.
type Envelope struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
}

// DeriveKey turns SESSION_SECRET into a 32-byte AES key (HKDF-SHA256).
func DeriveKey(sessionSecret string) ([]byte, error) {
	if len(sessionSecret) < 32 {
		return nil, fmt.Errorf("session secret too short")
	}
	r := hkdf.New(sha256.New, []byte(sessionSecret), nil, []byte(hkdfInfo))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}

// Encrypt seals plaintext with AES-256-GCM using the derived key.
func Encrypt(key, plaintext []byte) (Envelope, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return Envelope{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Envelope{}, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return Envelope{Ciphertext: ct, Nonce: nonce, KeyVersion: KeyVersionV1}, nil
}

// Decrypt opens an envelope produced by Encrypt.
func Decrypt(key []byte, env Envelope) ([]byte, error) {
	if env.KeyVersion != 0 && env.KeyVersion != KeyVersionV1 {
		return nil, fmt.Errorf("unsupported key_version %d", env.KeyVersion)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(env.Nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size")
	}
	return gcm.Open(nil, env.Nonce, env.Ciphertext, nil)
}
