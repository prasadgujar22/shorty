// Package consumer pulls click events from JetStream in batches and applies
// them to the store. Delivery is at-least-once; the store makes it
// effectively-once by de-duplicating on event ID.
package consumer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/store"
)

// Applier persists a batch idempotently.
type Applier interface {
	Apply(ctx context.Context, batch []events.Click) (store.ApplyResult, error)
}

// Options tune the consumer.
type Options struct {
	Stream        string
	Subject       string
	Durable       string
	BatchSize     int
	FetchMaxWait  time.Duration
	AckWait       time.Duration
	MaxAckPending int
}

// Consumer runs the fetch/apply/ack loop.
type Consumer struct {
	js    jetstream.JetStream
	store Applier
	opts  Options
	log   *slog.Logger

	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	cons   jetstream.Consumer

	eventsTotal          *prometheus.CounterVec
	batchSeconds         prometheus.Histogram
	batchSize            prometheus.Histogram
	pending, ackPending  prometheus.Gauge
	freshness            prometheus.Gauge
	applyErrors, redeliv prometheus.Counter
}

// New creates a consumer and registers its metrics.
func New(js jetstream.JetStream, st Applier, opts Options, log *slog.Logger, reg prometheus.Registerer) *Consumer {
	c := &Consumer{js: js, store: st, opts: opts, log: log, done: make(chan struct{}),
		eventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "shorty_analytics_events_total",
			Help: "Click events handled, by result: processed, duplicate, invalid.",
		}, []string{"result"}),
		batchSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "shorty_analytics_batch_seconds", Help: "Time to apply and ack one batch.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		}),
		batchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "shorty_analytics_batch_size", Help: "Events per fetched batch.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		}),
		pending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "shorty_analytics_consumer_pending", Help: "Messages in the stream not yet delivered to the consumer (lag).",
		}),
		ackPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "shorty_analytics_consumer_ack_pending", Help: "Messages delivered but not yet acknowledged.",
		}),
		freshness: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "shorty_analytics_last_event_age_seconds", Help: "Age of the newest event in the last applied batch (end-to-end delay).",
		}),
		applyErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "shorty_analytics_apply_errors_total", Help: "Batches that failed to apply and will be redelivered.",
		}),
		redeliv: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "shorty_analytics_redelivered_total", Help: "Messages received with delivery count > 1.",
		}),
	}
	reg.MustRegister(c.eventsTotal, c.batchSeconds, c.batchSize, c.pending, c.ackPending, c.freshness, c.applyErrors, c.redeliv)
	return c
}

// ConsumerConfig is the durable pull consumer definition.
func (c *Consumer) ConsumerConfig() jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{
		Durable:       c.opts.Durable,
		Description:   "analytics-svc aggregation",
		FilterSubject: c.opts.Subject,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       c.opts.AckWait,
		MaxAckPending: c.opts.MaxAckPending,
		MaxDeliver:    -1, // DB outages must not dead-letter good events; poison messages are Term()ed.
		// Replicas inherit the stream's replica count.
	}
}

// Start ensures the stream and consumer exist (retrying until NATS is
// reachable) and runs the loop in the background.
func (c *Consumer) Start(ctx context.Context, stream events.StreamConfig) {
	ctx, c.cancel = context.WithCancel(ctx)
	go func() {
		defer close(c.done)
		for ctx.Err() == nil {
			cons, err := c.ensure(ctx, stream)
			if err != nil {
				c.log.Warn("jetstream consumer not ready, retrying", "err", err)
				sleep(ctx, 2*time.Second)
				continue
			}
			c.mu.Lock()
			c.cons = cons
			c.mu.Unlock()
			c.log.Info("consuming", "stream", c.opts.Stream, "durable", c.opts.Durable, "batch", c.opts.BatchSize)
			c.loop(ctx, cons)
		}
	}()
	go c.pollLag(ctx)
}

func (c *Consumer) ensure(ctx context.Context, stream events.StreamConfig) (jetstream.Consumer, error) {
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s, err := events.EnsureStream(sctx, c.js, stream)
	if err != nil {
		return nil, err
	}
	return s.CreateOrUpdateConsumer(sctx, c.ConsumerConfig())
}

func (c *Consumer) loop(ctx context.Context, cons jetstream.Consumer) {
	for ctx.Err() == nil {
		batch, err := cons.Fetch(c.opts.BatchSize, jetstream.FetchMaxWait(c.opts.FetchMaxWait))
		if err != nil {
			c.log.Warn("fetch failed", "err", err)
			sleep(ctx, time.Second)
			if errors.Is(err, jetstream.ErrConsumerNotFound) || errors.Is(err, jetstream.ErrStreamNotFound) {
				return // re-create
			}
			continue
		}
		var msgs []jetstream.Msg
		for m := range batch.Messages() {
			msgs = append(msgs, m)
		}
		if err := batch.Error(); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, jetstream.ErrNoMessages) {
			c.log.Debug("fetch ended with error", "err", err, "received", len(msgs))
		}
		if len(msgs) > 0 {
			// Finish the batch even if shutdown begins mid-way.
			c.handle(context.WithoutCancel(ctx), msgs)
		}
	}
}

// handle applies one batch. Ack only after the DB commit; on failure, Nak
// with a delay so JetStream redelivers.
func (c *Consumer) handle(ctx context.Context, msgs []jetstream.Msg) {
	start := time.Now()
	c.batchSize.Observe(float64(len(msgs)))
	clicks := make([]events.Click, 0, len(msgs))
	valid := make([]jetstream.Msg, 0, len(msgs))
	var newest time.Time
	for _, m := range msgs {
		if md, err := m.Metadata(); err == nil && md.NumDelivered > 1 {
			c.redeliv.Inc()
		}
		ev, err := events.Decode(m.Data())
		if err != nil {
			c.log.Error("poison message terminated", "err", err, "subject", m.Subject())
			c.eventsTotal.WithLabelValues("invalid").Inc()
			_ = m.Term()
			continue
		}
		clicks = append(clicks, ev)
		valid = append(valid, m)
		if ev.At.After(newest) {
			newest = ev.At
		}
	}
	if len(clicks) == 0 {
		return
	}
	actx, cancel := context.WithTimeout(ctx, c.opts.AckWait/2)
	res, err := c.store.Apply(actx, clicks)
	cancel()
	if err != nil {
		c.applyErrors.Inc()
		c.log.Error("apply batch failed; will be redelivered", "err", err, "events", len(clicks))
		for _, m := range valid {
			_ = m.NakWithDelay(2 * time.Second)
		}
		return
	}
	for _, m := range valid {
		// Fire-and-forget ack: if it is lost, the redelivery is de-duplicated.
		_ = m.Ack()
	}
	c.eventsTotal.WithLabelValues("processed").Add(float64(res.Fresh))
	c.eventsTotal.WithLabelValues("duplicate").Add(float64(res.Duplicates))
	c.freshness.Set(time.Since(newest).Seconds())
	c.batchSeconds.Observe(time.Since(start).Seconds())
}

func (c *Consumer) pollLag(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		c.mu.Lock()
		cons := c.cons
		c.mu.Unlock()
		if cons == nil {
			continue
		}
		ictx, cancel := context.WithTimeout(ctx, 2*time.Second)
		info, err := cons.Info(ictx)
		cancel()
		if err != nil {
			continue
		}
		c.pending.Set(float64(info.NumPending))
		c.ackPending.Set(float64(info.NumAckPending))
	}
}

// Stop ends the loop after the in-flight batch completes.
func (c *Consumer) Stop(ctx context.Context) error {
	if c.cancel == nil {
		return nil
	}
	c.cancel()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
