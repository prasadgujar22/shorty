// Package clicks records click events off the request path.
//
// The redirect handler calls Record, which never blocks: the event goes into
// a bounded in-memory buffer. Publisher workers drain the buffer into
// JetStream and retry each event (with backoff) until the server acks it, so
// a NATS restart or leader election delays clicks instead of losing them.
// Every event carries a unique ID used as Nats-Msg-Id, so a retry after a
// lost ack is de-duplicated by JetStream, and again by analytics-svc.
//
// Clicks can still be lost when (a) the buffer is full (counted in
// shorty_clicks_dropped_total) or (b) the pod is SIGKILLed with events in the
// buffer. On SIGTERM, Stop flushes the buffer before the process exits.
package clicks

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/pkg/events"
)

// Publisher sends one event and returns once the broker has persisted it.
type Publisher interface {
	Publish(ctx context.Context, c events.Click) error
}

// Options tune the recorder.
type Options struct {
	BufferSize     int
	Workers        int
	PublishTimeout time.Duration
	MaxBackoff     time.Duration
}

// Recorder buffers and publishes clicks.
type Recorder struct {
	pub  Publisher
	opts Options
	log  *slog.Logger

	mu      sync.RWMutex
	closed  bool
	ch      chan events.Click
	wg      sync.WaitGroup
	abort   context.Context
	abortFn context.CancelFunc

	enqueued, published, dropped, retries prometheus.Counter
	pubLatency                            prometheus.Histogram
}

// New creates a recorder, registers metrics and starts the workers.
func New(pub Publisher, opts Options, log *slog.Logger, reg prometheus.Registerer) *Recorder {
	opts.BufferSize = max(1, opts.BufferSize)
	opts.Workers = max(1, opts.Workers)
	if opts.PublishTimeout <= 0 {
		opts.PublishTimeout = 2 * time.Second
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 2 * time.Second
	}
	r := &Recorder{pub: pub, opts: opts, log: log, ch: make(chan events.Click, opts.BufferSize)}
	r.abort, r.abortFn = context.WithCancel(context.Background())
	c := func(name, help string) prometheus.Counter {
		return prometheus.NewCounter(prometheus.CounterOpts{Name: "shorty_clicks_" + name + "_total", Help: help})
	}
	r.enqueued = c("enqueued", "Click events accepted into the buffer.")
	r.published = c("published", "Click events acknowledged by JetStream.")
	r.dropped = c("dropped", "Click events dropped because the buffer was full or the process stopped.")
	r.retries = c("publish_retries", "Failed publish attempts that were retried.")
	r.pubLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "shorty_clicks_publish_seconds",
		Help:    "Latency of successful JetStream publishes (time to ack).",
		Buckets: []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
	})
	reg.MustRegister(r.enqueued, r.published, r.dropped, r.retries, r.pubLatency,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "shorty_clicks_buffer_depth", Help: "Click events waiting to be published.",
		}, func() float64 { return float64(len(r.ch)) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "shorty_clicks_buffer_capacity", Help: "Click buffer capacity.",
		}, func() float64 { return float64(cap(r.ch)) }),
	)
	for range opts.Workers {
		r.wg.Go(r.worker)
	}
	return r
}

// Record enqueues c without blocking. It returns false if c was dropped.
func (r *Recorder) Record(c events.Click) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		r.dropped.Inc()
		return false
	}
	select {
	case r.ch <- c:
		r.enqueued.Inc()
		return true
	default:
		r.dropped.Inc()
		return false
	}
}

func (r *Recorder) worker() {
	for c := range r.ch {
		r.publish(c)
	}
}

// publish retries until success or until Stop's deadline aborts the flush.
func (r *Recorder) publish(c events.Click) {
	backoff := 25 * time.Millisecond
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(r.abort, r.opts.PublishTimeout)
		start := time.Now()
		err := r.pub.Publish(ctx, c)
		cancel()
		if err == nil {
			r.published.Inc()
			r.pubLatency.Observe(time.Since(start).Seconds())
			return
		}
		if r.abort.Err() != nil {
			r.dropped.Inc()
			return
		}
		r.retries.Inc()
		if attempt == 1 || attempt%20 == 0 {
			r.log.Warn("click publish failed, retrying", "attempt", attempt, "err", err, "buffered", len(r.ch))
		}
		select {
		case <-r.abort.Done():
			r.dropped.Inc()
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, r.opts.MaxBackoff)
	}
}

// Stop stops accepting events and flushes the buffer until ctx expires.
// Events still unpublished at the deadline are counted as dropped.
func (r *Recorder) Stop(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.ch)
	}
	r.mu.Unlock()
	pending := len(r.ch)
	r.log.Info("flushing click buffer", "pending", pending)

	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		r.log.Info("click buffer flushed", "pending_at_stop", pending)
		return nil
	case <-ctx.Done():
		lost := len(r.ch)
		r.abortFn()
		for range r.ch {
			r.dropped.Inc()
		}
		<-done
		return errors.Join(errors.New("click flush deadline exceeded"), ctx.Err(), errLost(lost))
	}
}

type errLost int

func (e errLost) Error() string {
	return "approx. " + strconv.Itoa(int(e)) + " buffered clicks dropped"
}
