// Package linksclient looks up slugs in links-svc over HTTP.
package linksclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prasadgujar22/shorty/services/redirect-svc/internal/resolve"
)

// Client implements resolve.Source.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for links-svc at base (e.g. http://links-svc:8080).
func New(base string, timeout time.Duration, maxIdlePerHost int) *Client {
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        maxIdlePerHost,
		MaxIdleConnsPerHost: maxIdlePerHost,
		IdleConnTimeout:     60 * time.Second,
	}
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Transport: tr, Timeout: timeout}}
}

func (c *Client) Lookup(ctx context.Context, slug string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/links/"+url.PathEscape(slug), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", resolve.ErrNotFound
	default:
		return "", fmt.Errorf("links-svc: unexpected status %d", resp.StatusCode)
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return "", fmt.Errorf("links-svc: decode: %w", err)
	}
	if body.URL == "" {
		return "", fmt.Errorf("links-svc: empty url for %q", slug)
	}
	return body.URL, nil
}

// Ping is a (soft) readiness check: links-svc answers its admin-free API.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/links?limit=1", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("links-svc status %d", resp.StatusCode)
	}
	return nil
}
