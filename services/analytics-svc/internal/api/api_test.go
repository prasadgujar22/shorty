package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/store"
)

type fakeStats struct{ err error }

func (f fakeStats) Stats(_ context.Context, s string, top int) (store.Stats, error) {
	if f.err != nil {
		return store.Stats{}, f.err
	}
	return store.Stats{Slug: s, Total: 42, Referrers: make([]store.Referrer, 0, top)}, nil
}

func serve(st StatsReader, path string) *httptest.ResponseRecorder {
	a := &API{Store: st, TopReferrers: 10, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	a.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestStats(t *testing.T) {
	rec := serve(fakeStats{}, "/api/stats/abc")
	var got store.Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != 200 || err != nil || got.Total != 42 || got.Slug != "abc" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := serve(fakeStats{}, "/api/stats/a"); rec.Code != 404 {
		t.Fatalf("invalid slug: %d", rec.Code)
	}
	if rec := serve(fakeStats{err: errors.New("db")}, "/api/stats/abc"); rec.Code != 500 {
		t.Fatalf("db error: %d", rec.Code)
	}
}
