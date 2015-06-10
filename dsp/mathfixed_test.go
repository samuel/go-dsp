package dsp

import (
	"math"
	"testing"
)

func TestFastAtan2FixedError(t *testing.T) {
	maxE := 0.0
	sumE := 0.0
	count := 0
	for y := -32768; y < 32768; y += 64 {
		for x := -32768; x < 32768; x += 64 {
			ai := float64(FastAtan2Fixed(y, x)) * math.Pi / (1 << 14)
			af := math.Atan2(float64(y), float64(x))
			e := math.Abs(ai - af)
			sumE += e
			if e > maxE {
				maxE = e
			}
			count++
		}
	}
	if maxE > 0.08 {
		t.Errorf("Expected max error of 0.08 got %f", maxE)
	}
	t.Logf("Max error %f\n", maxE)
	t.Logf("Mean absolute error %f", sumE/float64(count))
}

func TestAtan2LUTError(t *testing.T) {
	maxE := 0.0
	sumE := 0.0
	count := 0
	for y := -32768; y < 32768; y += 64 {
		for x := -32768; x < 32768; x += 64 {
			ai := float64(Atan2LUT(y, x)) * math.Pi / (1 << 14)
			af := math.Atan2(float64(y), float64(x))
			e := math.Abs(ai - af)
			sumE += e
			if e > maxE {
				maxE = e
			}
			count++
		}
	}
	if maxE > 0.005 {
		t.Errorf("Expected max error of 0.005 got %f", maxE)
	}
	t.Logf("Max error %f\n", maxE)
	t.Logf("Mean absolute error %f", sumE/float64(count))
}

func BenchmarkFastAtan2Fixed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, xy := range atanBenchTableFixed {
			FastAtan2Fixed(xy[1], xy[0])
		}
	}
}

func BenchmarkAtan2LUT(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, xy := range atanBenchTableFixed {
			Atan2LUT(xy[1], xy[0])
		}
	}
}
