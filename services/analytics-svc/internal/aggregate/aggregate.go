// Package aggregate turns a batch of click events into counter deltas.
// It is pure (no I/O) so the idempotency logic is easy to test.
package aggregate

import (
	"sort"
	"time"

	"github.com/prasadgujar22/shorty/pkg/events"
)

// HourKey identifies an hourly bucket.
type HourKey struct {
	Slug string
	Hour time.Time
}

// RefKey identifies a referrer counter.
type RefKey struct {
	Slug     string
	Referrer string
}

// Total is a per-slug delta.
type Total struct {
	Clicks int64
	LastAt time.Time
}

// Deltas are the increments a batch applies. Slices are sorted by key so
// concurrent writers always lock rows in the same order (no deadlocks).
type Deltas struct {
	Hourly    []Count[HourKey]
	Referrers []Count[RefKey]
	Totals    []Count[string]
	LastAt    map[string]time.Time
}

// Count pairs a key with an increment.
type Count[K any] struct {
	Key K
	N   int64
}

// UniqueIDs returns the distinct event IDs in the batch, in input order.
func UniqueIDs(batch []events.Click) []string {
	seen := make(map[string]struct{}, len(batch))
	ids := make([]string, 0, len(batch))
	for _, c := range batch {
		if _, ok := seen[c.ID]; ok {
			continue
		}
		seen[c.ID] = struct{}{}
		ids = append(ids, c.ID)
	}
	return ids
}

// Build aggregates the events whose ID is in fresh (i.e. not processed
// before). Each fresh ID is counted exactly once even if it appears several
// times in the batch.
func Build(batch []events.Click, fresh map[string]bool) Deltas {
	hourly := map[HourKey]int64{}
	refs := map[RefKey]int64{}
	totals := map[string]int64{}
	last := map[string]time.Time{}
	counted := make(map[string]bool, len(fresh))

	for _, c := range batch {
		if !fresh[c.ID] || counted[c.ID] {
			continue
		}
		counted[c.ID] = true
		at := c.At.UTC()
		hourly[HourKey{c.Slug, at.Truncate(time.Hour)}]++
		ref := c.Referrer
		if ref == "" {
			ref = "direct"
		}
		refs[RefKey{c.Slug, ref}]++
		totals[c.Slug]++
		if at.After(last[c.Slug]) {
			last[c.Slug] = at
		}
	}

	d := Deltas{LastAt: last}
	for k, n := range hourly {
		d.Hourly = append(d.Hourly, Count[HourKey]{k, n})
	}
	for k, n := range refs {
		d.Referrers = append(d.Referrers, Count[RefKey]{k, n})
	}
	for k, n := range totals {
		d.Totals = append(d.Totals, Count[string]{k, n})
	}
	sort.Slice(d.Hourly, func(i, j int) bool {
		a, b := d.Hourly[i].Key, d.Hourly[j].Key
		return a.Slug < b.Slug || (a.Slug == b.Slug && a.Hour.Before(b.Hour))
	})
	sort.Slice(d.Referrers, func(i, j int) bool {
		a, b := d.Referrers[i].Key, d.Referrers[j].Key
		return a.Slug < b.Slug || (a.Slug == b.Slug && a.Referrer < b.Referrer)
	})
	sort.Slice(d.Totals, func(i, j int) bool { return d.Totals[i].Key < d.Totals[j].Key })
	return d
}

// Empty reports whether there is nothing to apply.
func (d Deltas) Empty() bool { return len(d.Totals) == 0 }
