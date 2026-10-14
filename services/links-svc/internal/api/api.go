// Package api exposes links-svc over HTTP.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prasadgujar22/shorty/pkg/httpx"
	"github.com/prasadgujar22/shorty/pkg/slug"
	"github.com/prasadgujar22/shorty/services/links-svc/internal/links"
)

const (
	maxBody         = 8 << 10
	defaultLimit    = 20
	maxLimit        = 100
	maxSlugAttempts = 5
)

// API holds handler dependencies.
type API struct {
	Store   links.Store
	BaseURL string // public origin used to build short_url, e.g. http://localhost:8088
	Log     *slog.Logger
	created *prometheus.CounterVec
}

// New builds the API and registers its business metrics.
func New(store links.Store, baseURL string, log *slog.Logger, reg prometheus.Registerer) *API {
	a := &API{Store: store, BaseURL: strings.TrimRight(baseURL, "/"), Log: log,
		created: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "shorty_links_created_total",
			Help: "Links created, by slug kind (custom|generated).",
		}, []string{"kind"}),
	}
	reg.MustRegister(a.created)
	return a
}

// Routes returns the public mux.
func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", a.create)
	mux.HandleFunc("GET /api/links", a.list)
	mux.HandleFunc("GET /api/links/{slug}", a.get)
	return mux
}

type linkResponse struct {
	links.Link
	ShortURL string `json:"short_url"`
}

func (a *API) resp(l links.Link) linkResponse {
	return linkResponse{Link: l, ShortURL: a.BaseURL + "/" + l.Slug}
}

type createRequest struct {
	URL  string `json:"url"`
	Slug string `json:"slug,omitempty"`
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_json", "request body must be JSON: {\"url\": \"...\", \"slug\": \"optional\"}")
		return
	}
	u, err := links.NormalizeURL(req.URL)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}
	req.Slug = strings.TrimSpace(req.Slug)

	if req.Slug != "" {
		if err := links.ValidateSlug(req.Slug); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_slug", err.Error())
			return
		}
		l, err := a.Store.Create(r.Context(), links.Link{Slug: req.Slug, URL: u, Custom: true})
		switch {
		case errors.Is(err, links.ErrSlugTaken):
			httpx.Fail(w, http.StatusConflict, "slug_taken", err.Error())
		case err != nil:
			a.internal(w, "create link", err)
		default:
			a.created.WithLabelValues("custom").Inc()
			w.Header().Set("Location", "/api/links/"+l.Slug)
			httpx.JSON(w, http.StatusCreated, a.resp(l))
		}
		return
	}

	// Generated slug: 56^7 ≈ 1.7e12 space, so collisions are rare; retry a few times.
	for range maxSlugAttempts {
		l, err := a.Store.Create(r.Context(), links.Link{Slug: slug.New(), URL: u})
		if errors.Is(err, links.ErrSlugTaken) {
			continue
		}
		if err != nil {
			a.internal(w, "create link", err)
			return
		}
		a.created.WithLabelValues("generated").Inc()
		w.Header().Set("Location", "/api/links/"+l.Slug)
		httpx.JSON(w, http.StatusCreated, a.resp(l))
		return
	}
	a.internal(w, "create link", errors.New("could not generate a unique slug"))
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	s := r.PathValue("slug")
	if !slug.Valid(s) {
		httpx.Fail(w, http.StatusNotFound, "not_found", links.ErrNotFound.Error())
		return
	}
	l, err := a.Store.Get(r.Context(), s)
	switch {
	case errors.Is(err, links.ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, "not_found", err.Error())
	case err != nil:
		a.internal(w, "get link", err)
	default:
		httpx.JSON(w, http.StatusOK, a.resp(l))
	}
}

type listResponse struct {
	Links      []linkResponse `json:"links"`
	NextBefore *time.Time     `json:"next_before,omitempty"`
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httpx.Fail(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
			return
		}
		limit = min(n, maxLimit)
	}
	var before time.Time
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_before", "before must be an RFC 3339 timestamp")
			return
		}
		before = t
	}
	ls, err := a.Store.Recent(r.Context(), limit, before)
	if err != nil {
		a.internal(w, "list links", err)
		return
	}
	out := listResponse{Links: make([]linkResponse, 0, len(ls))}
	for _, l := range ls {
		out.Links = append(out.Links, a.resp(l))
	}
	if len(ls) == limit {
		next := ls[len(ls)-1].CreatedAt
		out.NextBefore = &next
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) internal(w http.ResponseWriter, op string, err error) {
	a.Log.Error(op+" failed", "err", err)
	httpx.Fail(w, http.StatusInternalServerError, "internal", "internal error")
}
