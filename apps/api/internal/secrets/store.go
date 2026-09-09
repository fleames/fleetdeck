package secrets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists envelope-encrypted values in the `secrets` table.
// Product features that need ciphertext should use this; nothing is written
// automatically at boot — Put via admin API or code paths.
type Store struct {
	pool *pgxpool.Pool
	key  []byte
}

// Meta is non-sensitive listing metadata for a secret row.
type Meta struct {
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	KeyVersion int        `json:"key_version"`
	CreatedAt  time.Time  `json:"created_at"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
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

// List returns metadata for all secrets (never plaintext).
func (s *Store) List(ctx context.Context) ([]Meta, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT kind, name, key_version, created_at, rotated_at
		FROM secrets ORDER BY kind, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Meta
	for rows.Next() {
		var m Meta
		if err := rows.Scan(&m.Kind, &m.Name, &m.KeyVersion, &m.CreatedAt, &m.RotatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Exists reports whether (kind, name) is stored.
func (s *Store) Exists(ctx context.Context, kind, name string) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM secrets WHERE kind=$1 AND name=$2`, kind, name).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
