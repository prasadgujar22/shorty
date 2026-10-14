package resolve

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type memCache struct {
	mu   sync.Mutex
	m    map[string]string
	ttls map[string]time.Duration
	err  error
}

func newCache() *memCache { return &memCache{m: map[string]string{}, ttls: map[string]time.Duration{}} }

func (c *memCache) Get(_ context.Context, k string) (string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return "", false, c.err
	}
	v, ok := c.m[k]
	return v, ok, nil
}

func (c *memCache) Set(_ context.Context, k, v string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.m[k], c.ttls[k] = v, ttl
	return nil
}

type fakeSource struct {
	calls atomic.Int32
	delay time.Duration
	urls  map[string]string
	err   error
}

type src struct{ *fakeSource }

func (s src) Lookup(_ context.Context, slug string) (string, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	if s.err != nil {
		return "", s.err
	}
	u, ok := s.urls[slug]
	if !ok {
		return "", ErrNotFound
	}
	return u, nil
}

func setup() (*Resolver, *memCache, *fakeSource) {
	c := newCache()
	s := &fakeSource{urls: map[string]string{"abc": "https://a.io"}}
	r := New(c, src{s}, Options{TTL: time.Hour, TTLJitter: 0.1, NegativeTTL: 30 * time.Second}, prometheus.NewRegistry())
	return r, c, s
}

func TestCacheAside(t *testing.T) {
	r, c, s := setup()
	ctx := context.Background()
	for range 3 {
		u, err := r.Resolve(ctx, "abc")
		if err != nil || u != "https://a.io" {
			t.Fatalf("resolve: %q %v", u, err)
		}
	}
	if n := s.calls.Load(); n != 1 {
		t.Fatalf("links-svc called %d times, want 1", n)
	}
	ttl := c.ttls[key("abc")]
	if ttl < 54*time.Minute || ttl > 66*time.Minute {
		t.Fatalf("ttl %v outside jitter band", ttl)
	}
	if got := testutil.ToFloat64(r.results.WithLabelValues("hit")); got != 2 {
		t.Fatalf("hits = %v", got)
	}
}

func TestNegativeCache(t *testing.T) {
	r, c, s := setup()
	for range 3 {
		if _, err := r.Resolve(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	}
	if s.calls.Load() != 1 || c.ttls[key("nope")] != 30*time.Second {
		t.Fatalf("calls=%d ttl=%v", s.calls.Load(), c.ttls[key("nope")])
	}
}

func TestCacheDownFallsBackToSource(t *testing.T) {
	r, c, s := setup()
	c.err = errors.New("redis: connection refused")
	u, err := r.Resolve(context.Background(), "abc")
	if err != nil || u != "https://a.io" || s.calls.Load() != 1 {
		t.Fatalf("degraded resolve: %q %v calls=%d", u, err, s.calls.Load())
	}
}

func TestSourceDown(t *testing.T) {
	r, _, s := setup()
	s.err = errors.New("dial tcp: refused")
	if _, err := r.Resolve(context.Background(), "abc"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestSingleflightCoalescesMisses(t *testing.T) {
	r, _, s := setup()
	s.delay = 50 * time.Millisecond
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if u, err := r.Resolve(context.Background(), "abc"); err != nil || u != "https://a.io" {
				t.Errorf("resolve: %q %v", u, err)
			}
		})
	}
	wg.Wait()
	if n := s.calls.Load(); n != 1 {
		t.Fatalf("links-svc called %d times for 50 concurrent misses", n)
	}
}

func TestL1ServesWhenRedisAndLinksAreDown(t *testing.T) {
	c := newCache()
	s := &fakeSource{urls: map[string]string{"abc": "https://a.io"}}
	r := New(c, src{s}, Options{TTL: time.Hour, NegativeTTL: time.Minute, L1Size: 10, L1TTL: time.Minute, L1NegativeTTL: time.Minute}, prometheus.NewRegistry())
	ctx := context.Background()
	if _, err := r.Resolve(ctx, "abc"); err != nil { // fills L1 + Redis
		t.Fatal(err)
	}
	if _, err := r.Resolve(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	c.err = errors.New("redis down")
	s.err = errors.New("links-svc down")
	if u, err := r.Resolve(ctx, "abc"); err != nil || u != "https://a.io" {
		t.Fatalf("L1 should serve during outage: %q %v", u, err)
	}
	if _, err := r.Resolve(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("negative L1 entry: %v", err)
	}
	if got := testutil.ToFloat64(r.results.WithLabelValues("l1_hit")); got != 1 {
		t.Fatalf("l1_hit = %v", got)
	}
}

func TestL1EvictionAndTTL(t *testing.T) {
	now := time.Unix(0, 0)
	c := newL1(2)
	c.now = func() time.Time { return now }
	c.set("a", "1", time.Second)
	c.set("b", "2", time.Second)
	c.get("a") // a is now most recent
	c.set("c", "3", time.Second)
	if _, ok := c.get("b"); ok {
		t.Fatal("b should have been evicted (LRU)")
	}
	if v, ok := c.get("a"); !ok || v != "1" {
		t.Fatal("a should remain")
	}
	now = now.Add(2 * time.Second)
	if _, ok := c.get("a"); ok || c.len() != 1 {
		t.Fatalf("a should have expired; len=%d", c.len())
	}
	var disabled *l1
	disabled.set("x", "y", time.Second)
	if _, ok := disabled.get("x"); ok {
		t.Fatal("nil L1 must be a no-op")
	}
}
