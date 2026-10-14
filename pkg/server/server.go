// Package server runs a service's public HTTP listener and its admin
// listener (/healthz, /readyz, /metrics) with Kubernetes-friendly graceful
// shutdown:
//
//  1. SIGTERM arrives (Kubernetes has already started removing the pod from
//     Endpoints, but kube-proxy/Traefik may lag a little).
//  2. /readyz flips to 503 immediately, so the readiness probe confirms the
//     removal.
//  3. We keep serving for DrainDelay so in-flight routing updates settle.
//  4. http.Server.Shutdown stops accepting and waits for in-flight requests
//     up to ShutdownTimeout.
//  5. Background workers are stopped (Stop hooks), then dependencies closed.
//
// The pod's terminationGracePeriodSeconds must exceed preStop + DrainDelay +
// ShutdownTimeout.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/prasadgujar22/shorty/pkg/buildinfo"
	"github.com/prasadgujar22/shorty/pkg/config"
)

// Check is a readiness dependency check. It must honour ctx.
type Check func(ctx context.Context) error

type namedCheck struct {
	fn       Check
	critical bool
}

// Options configure the listeners and shutdown behaviour.
type Options struct {
	Addr            string
	AdminAddr       string
	DrainDelay      time.Duration
	ShutdownTimeout time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	CheckTimeout    time.Duration
}

// OptionsFromEnv reads the standard listener/shutdown variables.
func OptionsFromEnv(env *config.Env) Options {
	return Options{
		Addr:            env.String("HTTP_ADDR", ":8080"),
		AdminAddr:       env.String("ADMIN_ADDR", ":9090"),
		DrainDelay:      env.Duration("SHUTDOWN_DRAIN_DELAY", 5*time.Second),
		ShutdownTimeout: env.Duration("SHUTDOWN_TIMEOUT", 15*time.Second),
		ReadTimeout:     env.Duration("HTTP_READ_TIMEOUT", 5*time.Second),
		WriteTimeout:    env.Duration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:     env.Duration("HTTP_IDLE_TIMEOUT", 90*time.Second),
		CheckTimeout:    env.Duration("READINESS_CHECK_TIMEOUT", 2*time.Second),
	}
}

// Server is a service process: public handler + admin endpoints.
type Server struct {
	opts     Options
	log      *slog.Logger
	Registry *prometheus.Registry

	mu     sync.Mutex
	checks map[string]namedCheck
	stops  []func(context.Context) error

	ready    atomic.Bool // dependencies initialised
	draining atomic.Bool // shutdown started
}

// New creates a server with a fresh Prometheus registry that already
// contains Go runtime, process and build-info collectors.
func New(opts Options, log *slog.Logger) *Server {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	buildinfo.Register(reg)
	return &Server{opts: opts, log: log, Registry: reg, checks: map[string]namedCheck{}}
}

// AddCheck registers a critical readiness check: when it fails the pod is
// taken out of load balancing.
func (s *Server) AddCheck(name string, c Check) { s.addCheck(name, c, true) }

// AddSoftCheck registers a dependency the service can degrade around. Its
// status is reported by /readyz ("degraded: ...") but never fails readiness:
// pulling every replica out of rotation because a shared dependency blipped
// would turn a partial failure into a full outage.
func (s *Server) AddSoftCheck(name string, c Check) { s.addCheck(name, c, false) }

func (s *Server) addCheck(name string, c Check, critical bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks[name] = namedCheck{fn: c, critical: critical}
}

// OnStop registers a hook run after the HTTP server has drained, in reverse
// registration order (like defer).
func (s *Server) OnStop(f func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops = append(s.stops, f)
}

// SetReady marks dependency initialisation as complete.
func (s *Server) SetReady() { s.ready.Store(true) }

// Draining reports whether shutdown has begun.
func (s *Server) Draining() bool { return s.draining.Load() }

