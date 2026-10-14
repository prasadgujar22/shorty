package linksclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/resolve"
)

func TestLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/links/abc":
			_, _ = w.Write([]byte(`{"slug":"abc","url":"https://a.io"}`))
		case "/api/links/boom":
			w.WriteHeader(http.StatusInternalServerError)
		case "/api/links/slow":
			time.Sleep(200 * time.Millisecond)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := New(srv.URL+"/", 100*time.Millisecond, 4)
	ctx := context.Background()

	if u, err := c.Lookup(ctx, "abc"); err != nil || u != "https://a.io" {
		t.Fatalf("found: %q %v", u, err)
	}
	if _, err := c.Lookup(ctx, "nope"); !errors.Is(err, resolve.ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
	if _, err := c.Lookup(ctx, "boom"); err == nil || errors.Is(err, resolve.ErrNotFound) {
		t.Fatalf("500 should be a non-NotFound error: %v", err)
	}
	if _, err := c.Lookup(ctx, "slow"); err == nil {
		t.Fatal("expected timeout")
	}
}
