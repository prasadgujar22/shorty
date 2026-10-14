// Package config reads 12-factor configuration from environment variables.
//
// Parsing errors are collected rather than returned one by one so a service
// can report every bad variable at startup in a single log line.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Env reads typed values from the environment, accumulating errors.
type Env struct {
	lookup func(string) (string, bool)
	errs   []error
}

// New returns an Env backed by os.LookupEnv.
func New() *Env { return &Env{lookup: os.LookupEnv} }

// NewFromMap returns an Env backed by a map; used in tests.
func NewFromMap(m map[string]string) *Env {
	return &Env{lookup: func(k string) (string, bool) { v, ok := m[k]; return v, ok }}
}

func (e *Env) raw(key string) (string, bool) {
	v, ok := e.lookup(key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// String returns the value of key or def when unset/empty.
func (e *Env) String(key, def string) string {
	if v, ok := e.raw(key); ok {
		return v
	}
	return def
}

// Required returns the value of key and records an error when it is unset.
func (e *Env) Required(key string) string {
	v, ok := e.raw(key)
	if !ok {
		e.errs = append(e.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

// Int returns key parsed as an int, or def.
func (e *Env) Int(key string, def int) int {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %q is not an integer", key, v))
		return def
	}
	return n
}

// Float returns key parsed as a float64, or def.
func (e *Env) Float(key string, def float64) float64 {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %q is not a number", key, v))
		return def
	}
	return f
}

// Duration returns key parsed with time.ParseDuration, or def.
func (e *Env) Duration(key string, def time.Duration) time.Duration {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %q is not a duration", key, v))
		return def
	}
	return d
}

// Bool returns key parsed with strconv.ParseBool, or def.
func (e *Env) Bool(key string, def bool) bool {
	v, ok := e.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %q is not a boolean", key, v))
		return def
	}
	return b
}

// Err returns all accumulated errors joined, or nil.
func (e *Env) Err() error { return errors.Join(e.errs...) }
