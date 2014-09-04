package sdr

import (
	"math"
	"math/cmplx"
	"testing"
)

func approxEqual(a, b, e float64) bool {
	return math.Abs(a-b) <= e
}

func TestGoertzel(t *testing.T) {
	samplerate := 1024
	blocksize := 1024
	freq := 128
	samples := make([]float32, blocksize)
	w := 2 * math.Pi / float64(samplerate)
	for i := 0; i < blocksize; i++ {
		samples[i] = float32(math.Sin(float64(i) * float64(freq) * w))
	}
	g := NewGoertzel([]int{128, 129}, samplerate, blocksize)
	g.Feed(samples)
	m := g.Magnitude()
	if e := math.Pow(float64(blocksize)/2, 2); float64(m[0]) != e {
		t.Errorf("Goertzel magnitude = %f. Want %f", m[0], e)
	}
	if !approxEqual(float64(m[1]), 0.0, 1e-10) {
		t.Errorf("Foertzel magnitude = %f. Want 0.0", m[1])
	}
	c := g.Complex()
	if e, m := math.Sqrt(math.Pow(float64(blocksize)/2, 2)), cmplx.Abs(complex128(c[0])); m != e {
		t.Errorf("Goertzel magnitude = %f. Want %f", m, e)
	}
	if e, p := -math.Pi/2, cmplx.Phase(complex128(c[0])); !approxEqual(p, e, 1e-12) {
		t.Errorf("Goertzel phase = %f. Want %f", p, e)
	}
}
