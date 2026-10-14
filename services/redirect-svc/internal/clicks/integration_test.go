//go:build integration

package clicks

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/pkg/events"
)

func TestJetStreamPublishAndDedup(t *testing.T) {
	url := os.Getenv("TEST_NATS_URL")
	if url == "" {
		t.Skip("TEST_NATS_URL not set")
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	nc, err := events.Connect(url, "redirect-it", log)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg := events.StreamConfig{Name: "CLICKS_IT_REDIRECT", Subject: "it.redirect.clicks", Replicas: 1, MaxAge: time.Hour, MaxBytes: 64 << 20, DuplicateWindow: time.Minute}
	_ = js.DeleteStream(ctx, cfg.Name)
	stream, err := events.EnsureStream(ctx, js, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Idempotent: a second EnsureStream with the same config is a no-op.
	if _, err := events.EnsureStream(ctx, js, cfg); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = js.DeleteStream(context.Background(), cfg.Name) }()

	pub := JetStream{JS: js, Stream: cfg.Name, Subject: cfg.Subject}
	r := New(pub, Options{BufferSize: 1000, Workers: 8}, log, prometheus.NewRegistry())
	for range 500 {
		r.Record(events.Click{ID: events.NewID(), Slug: "it", At: time.Now().UTC(), Referrer: "direct"})
	}
	// A retried publish after a lost ack reuses the same ID.
	dup := events.Click{ID: events.NewID(), Slug: "it", At: time.Now().UTC(), Referrer: "direct"}
	r.Record(dup)
	r.Record(dup)
	if err := r.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 501 {
		t.Fatalf("stream has %d messages, want 501 (500 + 1 de-duplicated)", info.State.Msgs)
	}
}
