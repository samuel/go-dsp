package analysis

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

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
		if got := slices.Max(w.Max); got != 0.93 {
			t.Errorf("block %d: peak %v, want 0.93", block, got)
		}
		if got := slices.Min(w.Min); got != -0.97 {
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
