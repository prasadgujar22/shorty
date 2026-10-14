package pg

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate applies every *.sql file in fsys (sorted by name) that has not yet
// been recorded in schema_migrations, each in its own transaction.
//
// The target schema must already exist and be owned by the service role
// (it is created by the database bootstrap, see deploy/charts/shorty), so the
// service needs no CREATE privilege on the database. A session-level
// advisory lock serialises concurrent replicas starting at the same time.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	var schema string
	if err := conn.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		return fmt.Errorf("current schema: %w", err)
	}
	if schema == "" {
		return fmt.Errorf("no current schema: does the schema in search_path exist?")
	}
	lockID := lockKey(schema)
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockID) }()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	applied := 0
	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
			return err
		}); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		applied++
		log.Info("migration applied", "schema", schema, "version", version)
	}
	log.Info("migrations up to date", "schema", schema, "applied", applied, "total", len(names))
	return nil
}

func lockKey(schema string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("shorty-migrate:" + schema))
	return int64(h.Sum64() >> 1)
}
