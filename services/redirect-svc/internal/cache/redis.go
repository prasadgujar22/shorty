// Package cache is the Redis implementation of resolve.Cache, guarded by a
// small circuit breaker so a dead Redis costs one fast error instead of a
// timeout on every request.
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/prasadgujar22/shorty/pkg/config"
)

// ErrOpen is returned while the breaker is open.
var ErrOpen = errors.New("redis circuit open")

// Options configure the Redis client and breaker.
type Options struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	MinIdle      int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	BreakerThreshold int           // consecutive failures that open the breaker
	BreakerCooldown  time.Duration // how long it stays open before a trial request
}

// OptionsFromEnv reads REDIS_* variables.
func OptionsFromEnv(env *config.Env) Options {
	return Options{
		Addr:             env.String("REDIS_ADDR", "localhost:6379"),
		Password:         env.String("REDIS_PASSWORD", ""),
		DB:               env.Int("REDIS_DB", 0),
		PoolSize:         env.Int("REDIS_POOL_SIZE", 32),
		MinIdle:          env.Int("REDIS_MIN_IDLE", 4),
		DialTimeout:      env.Duration("REDIS_DIAL_TIMEOUT", 250*time.Millisecond),
		ReadTimeout:      env.Duration("REDIS_READ_TIMEOUT", 100*time.Millisecond),
		WriteTimeout:     env.Duration("REDIS_WRITE_TIMEOUT", 100*time.Millisecond),
		BreakerThreshold: env.Int("REDIS_BREAKER_THRESHOLD", 5),
		BreakerCooldown:  env.Duration("REDIS_BREAKER_COOLDOWN", 2*time.Second),
	}
}

// Redis wraps a go-redis client with a breaker.
type Redis struct {
	client *redis.Client
	br     *Breaker
}

// New creates the client (lazy connect) and registers breaker metrics.
func New(o Options, reg prometheus.Registerer) *Redis {
	c := redis.NewClient(&redis.Options{
		Addr:         o.Addr,
		Password:     o.Password,
		DB:           o.DB,
		PoolSize:     o.PoolSize,
		MinIdleConns: o.MinIdle,
		DialTimeout:  o.DialTimeout,
		ReadTimeout:  o.ReadTimeout,
		WriteTimeout: o.WriteTimeout,
		PoolTimeout:  o.ReadTimeout * 2,
		// A cache must fail fast; the resolver falls back to links-svc.
		MaxRetries: 1,
	})
	br := NewBreaker(o.BreakerThreshold, o.BreakerCooldown)
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "shorty_redis_breaker_open",
		Help: "1 while the Redis circuit breaker is open.",
	}, func() float64 {
		if br.Open() {
			return 1
		}
		return 0
	}))
	return &Redis{client: c, br: br}
}

func (r *Redis) Get(ctx context.Context, key string) (string, bool, error) {
	if !r.br.Allow() {
		return "", false, ErrOpen
	}
	v, err := r.client.Get(ctx, key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		r.br.Success()
		return "", false, nil
	case err != nil:
		r.br.Failure()
		return "", false, err
	}
	r.br.Success()
	return v, true, nil
}

func (r *Redis) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	if !r.br.Allow() {
		return ErrOpen
	}
	if err := r.client.Set(ctx, key, val, ttl).Err(); err != nil {
		r.br.Failure()
		return err
	}
	r.br.Success()
	return nil
}

// Ping is a (soft) readiness check.
func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Close closes the client.
func (r *Redis) Close() error { return r.client.Close() }

// Breaker is a minimal consecutive-failure circuit breaker:
// closed -> (threshold failures) -> open -> (cooldown) -> half-open: one
// trial request; success closes it, failure re-opens it.
type Breaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	failures  int
	openUntil time.Time
	trial     bool
	now       func() time.Time
}

// NewBreaker returns a closed breaker.
func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	return &Breaker{threshold: max(1, threshold), cooldown: cooldown, now: time.Now}
}

// Allow reports whether a request may proceed.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < b.threshold {
		return true
	}
	if b.now().Before(b.openUntil) || b.trial {
		return false
	}
	b.trial = true // half-open: let exactly one request through
	return true
}

// Success records a successful call.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures, b.trial = 0, false
}

// Failure records a failed call.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.trial = false
	if b.failures >= b.threshold {
		b.openUntil = b.now().Add(b.cooldown)
	}
}

// Open reports whether the breaker is currently rejecting requests.
func (b *Breaker) Open() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failures >= b.threshold && b.now().Before(b.openUntil)
}
