package f64

import (
	"math"
	"testing"
)

// The f64 kernels mirror f32's; these check the contract rather than re-deriving
// the accuracy argument, which f32.TestSumBeatsSerial makes once.
func TestSumExactWhenNothingRounds(t *testing.T) {
	for _, n := range []int{0, 1, 7, 8, 9, 17, 64, 65, 1000} {
		src := make([]float64, n)
		var want float64
		for i := range src {
			src[i] = float64(i%17) - 8
			want += src[i]
		}
		if got := Sum(src); got != want {
			t.Errorf("n=%d: Sum = %v, want %v", n, got, want)
		}
	}
}

func TestMaxIdx(t *testing.T) {
	if v, i := MaxIdx(nil); !math.IsInf(v, -1) || i != -1 {
		t.Errorf("MaxIdx(nil) = (%v, %d), want (-Inf, -1)", v, i)
	}
	if v, i := MaxIdx([]float64{1, 7, 7, 3}); v != 7 || i != 1 {
		t.Errorf("tie = (%v, %d), want (7, 1)", v, i)
	}
	src := []float64{-5, 2, -9, 2}
	v, i := MaxIdx(src)
	if src[i] != v || v != Max(src) {
		t.Errorf("MaxIdx = (%v, %d); slice holds %v, Max gives %v", v, i, src[i], Max(src))
	}
}

func TestCMagSq(t *testing.T) {
	src := []complex128{complex(3, 4), complex(-1, 0), complex(0, 2)}
	dst := make([]float64, len(src)+1)
	CMagSq(dst, src)
	for i, want := range []float64{25, 1, 4} {
		if dst[i] != want {
			t.Errorf("[%d] = %v, want %v", i, dst[i], want)
		}
	}
	if dst[len(src)] != 0 {
		t.Errorf("wrote past the end: %v", dst)
	}
}
