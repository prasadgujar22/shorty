// Package resolve maps a slug to its target URL using cache-aside:
// Redis first, links-svc on a miss, then populate Redis.
package resolve

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/singleflight"
)

var (
	// ErrNotFound means the slug does not exist (possibly from the negative cache).
	ErrNotFound = errors.New("not found")
	// ErrUnavailable means the cache missed and links-svc could not answer.
	ErrUnavailable = errors.New("lookup unavailable")
)

// negativeMarker is cached for slugs that do not exist, so scanners hammering
// random paths do not reach links-svc on every request.
const negativeMarker = "\x00"

// Cache is a string key/value cache (Redis in production).
type Cache interface {
	Get(ctx context.Context, key string) (val string, found bool, err error)
	Set(ctx context.Context, key, val string, ttl time.Duration) error
}

// Source is the system of record (links-svc).
type Source interface {
	// Lookup returns the URL for slug, ErrNotFound, or another error.
	Lookup(ctx context.Context, slug string) (string, error)
}

// Options tune caching.
type Options struct {
	TTL         time.Duration // positive entries (Redis)
	TTLJitter   float64       // ±fraction of TTL, spreads expiry to avoid stampedes
	NegativeTTL time.Duration // "does not exist" entries (Redis)

	L1Size        int           // in-process LRU entries; 0 disables the L1
	L1TTL         time.Duration // positive entries in the L1
	L1NegativeTTL time.Duration // "does not exist" entries in the L1
}

// Resolver implements cache-aside lookups with request coalescing.
type Resolver struct {
	cache  Cache
	source Source
	opts   Options
	group  singleflight.Group
	l1     *l1

	results *prometheus.CounterVec
	latency *prometheus.HistogramVec
}

// New creates a Resolver and registers its metrics.
func New(cache Cache, source Source, opts Options, reg prometheus.Registerer) *Resolver {
	r := &Resolver{cache: cache, source: source, opts: opts, l1: newL1(opts.L1Size),
		results: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "shorty_redirect_cache_requests_total",
			Help: "Cache lookups by result: l1_hit, l1_negative_hit, hit, negative_hit, miss, error.",
		}, []string{"result"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "shorty_redirect_lookup_seconds",
			Help:    "Lookup latency by source: cache or links.",
			Buckets: []float64{.0002, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5},
		}, []string{"source"}),
	}
	reg.MustRegister(r.results, r.latency, prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "shorty_redirect_l1_entries",
		Help: "Entries in the in-process L1 cache.",
	}, func() float64 { return float64(r.l1.len()) }))
	return r
}

func key(slug string) string { return "shorty:link:" + slug }

// Resolve returns the target URL for slug: L1 -> Redis -> links-svc.
func (r *Resolver) Resolve(ctx context.Context, slug string) (string, error) {
	if v, ok := r.l1.get(slug); ok {
		if v == negativeMarker {
			r.results.WithLabelValues("l1_negative_hit").Inc()
			return "", ErrNotFound
		}
		r.results.WithLabelValues("l1_hit").Inc()
		return v, nil
	}
	start := time.Now()
	val, found, err := r.cache.Get(ctx, key(slug))
	r.latency.WithLabelValues("cache").Observe(time.Since(start).Seconds())
	switch {
	case err != nil:
		// Redis down or breaker open: degrade to links-svc instead of failing.
		r.results.WithLabelValues("error").Inc()
	case found && val == negativeMarker:
		r.results.WithLabelValues("negative_hit").Inc()
		r.l1.set(slug, negativeMarker, r.opts.L1NegativeTTL)
		return "", ErrNotFound
	case found:
		r.results.WithLabelValues("hit").Inc()
		r.l1.set(slug, val, r.opts.L1TTL)
		return val, nil
	default:
		r.results.WithLabelValues("miss").Inc()
	}
	cacheOK := err == nil

	// Coalesce concurrent misses for the same slug into one links-svc call.
	v, err, _ := r.group.Do(slug, func() (any, error) {
		// Detach from the first caller's cancellation so one impatient
		// client cannot fail the request for everyone waiting on it.
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		start := time.Now()
		u, err := r.source.Lookup(lctx, slug)
		r.latency.WithLabelValues("links").Observe(time.Since(start).Seconds())
		switch {
		case errors.Is(err, ErrNotFound):
			r.l1.set(slug, negativeMarker, r.opts.L1NegativeTTL)
			if cacheOK {
				_ = r.cache.Set(lctx, key(slug), negativeMarker, r.opts.NegativeTTL)
			}
			return "", ErrNotFound
		case err != nil:
			return "", errors.Join(ErrUnavailable, err)
		}
		r.l1.set(slug, u, r.opts.L1TTL)
		if cacheOK {
			_ = r.cache.Set(lctx, key(slug), u, r.jitteredTTL())
		}
		return u, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (r *Resolver) jitteredTTL() time.Duration {
	if r.opts.TTLJitter <= 0 {
		return r.opts.TTL
	}
	f := 1 + r.opts.TTLJitter*(2*rand.Float64()-1) //nolint:gosec // jitter, not crypto
	return time.Duration(float64(r.opts.TTL) * f)
}
