package viz

import (
	"math"
	"testing"

	"samuelks.com/dsp/analysis"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// builderSignal is a deterministic block of samples for the builder tests: a
// tone at bin 8 of 256 with a little offset, so every accumulator here has
// something to accumulate.
func builderSignal(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = 0.5*math.Cos(2*math.Pi*float64((8*i)%256)/256) + 0.01
	}
	return x
}

// closedStats closes a builder for a test that already wrote to it.
func closedStats(t *testing.T, b *analysis.StatsBuilder) *analysis.SignalStats {
	t.Helper()
	s, err := b.Close()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func statsOf(t *testing.T, x []float64, opt analysis.StatsOptions) *analysis.SignalStats {
	t.Helper()
	s, err := analysis.StatsOf(x, opt)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// fsk builds a two-tone keyed signal, alternating one symbol at a time.
func fsk(n int, rate, mark, space, baud float64) []float64 {
	x := make([]float64, n)
	per := int(math.Round(rate / baud))
	for i := range x {
		f := mark
		if (i/per)%2 == 1 {
			f = space
		}
		x[i] = 0.5 * math.Sin(2*math.Pi*f*float64(i)/rate)
	}
	return x
}
