// Package store is the Postgres implementation of links.Store.
package store

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prasadgujar22/shorty/services/links-svc/internal/links"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the embedded migration files.
func Migrations() fs.FS {
	sub, _ := fs.Sub(migrations, "migrations")
	return sub
}

// Postgres implements links.Store.
type Postgres struct{ pool *pgxpool.Pool }

// New wraps a pool whose search_path points at the links schema.
func New(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

const uniqueViolation = "23505"

func (p *Postgres) Create(ctx context.Context, l links.Link) (links.Link, error) {
	err := p.pool.QueryRow(ctx,
		`INSERT INTO links (slug, url, custom) VALUES ($1, $2, $3) RETURNING created_at`,
		l.Slug, l.URL, l.Custom).Scan(&l.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return links.Link{}, links.ErrSlugTaken
	}
	return l, err
}

func (p *Postgres) Get(ctx context.Context, slug string) (links.Link, error) {
	var l links.Link
	err := p.pool.QueryRow(ctx,
		`SELECT slug, url, custom, created_at FROM links WHERE slug = $1`, slug).
		Scan(&l.Slug, &l.URL, &l.Custom, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return links.Link{}, links.ErrNotFound
	}
	return l, err
}

func (p *Postgres) Recent(ctx context.Context, limit int, before time.Time) ([]links.Link, error) {
	if before.IsZero() {
		before = time.Now().Add(time.Hour) // tolerate small clock skew between app and DB
	}
	rows, err := p.pool.Query(ctx,
		`SELECT slug, url, custom, created_at FROM links
		  WHERE created_at < $1 ORDER BY created_at DESC, slug LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (links.Link, error) {
		var l links.Link
		err := r.Scan(&l.Slug, &l.URL, &l.Custom, &l.CreatedAt)
		return l, err
	})
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
