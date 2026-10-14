package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/resolve"
)

type fakeResolver map[string]string

func (f fakeResolver) Resolve(_ context.Context, s string) (string, error) {
	if s == "broken" {
		return "", errors.Join(resolve.ErrUnavailable, errors.New("down"))
	}
	if u, ok := f[s]; ok {
		return u, nil
	}
	return "", resolve.ErrNotFound
}

type fakeRecorder struct{ got []events.Click }

func (f *fakeRecorder) Record(c events.Click) bool { f.got = append(f.got, c); return true }

func setup() (*fakeRecorder, http.Handler) {
	rec := &fakeRecorder{}
	h := &Handler{Resolver: fakeResolver{"abc": "https://a.io/x"}, Recorder: rec,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }}
	return rec, h.Routes()
}

func TestRedirectRecordsClick(t *testing.T) {
	rec, h := setup()
	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req.Header.Set("Referer", "https://www.reddit.com/r/golang")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://a.io/x" {
		t.Fatalf("got %d %q", w.Code, w.Header().Get("Location"))
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache-control = %q", w.Header().Get("Cache-Control"))
	}
	if len(rec.got) != 1 {
		t.Fatalf("clicks = %d", len(rec.got))
	}
	c := rec.got[0]
	if c.Slug != "abc" || c.Referrer != "reddit.com" || len(c.ID) != 32 || !c.At.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("click = %+v", c)
	}
}

func TestHeadDoesNotRecord(t *testing.T) {
	rec, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodHead, "/abc", nil))
	if w.Code != http.StatusFound || len(rec.got) != 0 {
		t.Fatalf("HEAD: %d clicks=%d", w.Code, len(rec.got))
	}
}

func TestNotFoundAndInvalid(t *testing.T) {
	rec, h := setup()
	for _, p := range []string{"/missing", "/favicon.ico", "/a", "/abc/extra", "/"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s -> %d", p, w.Code)
		}
	}
	if len(rec.got) != 0 {
		t.Fatal("no clicks for 404s")
	}
}

func TestUnavailable(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/broken", nil))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("got %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, h := setup()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/abc", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", w.Code)
	}
}
