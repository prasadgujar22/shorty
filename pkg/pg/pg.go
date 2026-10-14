// Package pg builds a pgx connection pool from 12-factor config, pins it to a
// single schema, applies embedded SQL migrations and exports pool metrics.
package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/pkg/config"
)

// Config is the pool configuration. DSN may be empty, in which case libpq
// environment variables (PGHOST, PGUSER, PGPASSWORD, PGDATABASE, ...) apply.
type Config struct {
	DSN               string
	Schema            string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	StatementTimeout  time.Duration
}

// ConfigFromEnv reads DATABASE_URL, DB_SCHEMA and the DB_* pool knobs.
func ConfigFromEnv(env *config.Env, defaultSchema string) Config {
	return Config{
		DSN:               env.String("DATABASE_URL", ""),
		Schema:            env.String("DB_SCHEMA", defaultSchema),
		MaxConns:          clampInt32(env.Int("DB_MAX_CONNS", 10)),
		MinConns:          clampInt32(env.Int("DB_MIN_CONNS", 2)),
		MaxConnLifetime:   env.Duration("DB_MAX_CONN_LIFETIME", 30*time.Minute),
		MaxConnIdleTime:   env.Duration("DB_MAX_CONN_IDLE_TIME", 5*time.Minute),
		HealthCheckPeriod: env.Duration("DB_HEALTH_CHECK_PERIOD", 15*time.Second),
		StatementTimeout:  env.Duration("DB_STATEMENT_TIMEOUT", 5*time.Second),
	}
}

func clampInt32(n int) int32 {
	const maxPool = 1 << 16
	return int32(max(0, min(n, maxPool))) //nolint:gosec // clamped above
}

// PoolConfig converts Config into a pgxpool.Config.
func (c Config) PoolConfig() (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(c.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	// Zero values keep pgx defaults (a zero health-check period would panic).
	if c.MaxConns > 0 {
		pc.MaxConns = c.MaxConns
	}
	if c.MinConns > 0 {
		pc.MinConns = min(c.MinConns, pc.MaxConns)
	}
	if c.MaxConnLifetime > 0 {
		pc.MaxConnLifetime = c.MaxConnLifetime
		pc.MaxConnLifetimeJitter = c.MaxConnLifetime / 10
	}
	if c.MaxConnIdleTime > 0 {
		pc.MaxConnIdleTime = c.MaxConnIdleTime
	}
	if c.HealthCheckPeriod > 0 {
		pc.HealthCheckPeriod = c.HealthCheckPeriod
	}
	rp := pc.ConnConfig.RuntimeParams
	if c.Schema != "" {
		// Each service only ever sees its own schema; table names stay unqualified.
		rp["search_path"] = c.Schema
	}
	if c.StatementTimeout > 0 {
		rp["statement_timeout"] = fmt.Sprint(c.StatementTimeout.Milliseconds())
	}
	if rp["application_name"] == "" {
		rp["application_name"] = "shorty"
	}
	return pc, nil
}

// Connect creates the pool and verifies connectivity with a ping.
func Connect(ctx context.Context, c Config) (*pgxpool.Pool, error) {
	pc, err := c.PoolConfig()
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// poolCollector exports pgxpool.Stat as Prometheus metrics.
type poolCollector struct {
	pool                                     *pgxpool.Pool
	total, idle, acquired, max, acquireCount *prometheus.Desc
	acquireDuration, emptyAcquire, canceled  *prometheus.Desc
}

// RegisterPoolMetrics exports shorty_db_pool_* metrics for pool.
func RegisterPoolMetrics(reg prometheus.Registerer, pool *pgxpool.Pool) {
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("shorty_db_pool_"+name, help, nil, nil)
	}
	reg.MustRegister(&poolCollector{
		pool:            pool,
		total:           d("connections", "Open connections."),
		idle:            d("idle_connections", "Idle connections."),
		acquired:        d("acquired_connections", "Connections currently checked out."),
		max:             d("max_connections", "Configured maximum pool size."),
		acquireCount:    d("acquires_total", "Successful connection acquires."),
		acquireDuration: d("acquire_seconds_total", "Total time spent waiting to acquire a connection."),
		emptyAcquire:    d("empty_acquires_total", "Acquires that had to wait because the pool was empty."),
		canceled:        d("canceled_acquires_total", "Acquires cancelled by the caller's context."),
	})
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.total, c.idle, c.acquired, c.max, c.acquireCount, c.acquireDuration, c.emptyAcquire, c.canceled} {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	g := func(d *prometheus.Desc, v float64) { ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v) }
	k := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}
	g(c.total, float64(s.TotalConns()))
	g(c.idle, float64(s.IdleConns()))
	g(c.acquired, float64(s.AcquiredConns()))
	g(c.max, float64(s.MaxConns()))
	k(c.acquireCount, float64(s.AcquireCount()))
	k(c.acquireDuration, s.AcquireDuration().Seconds())
	k(c.emptyAcquire, float64(s.EmptyAcquireCount()))
	k(c.canceled, float64(s.CanceledAcquireCount()))
}
