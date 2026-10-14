// Package api serves GET /{slug} -> 302.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prasadgujar22/shorty/pkg/events"
	"github.com/prasadgujar22/shorty/pkg/slug"
	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/resolve"
)

// Resolver looks up a slug.
type Resolver interface {
	Resolve(ctx context.Context, slug string) (string, error)
}

// Recorder records a click without blocking.
type Recorder interface {
	Record(c events.Click) bool
}

// Handler serves redirects.
type Handler struct {
	Resolver Resolver
	Recorder Recorder
	Log      *slog.Logger
	Now      func() time.Time
}

// Routes returns the public mux.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{slug}", h.redirect) // GET also matches HEAD
	return mux
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	s := r.PathValue("slug")
	if !slug.Valid(s) {
		notFound(w)
		return
	}
	target, err := h.Resolver.Resolve(r.Context(), s)
	switch {
	case errors.Is(err, resolve.ErrNotFound):
		notFound(w)
		return
	case err != nil:
		h.Log.Warn("resolve failed", "slug", s, "err", err)
		w.Header().Set("Retry-After", "1")
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	if r.Method == http.MethodGet { // link previews often use HEAD; don't count them
		now := time.Now
		if h.Now != nil {
			now = h.Now
		}
		h.Recorder.Record(events.Click{
			ID:       events.NewID(),
			Slug:     s,
			At:       now().UTC(),
			Referrer: events.ReferrerHost(r.Referer()),
		})
	}
	// Never let browsers or proxies cache the redirect: every visit must
	// reach us to be counted.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "unsafe-url")
	// Short links are not content: keep them out of search indexes.
	w.Header().Set("X-Robots-Tag", "noindex")
	// Redirecting to stored URLs is the product; links-svc only accepts
	// absolute http(s) URLs without userinfo.
	http.Redirect(w, r, target, http.StatusFound) //nolint:gosec // G710: intended redirect
}

func notFound(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=30")
	http.Error(w, "short link not found", http.StatusNotFound)
}
