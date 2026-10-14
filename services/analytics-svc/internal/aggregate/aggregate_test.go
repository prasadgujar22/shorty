package aggregate

import (
	"testing"
	"time"

	"github.com/prasadgujar22/shorty/pkg/events"
)

func TestBuildCountsEachFreshEventOnce(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 10, 15, 0, 0, time.UTC)
	batch := []events.Click{
		{ID: "1", Slug: "b", At: t0, Referrer: "x.com"},
		{ID: "2", Slug: "a", At: t0.Add(time.Minute), Referrer: ""},
		{ID: "2", Slug: "a", At: t0.Add(time.Minute), Referrer: ""}, // redelivered within the batch
		{ID: "3", Slug: "a", At: t0.Add(time.Hour), Referrer: "x.com"},
		{ID: "4", Slug: "a", At: t0, Referrer: "old.com"}, // already processed earlier
	}
	if ids := UniqueIDs(batch); len(ids) != 4 {
		t.Fatalf("unique ids = %v", ids)
	}
	d := Build(batch, map[string]bool{"1": true, "2": true, "3": true})

	if len(d.Totals) != 2 || d.Totals[0] != (Count[string]{"a", 2}) || d.Totals[1] != (Count[string]{"b", 1}) {
		t.Fatalf("totals = %+v", d.Totals)
	}
	wantHourly := []Count[HourKey]{
		{HourKey{"a", time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)}, 1},
		{HourKey{"a", time.Date(2026, 10, 14, 11, 0, 0, 0, time.UTC)}, 1},
		{HourKey{"b", time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)}, 1},
	}
	for i, w := range wantHourly {
		if d.Hourly[i].N != w.N || d.Hourly[i].Key.Slug != w.Key.Slug || !d.Hourly[i].Key.Hour.Equal(w.Key.Hour) {
			t.Fatalf("hourly[%d] = %+v, want %+v", i, d.Hourly[i], w)
		}
	}
	wantRefs := []Count[RefKey]{{RefKey{"a", "direct"}, 1}, {RefKey{"a", "x.com"}, 1}, {RefKey{"b", "x.com"}, 1}}
	for i, w := range wantRefs {
		if d.Referrers[i] != w {
			t.Fatalf("refs = %+v", d.Referrers)
		}
	}
	if !d.LastAt["a"].Equal(t0.Add(time.Hour)) {
		t.Fatalf("last = %v", d.LastAt)
	}
}

func TestBuildNothingFresh(t *testing.T) {
	d := Build([]events.Click{{ID: "1", Slug: "a", At: time.Now()}}, map[string]bool{})
	if !d.Empty() {
		t.Fatal("expected empty deltas")
	}
}
