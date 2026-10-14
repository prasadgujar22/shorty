package events

import (
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	in := Click{ID: NewID(), Slug: "abc", At: time.Now().UTC().Truncate(time.Millisecond), Referrer: "news.ycombinator.com"}
	b, err := in.Encode()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decode(b)
	if err != nil || out != in {
		t.Fatalf("round trip: %+v %v", out, err)
	}
	if _, err := Decode([]byte(`{"slug":"x"}`)); err == nil {
		t.Fatal("expected validation error")
	}
	if a, b := NewID(), NewID(); len(a) != 32 || a == b {
		t.Fatal("ids must be 32 hex chars and unique")
	}
}

func TestReferrerHost(t *testing.T) {
	cases := map[string]string{
		"":                                  "direct",
		"https://www.Google.com/search?q=x": "google.com",
		"http://t.co/abc":                   "t.co",
		"android-app://com.slack":           "com.slack",
		"not a url":                         "unknown",
	}
	for in, want := range cases {
		if got := ReferrerHost(in); got != want {
			t.Errorf("ReferrerHost(%q) = %q, want %q", in, got, want)
		}
	}
}
