// Package httpx contains HTTP middleware (RED metrics, access logs, panic
// recovery) and small JSON helpers shared by all services.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the RED (rate, errors, duration) collectors.
type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inflight prometheus.Gauge
}

// DefaultBuckets are tuned for sub-millisecond redirects up to slow API calls.
var DefaultBuckets = []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5}

// NewMetrics registers the HTTP collectors on reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "shorty_http_requests_total",
			Help: "HTTP requests by route pattern, method and status code.",
		}, []string{"route", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "shorty_http_request_duration_seconds",
			Help:    "HTTP request latency by route pattern and method.",
			Buckets: DefaultBuckets,
		}, []string{"route", "method"}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "shorty_http_requests_in_flight",
			Help: "Requests currently being served.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inflight)
	return m
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Instrument wraps a ServeMux-routed handler with recovery, RED metrics and
// access logging. The route label is the matched ServeMux pattern (bounded
// cardinality), never the raw path.
func Instrument(next http.Handler, m *Metrics, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		m.inflight.Inc()
		defer func() {
			m.inflight.Dec()
			if p := recover(); p != nil {
				log.Error("panic serving request", "panic", p, "path", r.URL.Path)
				if rec.status == 0 {
					http.Error(rec, `{"error":"internal error"}`, http.StatusInternalServerError)
				}
			}
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			elapsed := time.Since(start)
			m.requests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
			m.duration.WithLabelValues(route, r.Method).Observe(elapsed.Seconds())
			lvl := slog.LevelDebug
			if rec.status >= 500 {
				lvl = slog.LevelError
			}
			log.Log(r.Context(), lvl, "http request",
				"method", r.Method, "path", r.URL.Path, "route", route,
				"status", rec.status, "bytes", rec.bytes,
				"duration_ms", float64(elapsed.Microseconds())/1000,
				"remote", r.Header.Get("X-Forwarded-For"))
		}()
		next.ServeHTTP(rec, r)
	})
}

// JSON writes v as a JSON response with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error is the uniform error body: {"error": "...", "code": "..."}.
type Error struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// Fail writes an Error response.
func Fail(w http.ResponseWriter, status int, code, msg string) {
	JSON(w, status, Error{Error: msg, Code: code})
}
