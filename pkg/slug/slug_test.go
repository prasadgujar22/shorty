package slug

import (
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	ok := []string{"abc", "my-link", "Q3_report", "a1b2c3", strings.Repeat("x", 32)}
	bad := []string{"", "ab", "-abc", "_abc", "a b", "abc/def", "émoji", strings.Repeat("x", 33), "api", "API", "healthz", "favicon.ico", "s", "stats"}
	for _, s := range ok {
		if !Valid(s) {
			t.Errorf("Valid(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if Valid(s) {
			t.Errorf("Valid(%q) = true, want false", s)
		}
	}
}

func TestNewIsValidAndVaried(t *testing.T) {
	seen := map[string]bool{}
	for range 2000 {
		s := New()
		if len(s) != GeneratedLen || !Valid(s) {
			t.Fatalf("generated invalid slug %q", s)
		}
		if strings.ContainsAny(s, "0O1lI") {
			t.Fatalf("ambiguous character in %q", s)
		}
		seen[s] = true
	}
	if len(seen) < 1999 {
		t.Fatalf("too many collisions: %d unique of 2000", len(seen))
	}
}
