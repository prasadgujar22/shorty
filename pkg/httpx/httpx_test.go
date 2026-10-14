package httpx

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestInstrumentRecordsRouteAndStatus(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusCreated, map[string]string{"id": r.PathValue("id")})
	})
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	h := Instrument(mux, m, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, p := range []string{"/items/1", "/items/2", "/boom", "/nope"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}

	want := `
# HELP shorty_http_requests_total HTTP requests by route pattern, method and status code.
# TYPE shorty_http_requests_total counter
shorty_http_requests_total{code="201",method="GET",route="GET /items/{id}"} 2
shorty_http_requests_total{code="404",method="GET",route="unmatched"} 1
shorty_http_requests_total{code="500",method="GET",route="GET /boom"} 1
`
	if err := testutil.CollectAndCompare(m.requests, strings.NewReader(want)); err != nil {
		t.Fatal(err)
	}
	if got := testutil.CollectAndCount(m.duration); got != 3 {
		t.Errorf("duration series = %d, want 3", got)
	}
}

func TestFailBody(t *testing.T) {
	rec := httptest.NewRecorder()
	Fail(rec, http.StatusConflict, "slug_taken", "slug already exists")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"slug_taken"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
}
