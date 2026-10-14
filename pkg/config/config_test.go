package config

import (
	"testing"
	"time"
)

func TestEnvParsingAndErrors(t *testing.T) {
	e := NewFromMap(map[string]string{"A": "x", "N": "42", "D": "1.5s", "B": "true", "F": "0.8", "BAD": "nope", "EMPTY": ""})
	if e.String("A", "d") != "x" || e.String("EMPTY", "d") != "d" || e.String("MISSING", "d") != "d" {
		t.Error("String")
	}
	if e.Int("N", 0) != 42 || e.Duration("D", 0) != 1500*time.Millisecond || !e.Bool("B", false) || e.Float("F", 0) != 0.8 {
		t.Error("typed values")
	}
	if e.Err() != nil {
		t.Fatalf("unexpected err %v", e.Err())
	}
	e.Int("BAD", 1)
	e.Required("MISSING")
	if e.Err() == nil {
		t.Fatal("expected accumulated errors")
	}
}
