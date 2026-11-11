package plot

import (
	"math"
	"math/rand/v2"
	"testing"

	"samuelks.com/dsp/analysis"
)

// TestDecimateKeepsTheExtremes is the point of the function: a mean-based
// decimator passes every plausible test and loses the one-sample transient a
// waveform overview exists to show, so the true minimum and maximum of each
// column are asserted directly.
func TestDecimateKeepsTheExtremes(t *testing.T) {
	r := rand.New(rand.NewPCG(2, 3))
	x := make([]float64, 1000)
	for i := range x {
		x[i] = r.NormFloat64()
	}
	// A spike one sample wide, which a mean would bury under 99 neighbors.
	x[543] = 12

	const n = 10
	xs, mins, maxs := analysis.Envelope(x, n)
	if len(xs) != n || len(mins) != n || len(maxs) != n {
		t.Fatalf("got %d/%d/%d columns, want %d each", len(xs), len(mins), len(maxs), n)
	}
	for i := range n {
		lo, hi := len(x)*i/n, len(x)*(i+1)/n
		wantMin, wantMax := x[lo], x[lo]
		for _, v := range x[lo:hi] {
			wantMin, wantMax = math.Min(wantMin, v), math.Max(wantMax, v)
		}
		if mins[i] != wantMin || maxs[i] != wantMax {
			t.Errorf("column %d: got %v..%v, want %v..%v", i, mins[i], maxs[i], wantMin, wantMax)
		}
	}
	if maxs[5] != 12 {
		t.Errorf("the spike is gone: column 5 peaks at %v, want 12", maxs[5])
	}

	// The zigzag one series is drawn as has two points per column, so a single
	// path covers the whole band the samples occupied.
	zx, zy := zigzag(xs, mins, maxs)
	if len(zx) != 2*n || len(zy) != 2*n {
		t.Errorf("the zigzag has %d points, want %d", len(zx), 2*n)
	}
}
