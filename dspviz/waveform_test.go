package dspviz

import (
	"math"
	"math/rand/v2"
	"testing"
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
	xs, mins, maxs := Envelope(x, n)
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

func TestDecimateEdgeCases(t *testing.T) {
	if xs, _, _ := Envelope(nil, 4); xs != nil {
		t.Error("an empty signal should decimate to nothing")
	}
	if xs, _, _ := Envelope([]float64{1, 2}, 0); xs != nil {
		t.Error("zero columns should decimate to nothing")
	}
	// More columns than samples gives one column per sample rather than an
	// empty one in the gaps.
	xs, mins, maxs := Envelope([]float64{1, 2, 3}, 10)
	if len(xs) != 3 {
		t.Fatalf("got %d columns, want 3", len(xs))
	}
	for i := range xs {
		if mins[i] != float64(i+1) || maxs[i] != float64(i+1) {
			t.Errorf("column %d is %v..%v, want %v", i, mins[i], maxs[i], i+1)
		}
	}
}

// TestWaveformStreamingKeepsEveryPeak checks the reducer the same way the
// spectrogram's is checked: the envelope of a bounded number of columns still
// holds the largest and smallest sample in the whole signal.
func TestWaveformStreamingKeepsEveryPeak(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 7))
	x := make([]float64, 200000)
	for i := range x {
		x[i] = 0.1 * r.NormFloat64()
	}
	x[137] = -0.97
	x[190123] = 0.93

	for _, block := range []int{1, 3, 999, 65536} {
		b, err := NewWaveformBuilder(48000, WaveformOptions{Columns: 64})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < len(x); i += block {
			if err := b.Write(x[i:min(i+block, len(x))]); err != nil {
				t.Fatal(err)
			}
		}
		w, err := b.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(w.Max) > 64 || len(w.Max) < 32 {
			t.Errorf("block %d: %d columns, want between 32 and 64", block, len(w.Max))
		}
		if got := maxOf(w.Max); got != 0.93 {
			t.Errorf("block %d: peak %v, want 0.93", block, got)
		}
		if got := minOf(w.Min); got != -0.97 {
			t.Errorf("block %d: trough %v, want -0.97", block, got)
		}
		if w.Samples != int64(len(x)) {
			t.Errorf("block %d: counted %d samples, want %d", block, w.Samples, len(x))
		}
	}
}

func TestWaveformComplexIsAnEnvelope(t *testing.T) {
	z := make([]complex128, 1000)
	for i := range z {
		th := 2 * math.Pi * float64(i) / 50
		z[i] = complex(0.5*math.Cos(th), 0.5*math.Sin(th))
	}
	b, err := NewWaveformBuilder(8000, WaveformOptions{Columns: 16})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.WriteComplex(z); err != nil {
		t.Fatal(err)
	}
	w, err := b.Close()
	if err != nil {
		t.Fatal(err)
	}
	// A constant-envelope signal draws as a band of constant width about zero.
	for i := range w.Max {
		if math.Abs(w.Max[i]-0.5) > 1e-12 || math.Abs(w.Min[i]+0.5) > 1e-12 {
			t.Fatalf("column %d is %v..%v, want -0.5..0.5", i, w.Min[i], w.Max[i])
		}
	}
}
