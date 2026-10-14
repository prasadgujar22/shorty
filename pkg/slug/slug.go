// Package slug defines what a valid short-link slug is. It is shared by
// links-svc (validating custom slugs, generating random ones) and
// redirect-svc (rejecting junk paths before touching Redis or links-svc).
package slug

import (
	"crypto/rand"
	"regexp"
	"strings"
)

const (
	MinLen        = 3
	MaxLen        = 32
	GeneratedLen  = 7
	alphabet      = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/l/I
	alphabetSize  = byte(len(alphabet))
	maxUnbiasable = 256 - 256%int(alphabetSize)
)

var pattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,31}$`)

// reserved are first path segments the gateway routes elsewhere, or that we
// want to keep available for future UI/ops routes. Compared case-insensitively.
var reserved = map[string]struct{}{
	"api": {}, "app": {}, "_app": {}, "s": {}, "stats": {}, "static": {}, "assets": {}, "fonts": {},
	"healthz": {}, "readyz": {}, "livez": {}, "metrics": {}, "favicon.ico": {}, "favicon.svg": {},
	"robots.txt": {}, "index.html": {}, "admin": {}, "login": {}, "logout": {}, "new": {},
}

// Valid reports whether s is a syntactically valid, non-reserved slug.
func Valid(s string) bool {
	if !pattern.MatchString(s) {
		return false
	}
	_, isReserved := reserved[strings.ToLower(s)]
	return !isReserved
}

// Reserved reports whether s is a reserved word.
func Reserved(s string) bool {
	_, ok := reserved[strings.ToLower(s)]
	return ok
}

// New returns a random slug of GeneratedLen characters from an unambiguous
// alphabet using crypto/rand with rejection sampling (no modulo bias).
func New() string {
	out := make([]byte, 0, GeneratedLen)
	buf := make([]byte, GeneratedLen*2)
	for len(out) < GeneratedLen {
		_, _ = rand.Read(buf) // never returns an error on supported platforms
		for _, b := range buf {
			if int(b) >= maxUnbiasable {
				continue
			}
			out = append(out, alphabet[b%alphabetSize])
			if len(out) == GeneratedLen {
				break
			}
		}
	}
	return string(out)
}
