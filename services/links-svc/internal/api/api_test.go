package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/services/links-svc/internal/links"
)

// memStore is an in-memory links.Store for handler tests.
type memStore struct {
	mu       sync.Mutex
	m        map[string]links.Link
	failNext error
	taken    int // number of upcoming Create calls that report ErrSlugTaken
}

func newMem() *memStore { return &memStore{m: map[string]links.Link{}} }

func (s *memStore) Create(_ context.Context, l links.Link) (links.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return links.Link{}, err
	}
	if s.taken > 0 {
		s.taken--
		return links.Link{}, links.ErrSlugTaken
	}
	if _, ok := s.m[l.Slug]; ok {
		return links.Link{}, links.ErrSlugTaken
	}
	l.CreatedAt = time.Now().Add(time.Duration(len(s.m)) * time.Millisecond)
	s.m[l.Slug] = l
	return l, nil
}

func (s *memStore) Get(_ context.Context, slug string) (links.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.m[slug]
	if !ok {
		return links.Link{}, links.ErrNotFound
	}
	return l, nil
}

func (s *memStore) Recent(_ context.Context, limit int, before time.Time) ([]links.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []links.Link
	for _, l := range s.m {
		if before.IsZero() || l.CreatedAt.Before(before) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memStore) Ping(context.Context) error { return nil }

func setup() (*memStore, http.Handler) {
	st := newMem()
	a := New(st, "http://sho.rt/", slog.New(slog.NewTextHandler(io.Discard, nil)), prometheus.NewRegistry())
	return st, a.Routes()
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestCreateGenerated(t *testing.T) {
	_, h := setup()
	rec := do(h, "POST", "/api/links", `{"url":"https://Example.com/a?b=c"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body)
	}
	got := decode[linkResponse](t, rec)
	if len(got.Slug) != 7 || got.URL != "https://example.com/a?b=c" || got.Custom {
		t.Fatalf("unexpected %+v", got)
	}
	if got.ShortURL != "http://sho.rt/"+got.Slug {
		t.Fatalf("short_url = %q", got.ShortURL)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/links/"+got.Slug {
		t.Fatalf("Location = %q", loc)
	}
}

func TestCreateCustomAndConflict(t *testing.T) {
	_, h := setup()
	if rec := do(h, "POST", "/api/links", `{"url":"https://a.io","slug":"launch"}`); rec.Code != 201 {
		t.Fatalf("first create: %d", rec.Code)
	}
	rec := do(h, "POST", "/api/links", `{"url":"https://b.io","slug":"launch"}`)
	if rec.Code != http.StatusConflict || decode[map[string]string](t, rec)["code"] != "slug_taken" {
		t.Fatalf("conflict: %d %s", rec.Code, rec.Body)
	}
}

func TestCreateRetriesGeneratedCollisions(t *testing.T) {
	st, h := setup()
	st.taken = 3
	if rec := do(h, "POST", "/api/links", `{"url":"https://a.io"}`); rec.Code != 201 {
		t.Fatalf("code = %d", rec.Code)
	}
	st.taken = 10
	if rec := do(h, "POST", "/api/links", `{"url":"https://a.io"}`); rec.Code != 500 {
		t.Fatalf("expected 500 after exhausting attempts, got %d", rec.Code)
	}
}

func TestCreateValidation(t *testing.T) {
	_, h := setup()
	cases := map[string]string{
		`not json`:                                  "invalid_json",
		`{"url":"https://a.io","extra":1}`:          "invalid_json",
		`{"url":"ftp://a.io"}`:                      "invalid_url",
		`{"url":"https://a.io","slug":"api"}`:       "invalid_slug",
		`{"url":"https://a.io","slug":"no spaces"}`: "invalid_slug",
	}
	for body, code := range cases {
		rec := do(h, "POST", "/api/links", body)
		if rec.Code != 400 || decode[map[string]string](t, rec)["code"] != code {
			t.Errorf("%s -> %d %s, want 400 %s", body, rec.Code, rec.Body, code)
		}
	}
	big := `{"url":"https://a.io/` + strings.Repeat("a", 9000) + `"}`
	if rec := do(h, "POST", "/api/links", big); rec.Code != 400 {
		t.Errorf("oversized body: %d", rec.Code)
	}
}

func TestGet(t *testing.T) {
	st, h := setup()
	_, _ = st.Create(context.Background(), links.Link{Slug: "hello", URL: "https://a.io"})
	if rec := do(h, "GET", "/api/links/hello", ""); rec.Code != 200 || decode[linkResponse](t, rec).URL != "https://a.io" {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/links/missing", ""); rec.Code != 404 {
		t.Fatalf("missing: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/links/a", ""); rec.Code != 404 {
		t.Fatalf("invalid slug should 404 without hitting the store: %d", rec.Code)
	}
}

func TestListPagination(t *testing.T) {
	st, h := setup()
	for _, s := range []string{"aaa", "bbb", "ccc"} {
		_, _ = st.Create(context.Background(), links.Link{Slug: s, URL: "https://a.io/" + s})
	}
	page := decode[listResponse](t, do(h, "GET", "/api/links?limit=2", ""))
	if len(page.Links) != 2 || page.Links[0].Slug != "ccc" || page.NextBefore == nil {
		t.Fatalf("page1 = %+v", page)
	}
	page2 := decode[listResponse](t, do(h, "GET", "/api/links?limit=2&before="+url.QueryEscape(page.NextBefore.Format(time.RFC3339Nano)), ""))
	if len(page2.Links) != 1 || page2.Links[0].Slug != "aaa" || page2.NextBefore != nil {
		t.Fatalf("page2 = %+v", page2)
	}
	for _, q := range []string{"limit=0", "limit=x", "before=yesterday"} {
		if rec := do(h, "GET", "/api/links?"+q, ""); rec.Code != 400 {
			t.Errorf("%s -> %d", q, rec.Code)
		}
	}
}

func TestStoreErrorIs500(t *testing.T) {
	st, h := setup()
	st.failNext = errors.New("db down")
	if rec := do(h, "POST", "/api/links", `{"url":"https://a.io","slug":"abcd"}`); rec.Code != 500 || strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("expected opaque 500, got %d %s", rec.Code, rec.Body)
	}
}
