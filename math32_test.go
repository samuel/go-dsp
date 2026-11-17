package dsp

import (
	"math"
	"testing"
)

var atanBenchTable = [][2]float32{}

func init() {
	for y := -1.0; y <= 1.0; y += 0.5 {
		for x := -1.0; x <= 1.0; x += 0.5 {
			atanBenchTable = append(atanBenchTable, [2]float32{float32(x), float32(y)})
		}
	}
}

// TestFastAtan2Error holds FastAtan2 to its documented bounds around the whole
// circle and at several magnitudes, and holds the relative error at angles
// down to 1e-6, where an FM discriminator's phase steps are.
func TestFastAtan2Error(t *testing.T) {
	var maxAbs, maxRel float64
	for i := range 100000 {
		th := -math.Pi + 2*math.Pi*(float64(i)+0.5)/100000
		s, c := math.Sincos(th)
		for _, r := range []float64{1e-6, 1, 1e6} {
			got := float64(FastAtan2(float32(r*s), float32(r*c)))
			e := math.Abs(got - th)
			maxAbs = max(maxAbs, e)
			if math.Abs(th) <= math.Pi/2 {
				maxRel = max(maxRel, e/math.Abs(th))
			}
		}
	}
	for th := 1e-6; th < 0.5; th *= 1.1 {
		s, c := math.Sincos(th)
		got := float64(FastAtan2(float32(s), float32(c)))
		maxRel = max(maxRel, math.Abs(got-th)/th)
	}
	t.Logf("max absolute error %.3g, max relative error %.3g", maxAbs, maxRel)
	if maxAbs > 1.7e-4 {
		t.Errorf("max absolute error %g, want at most 1.7e-4", maxAbs)
	}
	if maxRel > 2.2e-4 {
		t.Errorf("max relative error %g, want at most 2.2e-4", maxRel)
	}
}

func BenchmarkConj(b *testing.B) {
	in := complex64(complex(1.0, -0.2))
	for range b.N {
		_ = Conj(in)
	}
}

func BenchmarkFastAtan2(b *testing.B) {
	for range b.N {
		for _, xy := range atanBenchTable {
			FastAtan2(xy[1], xy[0])
		}
	}
}

func BenchmarkAtan2(b *testing.B) {
	for range b.N {
		for _, xy := range atanBenchTable {
			math.Atan2(float64(xy[1]), float64(xy[0]))
		}
	}
}
