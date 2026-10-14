package cache

import (
	"testing"
	"time"
)

func TestBreakerLifecycle(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(3, time.Second)
	b.now = func() time.Time { return now }

	for range 2 {
		b.Failure()
	}
	if !b.Allow() || b.Open() {
		t.Fatal("should still be closed below threshold")
	}
	b.Failure()
	if b.Allow() || !b.Open() {
		t.Fatal("should be open at threshold")
	}

	now = now.Add(1100 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("half-open should allow one trial")
	}
	if b.Allow() {
		t.Fatal("half-open must allow only one trial at a time")
	}
	b.Failure()
	if b.Allow() || !b.Open() {
		t.Fatal("failed trial should re-open")
	}

	now = now.Add(1100 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("second trial")
	}
	b.Success()
	first, second := b.Allow(), b.Allow()
	if !first || !second || b.Open() {
		t.Fatal("success should close the breaker")
	}
}
