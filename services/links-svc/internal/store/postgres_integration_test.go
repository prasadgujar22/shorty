//go:build integration

package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prasadgujar22/shorty/pkg/pg"
	"github.com/prasadgujar22/shorty/services/links-svc/internal/links"
)

// newTestStore connects with TEST_DATABASE_URL (a superuser DSN), recreates a
// throwaway schema, and runs the real migrations into it.
func newTestStore(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS links_it CASCADE; CREATE SCHEMA links_it`); err != nil {
		t.Fatal(err)
	}
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, Schema: "links_it", MaxConns: 8, MinConns: 0, MaxConnLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Concurrent migrators (several replicas starting at once) must not conflict.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() { errs <- pg.Migrate(ctx, pool, Migrations(), log) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	return New(pool)
}

func TestPostgresStore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, links.Link{Slug: "first", URL: "https://a.io", Custom: true})
	if err != nil || created.CreatedAt.IsZero() {
		t.Fatalf("create: %+v %v", created, err)
	}
	if _, err := s.Create(ctx, links.Link{Slug: "first", URL: "https://b.io"}); !errors.Is(err, links.ErrSlugTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	got, err := s.Get(ctx, "first")
	if err != nil || got.URL != "https://a.io" || !got.Custom {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := s.Get(ctx, "nope"); !errors.Is(err, links.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	if _, err := s.Create(ctx, links.Link{Slug: "second", URL: "https://c.io"}); err != nil {
		t.Fatal(err)
	}
	recent, err := s.Recent(ctx, 10, time.Time{})
	if err != nil || len(recent) != 2 || recent[0].Slug != "second" {
		t.Fatalf("recent: %+v %v", recent, err)
	}
	older, err := s.Recent(ctx, 10, recent[0].CreatedAt)
	if err != nil || len(older) != 1 || older[0].Slug != "first" {
		t.Fatalf("keyset page: %+v %v", older, err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
