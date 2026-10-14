// redirect-svc serves GET /{slug} with a 302. It is stateless and sits on
// the hot path: Redis cache-aside lookup (links-svc on a miss), and click
// events are published to NATS JetStream asynchronously.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/prasadgujar22/shorty/pkg/config"
	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/pkg/httpx"
	"github.com/prasadgujar22/shorty/pkg/logging"
	"github.com/prasadgujar22/shorty/pkg/memlimit"
	"github.com/prasadgujar22/shorty/pkg/server"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/api"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/cache"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/clicks"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/linksclient"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/resolve"
)

const service = "redirect-svc"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	env := config.New()
	log := logging.New(service, env.String("LOG_LEVEL", "info"))
	memlimit.Apply(log, env.Float("GOMEMLIMIT_RATIO", 0.9))
	opts := server.OptionsFromEnv(env)
	redisOpts := cache.OptionsFromEnv(env)
	resolveOpts := resolve.Options{
		TTL:         env.Duration("CACHE_TTL", time.Hour),
		TTLJitter:   env.Float("CACHE_TTL_JITTER", 0.1),
		NegativeTTL: env.Duration("CACHE_NEGATIVE_TTL", 30*time.Second),
	}
	linksURL := env.String("LINKS_URL", "http://links-svc:8080")
	linksTimeout := env.Duration("LINKS_TIMEOUT", 500*time.Millisecond)
	linksMaxIdle := env.Int("LINKS_MAX_IDLE_CONNS", 64)
	natsURL := env.String("NATS_URL", "nats://nats:4222")
	stream := events.StreamConfigFromEnv(env)
	recOpts := clicks.Options{
		BufferSize:     env.Int("CLICK_BUFFER_SIZE", 100_000),
		Workers:        env.Int("CLICK_PUBLISH_WORKERS", 16),
		PublishTimeout: env.Duration("CLICK_PUBLISH_TIMEOUT", 2*time.Second),
		MaxBackoff:     env.Duration("CLICK_PUBLISH_MAX_BACKOFF", 2*time.Second),
	}
	if err := env.Err(); err != nil {
		return err
	}

	srv := server.New(opts, log)
	admin, err := srv.StartAdmin()
	if err != nil {
		return err
	}
	ctx := context.Background()

	// Redis: lazy, never blocks startup. A cold or dead cache only costs latency.
	rc := cache.New(redisOpts, srv.Registry)
	srv.AddSoftCheck("redis", rc.Ping)
	srv.OnStop(func(context.Context) error { return rc.Close() })

	lc := linksclient.New(linksURL, linksTimeout, linksMaxIdle)
	srv.AddSoftCheck("links-svc", lc.Ping)

	// NATS: connect in the background (RetryOnFailedConnect). Events buffer
	// in memory while it is unavailable.
	nc, err := events.Connect(natsURL, service, log)
	if err != nil {
		return err
	}
	srv.AddSoftCheck("nats", events.Ping(nc))
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}
	go func() {
		// Ensure the stream exists; retry until NATS is reachable.
		_ = server.Retry(ctx, log, "jetstream stream", 24*time.Hour, func(ctx context.Context) error {
			sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_, err := events.EnsureStream(sctx, js, stream)
			if err == nil {
				log.Info("jetstream stream ready", "stream", stream.Name, "replicas", stream.Replicas)
			}
			return err
		})
	}()

	rec := clicks.New(clicks.JetStream{JS: js, Stream: stream.Name}, recOpts, log, srv.Registry)
	// Stop hooks run in reverse: flush clicks first, then drain NATS.
	srv.OnStop(func(context.Context) error { return nc.Drain() })
	srv.OnStop(rec.Stop)

	h := &api.Handler{
		Resolver: resolve.New(rc, lc, resolveOpts, srv.Registry),
		Recorder: rec,
		Log:      log,
	}
	handler := httpx.Instrument(h.Routes(), httpx.NewMetrics(srv.Registry), log)
	srv.SetReady()
	log.Info("started", "links_url", linksURL, "redis", redisOpts.Addr, "nats", natsURL,
		"cache_ttl", resolveOpts.TTL.String(), "click_workers", recOpts.Workers, "click_buffer", recOpts.BufferSize)
	return srv.Run(ctx, handler, admin)
}
