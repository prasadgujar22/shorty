//go:build integration

package cache

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRedisRoundTripAndBreaker(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	ctx := context.Background()
	r := New(Options{Addr: addr, PoolSize: 4, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, BreakerThreshold: 2, BreakerCooldown: time.Second}, prometheus.NewRegistry())
	defer func() { _ = r.Close() }()

	if err := r.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.Set(ctx, "it:k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := r.Get(ctx, "it:k"); err != nil || !ok || v != "v" {
		t.Fatalf("get: %q %v %v", v, ok, err)
	}
	if _, ok, err := r.Get(ctx, "it:missing"); err != nil || ok {
		t.Fatalf("missing: %v %v", ok, err)
	}

	// A dead endpoint trips the breaker after two failures.
	dead := New(Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, ReadTimeout: 50 * time.Millisecond, WriteTimeout: 50 * time.Millisecond, BreakerThreshold: 2, BreakerCooldown: time.Minute}, prometheus.NewRegistry())
	defer func() { _ = dead.Close() }()
	for range 2 {
		if _, _, err := dead.Get(ctx, "x"); err == nil {
			t.Fatal("expected error")
		}
	}
	start := time.Now()
	if _, _, err := dead.Get(ctx, "x"); !errors.Is(err, ErrOpen) || time.Since(start) > 5*time.Millisecond {
		t.Fatalf("breaker should fail fast with ErrOpen, got %v after %v", err, time.Since(start))
	}
}
