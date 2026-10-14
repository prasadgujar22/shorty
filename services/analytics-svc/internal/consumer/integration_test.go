//go:build integration

package consumer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/pkg/pg"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/store"
)

// lostAck applies the batch for real, then reports failure the first few
// times: exactly what a crash between COMMIT and ack looks like.
type lostAck struct {
	inner store.Postgres
	fail  atomic.Int32
}

func (l *lostAck) Apply(ctx context.Context, b []events.Click) (store.ApplyResult, error) {
	res, err := l.inner.Apply(ctx, b)
	if err == nil && l.fail.Add(-1) >= 0 {
		return res, errors.New("simulated crash after commit")
	}
	return res, err
}

func newStore(t *testing.T) *store.Postgres {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS analytics_it_cons CASCADE; CREATE SCHEMA analytics_it_cons`); err != nil {
		t.Fatal(err)
	}
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, Schema: "analytics_it_cons", MaxConns: 8})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pg.Migrate(ctx, pool, store.Migrations(), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	return store.New(pool)
}

func TestConsumerAtLeastOnceIsExactlyCounted(t *testing.T) {
	url := os.Getenv("TEST_NATS_URL")
	if url == "" {
		t.Skip("TEST_NATS_URL not set")
	}
	st := newStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	nc, err := events.Connect(url, "analytics-it", log)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, _ := jetstream.New(nc)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sc := events.StreamConfig{Name: "CLICKS_IT_ANALYTICS", Subject: "it.analytics.clicks", Replicas: 1, MaxAge: time.Hour, MaxBytes: 64 << 20, DuplicateWindow: time.Minute}
	_ = js.DeleteStream(ctx, sc.Name)
	defer func() { _ = js.DeleteStream(context.Background(), sc.Name) }()
	if _, err := events.EnsureStream(ctx, js, sc); err != nil {
		t.Fatal(err)
	}

	// 300 unique events; 50 of them are published twice with different
	// Nats-Msg-Ids (i.e. JetStream can't de-dup them; analytics must).
	const unique = 300
	var sent []events.Click
	for i := range unique {
		c := events.Click{ID: events.NewID(), Slug: "e2e", At: time.Now().UTC(), Referrer: "direct"}
		sent = append(sent, c)
		if i < 50 {
			sent = append(sent, c)
		}
	}
	for i, c := range sent {
		b, _ := c.Encode()
		if _, err := js.Publish(ctx, sc.Subject, b, jetstream.WithMsgID(fmt.Sprintf("%s-%d", c.ID, i))); err != nil {
			t.Fatal(err)
		}
	}
	// One poison message must be terminated, not block the stream.
	if _, err := js.Publish(ctx, sc.Subject, []byte("{not json")); err != nil {
		t.Fatal(err)
	}

	app := &lostAck{inner: *st}
	app.fail.Store(2)
	reg := prometheus.NewRegistry()
	c := New(js, app, Options{Stream: sc.Name, Subject: sc.Subject, Durable: "analytics-it", BatchSize: 64, FetchMaxWait: 200 * time.Millisecond, AckWait: 5 * time.Second, MaxAckPending: 1000}, log, reg)
	c.Start(ctx, sc)

	deadline := time.Now().Add(30 * time.Second)
	var total int64
	for time.Now().Before(deadline) {
		s, err := st.Stats(ctx, "e2e", 5)
		if err != nil {
			t.Fatal(err)
		}
		total = s.Total
		cons, err := js.Consumer(ctx, sc.Name, "analytics-it")
		if err == nil {
			if info, err := cons.Info(ctx); err == nil && total == unique && info.NumPending == 0 && info.NumAckPending == 0 {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if total != unique {
		t.Fatalf("total = %d, want exactly %d despite duplicates and redeliveries", total, unique)
	}
	if n := app.fail.Load(); n >= 0 {
		t.Fatalf("simulated failures not exercised (remaining %d)", n)
	}
}
