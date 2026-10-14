// links-svc owns short links: create, look up and list. It is the system of
// record (Postgres schema "links") that redirect-svc falls back to on a cache
// miss.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/prasadgujar22/shorty/pkg/config"
	"github.com/prasadgujar22/shorty/pkg/httpx"
	"github.com/prasadgujar22/shorty/pkg/logging"
	"github.com/prasadgujar22/shorty/pkg/memlimit"
	"github.com/prasadgujar22/shorty/pkg/pg"
	"github.com/prasadgujar22/shorty/pkg/server"
	"github.com/prasadgujar22/shorty/services/links-svc/internal/api"
	"github.com/prasadgujar22/shorty/services/links-svc/internal/store"
)

const service = "links-svc"

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
	dbCfg := pg.ConfigFromEnv(env, "links")
	baseURL := env.String("PUBLIC_BASE_URL", "http://localhost:8080")
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

	a := api.New(st, baseURL, log, srv.Registry)
	handler := httpx.Instrument(a.Routes(), httpx.NewMetrics(srv.Registry), log)
	srv.SetReady()
	log.Info("started", "schema", dbCfg.Schema, "max_conns", dbCfg.MaxConns, "base_url", baseURL)
	return srv.Run(ctx, handler, admin)
}
