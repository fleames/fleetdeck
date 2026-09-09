package secrets

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists envelope-encrypted values in the `secrets` table.
// Product features that need ciphertext should use this; nothing is written
// automatically at boot — the table stays empty until Put is called.
type Store struct {
	pool *pgxpool.Pool
	key  []byte
}

func NewStore(pool *pgxpool.Pool, sessionSecret string) (*Store, error) {
	key, err := DeriveKey(sessionSecret)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, key: key}, nil
}

// Put encrypts plaintext and upserts by (kind, name).
func (s *Store) Put(ctx context.Context, kind, name string, plaintext []byte) (uuid.UUID, error) {
	if kind == "" || name == "" {
		return uuid.Nil, fmt.Errorf("kind and name required")
	}
	env, err := Encrypt(s.key, plaintext)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO secrets (kind, name, ciphertext, nonce, key_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (kind, name) DO UPDATE SET
			ciphertext = EXCLUDED.ciphertext,
			nonce = EXCLUDED.nonce,
			key_version = EXCLUDED.key_version,
			rotated_at = now()
		RETURNING id`,
		kind, name, env.Ciphertext, env.Nonce, env.KeyVersion,
	).Scan(&id)
	return id, err
}

// Get decrypts and returns plaintext for (kind, name).
func (s *Store) Get(ctx context.Context, kind, name string) ([]byte, error) {
	var env Envelope
	err := s.pool.QueryRow(ctx, `
		SELECT ciphertext, nonce, key_version FROM secrets WHERE kind=$1 AND name=$2`,
		kind, name,
	).Scan(&env.Ciphertext, &env.Nonce, &env.KeyVersion)
	if err != nil {
		return nil, err
	}
	return Decrypt(s.key, env)
}

// Delete removes a secret row.
func (s *Store) Delete(ctx context.Context, kind, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM secrets WHERE kind=$1 AND name=$2`, kind, name)
	return err
}
