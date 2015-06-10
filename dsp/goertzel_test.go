package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestGoertzel(t *testing.T) {
	samplerate := 1024
	blocksize := 1024
	freq := 128
	samples := make([]float64, blocksize)
	w := 2 * math.Pi / float64(samplerate)
	for i := 0; i < blocksize; i++ {
		samples[i] = math.Sin(float64(i) * float64(freq) * w)
	}
	g := NewGoertzel([]uint64{128, 129}, samplerate, blocksize)
	g.Feed(samples)
	m := g.Magnitude()
	if e := math.Pow(float64(blocksize)/2, 2); !approxEqual(m[0], e, 1e-8) {
		t.Errorf("Goertzel magnitude = %f. Want %f", m[0], e)
	}
	if !approxEqual(float64(m[1]), 0.0, 1e-10) {
		t.Errorf("Foertzel magnitude = %f. Want 0.0", m[1])
	}
	c := g.Complex()
	if e, m := math.Sqrt(math.Pow(float64(blocksize)/2, 2)), cmplx.Abs(complex128(c[0])); !approxEqual(m, e, 1e-8) {
		t.Errorf("Goertzel magnitude = %f. Want %f", m, e)
	}
	if e, p := -math.Pi/2, cmplx.Phase(complex128(c[0])); !approxEqual(p, e, 1e-12) {
		t.Errorf("Goertzel phase = %f. Want %f", p, e)
	}
}
