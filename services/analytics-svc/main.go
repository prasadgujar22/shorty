// analytics-svc consumes click events from JetStream, aggregates them
// idempotently into Postgres (schema "analytics"), and serves
// GET /api/stats/{slug}.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/prasadgujar22/shorty/pkg/config"
	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/pkg/httpx"
	"github.com/prasadgujar22/shorty/pkg/logging"
	"github.com/prasadgujar22/shorty/pkg/memlimit"
	"github.com/prasadgujar22/shorty/pkg/pg"
	"github.com/prasadgujar22/shorty/pkg/server"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/api"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/consumer"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/store"
)

const service = "analytics-svc"

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
	dbCfg := pg.ConfigFromEnv(env, "analytics")
	natsURL := env.String("NATS_URL", "nats://nats:4222")
	stream := events.StreamConfigFromEnv(env)
	consOpts := consumer.Options{
		Stream:        stream.Name,
		Subject:       stream.Subject,
		Durable:       env.String("CONSUMER_NAME", "analytics"),
		BatchSize:     env.Int("CONSUMER_BATCH_SIZE", 500),
		FetchMaxWait:  env.Duration("CONSUMER_FETCH_MAX_WAIT", 500*time.Millisecond),
		AckWait:       env.Duration("CONSUMER_ACK_WAIT", 30*time.Second),
		MaxAckPending: env.Int("CONSUMER_MAX_ACK_PENDING", 20_000),
	}
	topReferrers := env.Int("STATS_TOP_REFERRERS", 10)
	retention := env.Duration("PROCESSED_EVENTS_RETENTION", 8*24*time.Hour)
	startupTimeout := env.Duration("STARTUP_TIMEOUT", 90*time.Second)
	if err := env.Err(); err != nil {
		return err
	}

	srv := server.New(opts, log)
	admin, err := srv.StartAdmin()
	if err != nil {
		return err
	}
	ctx := context.Background()

	var pool *pgxpool.Pool
	if err := server.Retry(ctx, log, "postgres", startupTimeout, func(ctx context.Context) error {
		p, err := pg.Connect(ctx, dbCfg)
		if err != nil {
			return err
		}
		if err := pg.Migrate(ctx, p, store.Migrations(), log); err != nil {
			p.Close()
			return err
		}
		pool = p
		return nil
	}); err != nil {
		return err
	}
	srv.OnStop(func(context.Context) error { pool.Close(); return nil })
	pg.RegisterPoolMetrics(srv.Registry, pool)
	st := store.New(pool)
	srv.AddCheck("postgres", st.Ping)

	nc, err := events.Connect(natsURL, service, log)
	if err != nil {
		return err
	}
	// Stats keep being served while NATS is down; ingestion just lags.
	srv.AddSoftCheck("nats", events.Ping(nc))
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}
	cons := consumer.New(js, st, consOpts, log, srv.Registry)
	cons.Start(ctx, stream)
	// Reverse order on stop: finish the in-flight batch, then drain NATS, then close the pool.
	srv.OnStop(func(context.Context) error { return nc.Drain() })
	srv.OnStop(cons.Stop)

	go pruneLoop(ctx, st, retention, log)

	a := &api.API{Store: st, TopReferrers: topReferrers, Log: log}
	handler := httpx.Instrument(a.Routes(), httpx.NewMetrics(srv.Registry), log)
	srv.SetReady()
	log.Info("started", "schema", dbCfg.Schema, "stream", stream.Name, "durable", consOpts.Durable, "batch", consOpts.BatchSize)
	return srv.Run(ctx, handler, admin)
}

func pruneLoop(ctx context.Context, st *store.Postgres, retention time.Duration, log *slog.Logger) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n, err := st.PruneProcessed(ctx, retention)
		if err != nil {
			log.Warn("prune processed_events failed", "err", err)
		} else if n > 0 {
			log.Info("pruned processed_events", "rows", n)
		}
	}
}
