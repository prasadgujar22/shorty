//go:build integration

package store

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/pkg/pg"
)

// newTestStore recreates schema `schema` and migrates it.
func newTestStore(t *testing.T, schema string) *Postgres {
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
	defer admin.Close()
	if _, err := admin.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %[1]s CASCADE; CREATE SCHEMA %[1]s`, schema)); err != nil {
		t.Fatal(err)
	}
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, Schema: schema, MaxConns: 16})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pg.Migrate(ctx, pool, Migrations(), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return New(pool)
}

func clicks(n int, slug string, at time.Time, ref string) []events.Click {
	out := make([]events.Click, n)
	for i := range out {
		out[i] = events.Click{ID: events.NewID(), Slug: slug, At: at, Referrer: ref}
	}
	return out
}

func TestApplyIsIdempotent(t *testing.T) {
	s := newTestStore(t, "analytics_it")
	ctx := context.Background()
	now := time.Now().UTC()

	batch := append(clicks(30, "hot", now, "x.com"), clicks(10, "hot", now.Add(-26*time.Hour), "direct")...)
	res, err := s.Apply(ctx, batch)
	if err != nil || res.Fresh != 40 || res.Duplicates != 0 {
		t.Fatalf("first apply: %+v %v", res, err)
	}
	// Redelivery of the same batch, plus a few new events.
	res, err = s.Apply(ctx, append(batch, clicks(5, "hot", now, "y.com")...))
	if err != nil || res.Fresh != 5 || res.Duplicates != 40 {
		t.Fatalf("second apply: %+v %v", res, err)
	}

	st, err := s.Stats(ctx, "hot", 10)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 45 || len(st.Hourly) != 24 || len(st.Daily) != 7 {
		t.Fatalf("stats: total=%d hourly=%d daily=%d", st.Total, len(st.Hourly), len(st.Daily))
	}
	var last24 int64
	for _, p := range st.Hourly {
		last24 += p.Clicks
	}
	if last24 != 35 || st.Hourly[23].Clicks != 35 {
		t.Fatalf("last 24h = %d (current hour %d), want 35", last24, st.Hourly[23].Clicks)
	}
	var week int64
	for _, p := range st.Daily {
		week += p.Clicks
	}
	if week != 45 {
		t.Fatalf("7d = %d, want 45", week)
	}
	if st.Referrers[0] != (Referrer{"x.com", 30}) || len(st.Referrers) != 3 {
		t.Fatalf("referrers = %+v", st.Referrers)
	}
	if st.LastClickAt == nil || st.LastClickAt.Sub(now).Abs() > time.Millisecond {
		t.Fatalf("last click = %v", st.LastClickAt)
	}

	empty, err := s.Stats(ctx, "never", 10)
	if err != nil || empty.Total != 0 || len(empty.Hourly) != 24 || empty.LastClickAt != nil {
		t.Fatalf("unknown slug: %+v %v", empty, err)
	}
}

func TestConcurrentOverlappingBatchesStayExact(t *testing.T) {
	s := newTestStore(t, "analytics_it_conc")
	ctx := context.Background()
	now := time.Now().UTC()

	// 4 replicas apply overlapping batches touching the same hot rows in
	// different input orders; every event must be counted exactly once.
	all := append(clicks(400, "a", now, "r1"), clicks(400, "b", now, "r2")...)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for w := range 4 {
		wg.Go(func() {
			for i := 0; i < len(all); i += 50 {
				lo := (i + w*100) % len(all)
				hi := min(lo+100, len(all))
				part := append([]events.Click{}, all[lo:hi]...)
				if w%2 == 1 {
					for l, r := 0, len(part)-1; l < r; l, r = l+1, r-1 {
						part[l], part[r] = part[r], part[l]
					}
				}
				if _, err := s.Apply(ctx, part); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("apply: %v", err)
	}
	// Anything not covered above gets applied once more (also idempotent).
	if _, err := s.Apply(ctx, all); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"a", "b"} {
		st, err := s.Stats(ctx, slug, 10)
		if err != nil || st.Total != 400 {
			t.Fatalf("%s total = %d %v", slug, st.Total, err)
		}
	}
}

func TestPrune(t *testing.T) {
	s := newTestStore(t, "analytics_it_prune")
	ctx := context.Background()
	if _, err := s.Apply(ctx, clicks(3, "p", time.Now(), "")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE processed_events SET processed_at = now() - interval '10 days'`); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneProcessed(ctx, 8*24*time.Hour)
	if err != nil || n != 3 {
		t.Fatalf("pruned %d %v", n, err)
	}
}
