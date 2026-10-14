// Package store persists click aggregates in the analytics schema.
package store

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/aggregate"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the embedded migration files.
func Migrations() fs.FS {
	sub, _ := fs.Sub(migrations, "migrations")
	return sub
}

// Postgres is the analytics store.
type Postgres struct{ pool *pgxpool.Pool }

// New wraps a pool whose search_path points at the analytics schema.
func New(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

// Ping checks connectivity.
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// ApplyResult reports what a batch did.
type ApplyResult struct {
	Fresh      int // events counted for the first time
	Duplicates int // events already processed (redeliveries)
}

// Apply records a batch idempotently in one transaction: it claims each
// event ID in processed_events and only aggregates the IDs it newly claimed.
// If the transaction fails nothing is counted; the caller does not ack and
// JetStream redelivers.
func (p *Postgres) Apply(ctx context.Context, batch []events.Click) (ApplyResult, error) {
	ids := aggregate.UniqueIDs(batch)
	var res ApplyResult
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO processed_events (event_id)
			SELECT unnest($1::text[])
			ON CONFLICT (event_id) DO NOTHING
			RETURNING event_id`, ids)
		if err != nil {
			return err
		}
		claimed, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		fresh := make(map[string]bool, len(claimed))
		for _, id := range claimed {
			fresh[id] = true
		}
		res = ApplyResult{Fresh: len(claimed), Duplicates: len(batch) - len(claimed)}
		d := aggregate.Build(batch, fresh)
		if d.Empty() {
			return nil
		}
		return applyDeltas(ctx, tx, d)
	})
	return res, err
}

func applyDeltas(ctx context.Context, tx pgx.Tx, d aggregate.Deltas) error {
	hs, hh, hn := make([]string, len(d.Hourly)), make([]time.Time, len(d.Hourly)), make([]int64, len(d.Hourly))
	for i, c := range d.Hourly {
		hs[i], hh[i], hn[i] = c.Key.Slug, c.Key.Hour, c.N
	}
	rs, rr, rn := make([]string, len(d.Referrers)), make([]string, len(d.Referrers)), make([]int64, len(d.Referrers))
	for i, c := range d.Referrers {
		rs[i], rr[i], rn[i] = c.Key.Slug, c.Key.Referrer, c.N
	}
	ts, tn, tl := make([]string, len(d.Totals)), make([]int64, len(d.Totals)), make([]time.Time, len(d.Totals))
	for i, c := range d.Totals {
		ts[i], tn[i], tl[i] = c.Key, c.N, d.LastAt[c.Key]
	}

	// Inputs are sorted by key (see aggregate.Build) so concurrent replicas
	// acquire row locks in the same order. ORDER BY keeps it that way.
	b := &pgx.Batch{}
	b.Queue(`
		INSERT INTO clicks_hourly (slug, hour, clicks)
		SELECT * FROM unnest($1::text[], $2::timestamptz[], $3::bigint[]) ORDER BY 1, 2
		ON CONFLICT (slug, hour) DO UPDATE SET clicks = clicks_hourly.clicks + EXCLUDED.clicks`, hs, hh, hn)
	b.Queue(`
		INSERT INTO referrers (slug, referrer, clicks)
		SELECT * FROM unnest($1::text[], $2::text[], $3::bigint[]) ORDER BY 1, 2
		ON CONFLICT (slug, referrer) DO UPDATE SET clicks = referrers.clicks + EXCLUDED.clicks`, rs, rr, rn)
	b.Queue(`
		INSERT INTO link_totals (slug, clicks, last_click_at)
		SELECT * FROM unnest($1::text[], $2::bigint[], $3::timestamptz[]) ORDER BY 1
		ON CONFLICT (slug) DO UPDATE SET
		  clicks = link_totals.clicks + EXCLUDED.clicks,
		  last_click_at = GREATEST(link_totals.last_click_at, EXCLUDED.last_click_at)`, ts, tn, tl)
	return tx.SendBatch(ctx, b).Close()
}

// Point is one bucket in a time series.
type Point struct {
	T      time.Time `json:"t"`
	Clicks int64     `json:"clicks"`
}

// Referrer is a referrer and its click count.
type Referrer struct {
	Referrer string `json:"referrer"`
	Clicks   int64  `json:"clicks"`
}

// Stats is the GET /api/stats/{slug} payload.
type Stats struct {
	Slug        string     `json:"slug"`
	Total       int64      `json:"total"`
	LastClickAt *time.Time `json:"last_click_at"`
	Hourly      []Point    `json:"hourly"` // last 24h, oldest first, current hour last
	Daily       []Point    `json:"daily"`  // last 7 days (UTC), today last
	Referrers   []Referrer `json:"referrers"`
}

// Stats returns aggregates for slug; unknown slugs return zeroed series.
func (p *Postgres) Stats(ctx context.Context, slug string, topReferrers int) (Stats, error) {
	s := Stats{Slug: slug, Hourly: []Point{}, Daily: []Point{}, Referrers: []Referrer{}}
	b := &pgx.Batch{}
	b.Queue(`SELECT clicks, last_click_at FROM link_totals WHERE slug = $1`, slug).QueryRow(func(r pgx.Row) error {
		err := r.Scan(&s.Total, &s.LastClickAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	b.Queue(`
		SELECT h, COALESCE(c.clicks, 0)
		  FROM generate_series(date_trunc('hour', now()) - interval '23 hours', date_trunc('hour', now()), interval '1 hour') AS h
		  LEFT JOIN clicks_hourly c ON c.slug = $1 AND c.hour = h
		 ORDER BY h`, slug).Query(func(rows pgx.Rows) error {
		var err error
		s.Hourly, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Point])
		return err
	})
	b.Queue(`
		SELECT d, COALESCE(sum(c.clicks), 0)::bigint
		  FROM generate_series(date_trunc('day', now()) - interval '6 days', date_trunc('day', now()), interval '1 day') AS d
		  LEFT JOIN clicks_hourly c ON c.slug = $1 AND c.hour >= d AND c.hour < d + interval '1 day'
		 GROUP BY d ORDER BY d`, slug).Query(func(rows pgx.Rows) error {
		var err error
		s.Daily, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Point])
		return err
	})
	b.Queue(`SELECT referrer, clicks FROM referrers WHERE slug = $1 ORDER BY clicks DESC, referrer LIMIT $2`, slug, topReferrers).
		Query(func(rows pgx.Rows) error {
			var err error
			s.Referrers, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Referrer])
			return err
		})
	if err := p.pool.SendBatch(ctx, b).Close(); err != nil {
		return Stats{}, err
	}
	return s, nil
}

// PruneProcessed deletes idempotency rows older than retention in chunks.
// Only one replica prunes at a time (advisory try-lock); others skip.
func (p *Postgres) PruneProcessed(ctx context.Context, retention time.Duration) (int64, error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('shorty-analytics-prune'))`).Scan(&locked); err != nil || !locked {
		return 0, err
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtext('shorty-analytics-prune'))`)
	}()
	var total int64
	for {
		tag, err := conn.Exec(ctx, `
			DELETE FROM processed_events WHERE ctid IN (
			  SELECT ctid FROM processed_events WHERE processed_at < now() - make_interval(secs => $1) LIMIT 10000)`,
			retention.Seconds())
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < 10000 {
			return total, nil
		}
	}
}
