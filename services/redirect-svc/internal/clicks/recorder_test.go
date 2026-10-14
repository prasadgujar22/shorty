package clicks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/prasadgujar22/shorty/pkg/events"
)

type fakePub struct {
	mu       sync.Mutex
	got      map[string]int
	failures atomic.Int32 // fail this many publishes first
	block    chan struct{}
}

func (f *fakePub) Publish(ctx context.Context, c events.Click) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.failures.Add(-1) >= 0 {
		return errors.New("nats: no responders")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got[c.ID]++
	return nil
}

func newRec(p Publisher, buf int) *Recorder {
	return New(p, Options{BufferSize: buf, Workers: 4, PublishTimeout: 100 * time.Millisecond, MaxBackoff: 10 * time.Millisecond},
		slog.New(slog.NewTextHandler(io.Discard, nil)), prometheus.NewRegistry())
}

func click() events.Click {
	return events.Click{ID: events.NewID(), Slug: "abc", At: time.Now(), Referrer: "direct"}
}

func TestRetriesUntilPublishedAndFlushesOnStop(t *testing.T) {
	p := &fakePub{got: map[string]int{}}
	p.failures.Store(25) // a NATS outage spanning many attempts
	r := newRec(p, 1000)
	for range 200 {
		if !r.Record(click()) {
			t.Fatal("unexpected drop")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if len(p.got) != 200 {
		t.Fatalf("published %d of 200", len(p.got))
	}
	if testutil.ToFloat64(r.dropped) != 0 || testutil.ToFloat64(r.retries) < 25 {
		t.Fatalf("dropped=%v retries=%v", testutil.ToFloat64(r.dropped), testutil.ToFloat64(r.retries))
	}
	if r.Record(click()) {
		t.Fatal("Record after Stop must be rejected")
	}
}

func TestDropsWhenBufferFull(t *testing.T) {
	p := &fakePub{got: map[string]int{}, block: make(chan struct{})}
	r := newRec(p, 10)
	accepted := 0
	for range 100 {
		if r.Record(click()) {
			accepted++
		}
	}
	// 4 workers each hold one event, plus 10 buffered.
	if accepted > 14 || testutil.ToFloat64(r.dropped) < 86 {
		t.Fatalf("accepted=%d dropped=%v", accepted, testutil.ToFloat64(r.dropped))
	}
	close(p.block)
	_ = r.Stop(context.Background())
}

func TestStopDeadlineCountsLostEvents(t *testing.T) {
	p := &fakePub{got: map[string]int{}}
	p.failures.Store(1 << 30) // NATS never comes back
	r := newRec(p, 100)
	for range 50 {
		r.Record(click())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := r.Stop(ctx); err == nil {
		t.Fatal("expected deadline error")
	}
	if got := testutil.ToFloat64(r.dropped); got != 50 {
		t.Fatalf("dropped = %v, want 50", got)
	}
}
