// Package api serves GET /api/stats/{slug}.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prasadgujar22/shorty/pkg/httpx"
	"github.com/prasadgujar22/shorty/pkg/slug"
	"github.com/prasadgujar22/shorty/services/analytics-svc/internal/store"
)

// StatsReader reads aggregates.
type StatsReader interface {
	Stats(ctx context.Context, slug string, topReferrers int) (store.Stats, error)
}

// API holds handler dependencies.
type API struct {
	Store        StatsReader
	TopReferrers int
	Log          *slog.Logger
}

// Routes returns the public mux.
func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/stats/{slug}", a.stats)
	return mux
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	s := r.PathValue("slug")
	if !slug.Valid(s) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "unknown slug")
		return
	}
	// Existence is owned by links-svc; analytics returns zeroed series for
	// slugs it has never seen, which is also the truth for a brand-new link.
	st, err := a.Store.Stats(r.Context(), s, a.TopReferrers)
	if err != nil {
		a.Log.Error("stats query failed", "slug", s, "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}
