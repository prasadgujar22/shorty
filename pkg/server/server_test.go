package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTest() *Server {
	return New(Options{CheckTimeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func get(t *testing.T, h http.Handler, path string) (int, map[string]string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestReadinessLifecycle(t *testing.T) {
	s := newTest()
	h := s.AdminHandler()
	var dbErr error
	s.AddCheck("postgres", func(context.Context) error { return dbErr })

	if code, body := get(t, h, "/readyz"); code != 503 || body["server"] != "starting" {
		t.Fatalf("before ready: %d %v", code, body)
	}
	s.SetReady()
	if code, body := get(t, h, "/readyz"); code != 200 || body["postgres"] != "ok" {
		t.Fatalf("ready: %d %v", code, body)
	}
	s.AddSoftCheck("redis", func(context.Context) error { return errors.New("down") })
	if code, body := get(t, h, "/readyz"); code != 200 || body["redis"] != "degraded: down" {
		t.Fatalf("soft check must not fail readiness: %d %v", code, body)
	}
	dbErr = errors.New(`conn "refused"`)
	if code, body := get(t, h, "/readyz"); code != 503 || body["postgres"] != `conn "refused"` {
		t.Fatalf("dep down: %d %v", code, body)
	}
	dbErr = nil
	s.draining.Store(true)
	if code, body := get(t, h, "/readyz"); code != 503 || body["server"] != "draining" {
		t.Fatalf("draining: %d %v", code, body)
	}
	// Liveness never depends on readiness state.
	if code, _ := get(t, h, "/healthz"); code != 200 {
		t.Fatalf("healthz = %d", code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	newTest().AdminHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "shorty_build_info") || !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("metrics missing expected series: %d", rec.Code)
	}
}

func TestRunGracefulShutdownWaitsForInflight(t *testing.T) {
	s := New(Options{Addr: "127.0.0.1:0", DrainDelay: 10 * time.Millisecond, ShutdownTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: 2 * time.Second},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	stopped := make(chan struct{})
	s.OnStop(func(context.Context) error { close(stopped); return nil })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // shut down immediately: Run must still drain and call hooks
	if err := s.Run(ctx, http.NotFoundHandler(), nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("stop hook not called")
	}
	if !s.Draining() {
		t.Fatal("expected draining=true after Run")
	}
}

func TestRetry(t *testing.T) {
	n := 0
	err := Retry(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), "x", 5*time.Second, func(context.Context) error {
		n++
		if n < 3 {
			return errors.New("nope")
		}
		return nil
	})
	if err != nil || n != 3 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	err = Retry(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), "x", 50*time.Millisecond, func(context.Context) error { return errors.New("never") })
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
