package links

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://Example.COM/Path?q=1": "https://example.com/Path?q=1",
		"  http://a.io  ":              "http://a.io",
		"HTTPS://x.dev/#frag":          "https://x.dev/#frag",
	}
	for in, want := range cases {
		got, err := NormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "example.com", "ftp://x.io", "javascript:alert(1)", "https://", "https://user:pw@evil.com", "/relative", "https://x.io/" + strings.Repeat("a", 2048)} {
		if _, err := NormalizeURL(in); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("NormalizeURL(%q) err = %v, want ErrInvalidURL", in, err)
		}
	}
}

func TestValidateSlug(t *testing.T) {
	if err := ValidateSlug("launch-2026"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSlug("api"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("reserved slug accepted: %v", err)
	}
}
