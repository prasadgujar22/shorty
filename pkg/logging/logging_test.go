package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestJSONOutputAndLevel(t *testing.T) {
	var buf bytes.Buffer
	l := newWithWriter(&buf, "links-svc", "warn")
	l.Info("dropped")
	l.Warn("kept", "slug", "abc")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("expected exactly one JSON line, got %q: %v", buf.String(), err)
	}
	for k, want := range map[string]any{"msg": "kept", "service": "links-svc", "slug": "abc", "level": "WARN"} {
		if rec[k] != want {
			t.Errorf("%s = %v, want %v", k, rec[k], want)
		}
	}
	if _, ok := rec["ts"]; !ok {
		t.Error("missing ts field")
	}
}
