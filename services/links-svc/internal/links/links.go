// Package links is the links-svc domain: what a link is, how input is
// validated, and the storage contract.
package links

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/prasadgujar22/shorty/pkg/slug"
)

// MaxURLLength bounds stored URLs (browsers handle more, but nothing sane needs it).
const MaxURLLength = 2048

// Link is a short link.
type Link struct {
	Slug      string    `json:"slug"`
	URL       string    `json:"url"`
	Custom    bool      `json:"custom"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	ErrNotFound    = errors.New("link not found")
	ErrSlugTaken   = errors.New("slug already exists")
	ErrInvalidURL  = errors.New("url must be an absolute http(s) URL up to 2048 characters")
	ErrInvalidSlug = errors.New("slug must be 3-32 characters of letters, digits, '-' or '_', start with a letter or digit, and not be reserved")
)

// Store persists links.
type Store interface {
	// Create inserts l; returns ErrSlugTaken on a duplicate slug.
	Create(ctx context.Context, l Link) (Link, error)
	Get(ctx context.Context, slug string) (Link, error)
	// Recent returns up to limit links, newest first, created strictly before `before` (zero = now).
	Recent(ctx context.Context, limit int, before time.Time) ([]Link, error)
	Ping(ctx context.Context) error
}

// NormalizeURL validates raw and returns its canonical form.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxURLLength {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		// Reject userinfo: "https://bank.com@evil.com" is a classic phishing trick.
		return "", ErrInvalidURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// ValidateSlug checks a user-supplied custom slug.
func ValidateSlug(s string) error {
	if !slug.Valid(s) {
		return ErrInvalidSlug
	}
	return nil
}
