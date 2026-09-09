package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		sqlText := string(body)
		// Strip simple migrate markers if present
		sqlText = strings.ReplaceAll(sqlText, "-- +migrate Up", "")
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, sqlText); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(filename) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	// Always heal orphan inventory (safe no-op when FKs/CASCADE already correct).
	if err := CleanOrphanedInventory(ctx, pool); err != nil {
		return fmt.Errorf("orphan cleanup: %w", err)
	}
	return nil
}

// CleanOrphanedInventory removes docker/fleet rows whose server_id no longer exists.
// Runs on every API start so already-deleted servers stop appearing in UI categories.
func CleanOrphanedInventory(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`DELETE FROM container_metrics_raw c WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = c.server_id)`,
		`DELETE FROM server_metrics_raw m WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = m.server_id)`,
		`DELETE FROM server_metrics_5m m WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = m.server_id)`,
		`DELETE FROM server_metrics_1h m WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = m.server_id)`,
		`DELETE FROM agent_commands c WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = c.server_id)`,
		`DELETE FROM containers c WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = c.server_id)`,
		`DELETE FROM images i WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = i.server_id)`,
		`DELETE FROM volumes v WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = v.server_id)`,
		`DELETE FROM networks n WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = n.server_id)`,
		`DELETE FROM compose_projects p WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = p.server_id)`,
		`DELETE FROM docker_hosts d WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = d.server_id)`,
		`DELETE FROM agents a WHERE NOT EXISTS (SELECT 1 FROM servers s WHERE s.id = a.server_id)`,
	}
	for _, q := range stmts {
		if _, err := pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
