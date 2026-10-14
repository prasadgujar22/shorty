package memlimit

import "testing"

func TestCompute(t *testing.T) {
	cases := []struct {
		limit int64
		ratio float64
		want  int64
	}{
		{1000, 0.9, 900},
		{1000, 0, 900},
		{1000, 1.5, 900},
		{1000, 1, 1000},
		{128 << 20, 0.8, 107374182},
	}
	for _, c := range cases {
		if got := Compute(c.limit, c.ratio); got != c.want {
			t.Errorf("Compute(%d, %v) = %d, want %d", c.limit, c.ratio, got, c.want)
		}
	}
}
