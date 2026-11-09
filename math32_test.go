package dsp

import (
	"math"
	"testing"
)

const (
	approxErrorLimit = 0.011
)

var atanBenchTable = [][2]float32{}

func init() {
	for y := -1.0; y <= 1.0; y += 0.5 {
		for x := -1.0; x <= 1.0; x += 0.5 {
			atanBenchTable = append(atanBenchTable, [2]float32{float32(x), float32(y)})
		}
	}
}

func TestAtan2(t *testing.T) {
	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			expected := float32(math.Atan2(y, x))
			if err := math.Abs(float64(expected - FastAtan2(float32(y), float32(x)))); err > approxErrorLimit {
				t.Errorf("FastAtan2 gave an error of %f for x=%f y=%f", err, x, y)
			}
			if err := math.Abs(float64(expected - FastAtan2Fine(float32(y), float32(x)))); err > approxErrorLimit {
				t.Errorf("FastAtan2Fine gave an error of %f for x=%f y=%f", err, x, y)
			}
		}
	}
	x, y := 0.0, 0.0
	expected := float32(math.Atan2(y, x))
	if err := math.Abs(float64(expected - FastAtan2(float32(y), float32(x)))); err > approxErrorLimit {
		t.Errorf("FastAtan2 gave an error of %f for x=%f y=%f", err, x, y)
	}
	if err := math.Abs(float64(expected - FastAtan2Fine(float32(y), float32(x)))); err > approxErrorLimit {
		t.Errorf("FastAtan2Fine gave an error of %f for x=%f y=%f", err, x, y)
	}
}

func TestFastAtan2Error(t *testing.T) {
	maxE := 0.0
	sumE := 0.0
	count := 0
	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			ai := float64(FastAtan2(float32(y), float32(x)))
			af := math.Atan2(y, x)
			e := math.Abs(ai - af)
			sumE += e
			if e > maxE {
				maxE = e
			}
			count++
		}
	}
	if maxE > 0.0102 {
		t.Errorf("Expected max error of 0.0102 got %f", maxE)
	}
	t.Logf("Max error %f\n", maxE)
	t.Logf("Mean absolute error %f", sumE/float64(count))
}

func TestFastAtan2FineError(t *testing.T) {
	maxE := 0.0
	sumE := 0.0
	count := 0
	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			ai := float64(FastAtan2Fine(float32(y), float32(x)))
			af := math.Atan2(y, x)
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

func BenchmarkFastAtan2_Go(b *testing.B) {
	for range b.N {
		for _, xy := range atanBenchTable {
			fastAtan2(xy[1], xy[0])
		}
	}
}

func BenchmarkFastAtan2Fine(b *testing.B) {
	for range b.N {
		for _, xy := range atanBenchTable {
			FastAtan2Fine(xy[1], xy[0])
		}
	}
}

func BenchmarkFastAtan2Fine_Go(b *testing.B) {
	for range b.N {
		for _, xy := range atanBenchTable {
			fastAtan2Fine(xy[1], xy[0])
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