// AdminHandler serves /healthz, /readyz and /metrics.
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		// Liveness only proves the process can serve HTTP. It must not
		// depend on downstreams, or a DB blip would restart every pod.
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.Registry, promhttp.HandlerOpts{Registry: s.Registry}))
	return mux
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{}
	code := http.StatusOK
	switch {
	case s.draining.Load():
		status["server"] = "draining"
		code = http.StatusServiceUnavailable
	case !s.ready.Load():
		status["server"] = "starting"
		code = http.StatusServiceUnavailable
	default:
		s.mu.Lock()
		checks := make(map[string]namedCheck, len(s.checks))
		for k, v := range s.checks {
			checks[k] = v
		}
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(r.Context(), s.opts.CheckTimeout)
		defer cancel()
		var wg sync.WaitGroup
		var mu sync.Mutex
		for name, check := range checks {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := check.fn(ctx)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err != nil && check.critical:
					status[name] = err.Error()
					code = http.StatusServiceUnavailable
				case err != nil:
					status[name] = "degraded: " + err.Error()
				default:
					status[name] = "ok"
				}
			}()
		}
		wg.Wait()
	}
	if code != http.StatusOK {
		s.log.Warn("not ready", "status", status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(encodeStatus(status))
}

func encodeStatus(m map[string]string) []byte {
	b := []byte{'{'}
	first := true
	for k, v := range m {
		if !first {
			b = append(b, ',')
		}
		first = false
		b = appendQuoted(b, k)
		b = append(b, ':')
		b = appendQuoted(b, v)
	}
	return append(b, '}', '\n')
}

func appendQuoted(b []byte, s string) []byte {
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b = append(b, '\\', byte(r))
		case '\n':
			b = append(b, '\\', 'n')
		default:
			if r < 0x20 {
				continue
			}
			b = append(b, string(r)...)
		}
	}
	return append(b, '"')
}

// StartAdmin starts the admin listener in the background so liveness and
// startup probes answer while dependencies are still initialising.
func (s *Server) StartAdmin() (*http.Server, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", s.opts.AdminAddr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: s.AdminHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("admin server failed", "err", err)
		}
	}()
	s.log.Info("admin listening", "addr", ln.Addr().String())
	return srv, nil
}

// Run serves h on the public listener until ctx is cancelled or SIGTERM /
// SIGINT arrives, then performs the graceful shutdown sequence.
func (s *Server) Run(ctx context.Context, h http.Handler, admin *http.Server) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: s.opts.ReadTimeout,
		ReadTimeout:       s.opts.ReadTimeout,
		WriteTimeout:      s.opts.WriteTimeout,
		IdleTimeout:       s.opts.IdleTimeout,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	s.log.Info("listening", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	s.draining.Store(true)
	s.log.Info("shutdown: draining", "drain_delay", s.opts.DrainDelay.String())
	time.Sleep(s.opts.DrainDelay)

	// Disable keep-alives so clients reconnect to other pods.
	srv.SetKeepAlivesEnabled(false)
	sctx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		s.log.Error("shutdown: http server", "err", err)
	}
	s.log.Info("shutdown: http drained")

	s.mu.Lock()
	stops := append([]func(context.Context) error(nil), s.stops...)
	s.mu.Unlock()
	for i := len(stops) - 1; i >= 0; i-- {
		if err := stops[i](sctx); err != nil {
			s.log.Error("shutdown: stop hook", "err", err)
		}
	}
	if admin != nil {
		_ = admin.Shutdown(sctx)
	}
	s.log.Info("shutdown: complete")
	return nil
}

// Retry calls f until it succeeds, ctx expires, or the deadline passes,
// backing off exponentially (capped at 5s). Used for startup dependencies.
func Retry(ctx context.Context, log *slog.Logger, what string, timeout time.Duration, f func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	backoff := 200 * time.Millisecond
	for attempt := 1; ; attempt++ {
		err := f(ctx)
		if err == nil {
			return nil
		}
		log.Warn("dependency not ready, retrying", "dependency", what, "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}
