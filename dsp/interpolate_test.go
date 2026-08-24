package dsp

import (
	"math"
	"testing"
)

var interpolators = []struct {
	name string
	f64  func([]float64, float64) float64
	f32  func([]float32, float32) float32
	// tol is the absolute error allowed on a signal spanning 0 to 70. Linear
	// and Hermite reproduce a straight line exactly; the optimal 2x kernels are
	// least-squares fits meant for 2x-oversampled input, so they only come
	// close, and the 6-point one trades interpolation error for stopband
	// rejection more aggressively than the 4-point one.
	tol float64
}{
	{"Linear", Linear[float64], Linear[float32], 1e-12},
	{"Hermite4p3o", Hermite4p3o[float64], Hermite4p3o[float32], 1e-9},
	{"Optimal2x4p4o", Optimal2x4p4o[float64], Optimal2x4p4o[float32], 0.02},
	{"Optimal2x6p5o", Optimal2x6p5o[float64], Optimal2x6p5o[float32], 0.25},
}

func ramp(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = float64(i) * 10
	}
	return s
}

func toF32(s []float64) []float32 {
	o := make([]float32, len(s))
	for i, v := range s {
		o[i] = float32(v)
	}
	return o
}

// TestInterpolateFinite checks that no interpolator returns NaN or Inf, and
// that none of them panics, anywhere from well before the buffer to well after
// it. Linear used to panic for a negative x while LinearF32 returned zero, and
// the other six read the wrong sample for a negative x because they truncated
// toward zero for the index but floored for the fraction.
func TestInterpolateFinite(t *testing.T) {
	for _, in := range interpolators {
		t.Run(in.name, func(t *testing.T) {
			for _, n := range []int{0, 1, 2, 3, 8} {
				s := ramp(n)
				s32 := toF32(s)
				for x := -8.0; x <= 16.0; x += 0.25 {
					got := in.f64(s, x)
					if math.IsNaN(got) || math.IsInf(got, 0) {
						t.Fatalf("n=%d x=%v: %v", n, x, got)
					}
					got32 := in.f32(s32, float32(x))
					if math.Abs(float64(got32)-got) > 1e-4*(1+math.Abs(got)) {
						t.Fatalf("n=%d x=%v: float32 %v, float64 %v", n, x, got32, got)
					}
				}
			}
		})
	}
}

// TestInterpolateNonFiniteX pins the answer for an x that cannot be converted
// to an index. Converting a non-finite float to an int is
// implementation-defined in Go, so these have to be intercepted.
func TestInterpolateNonFiniteX(t *testing.T) {
	s := ramp(8)
	for _, in := range interpolators {
		t.Run(in.name, func(t *testing.T) {
			if got := in.f64(s, math.NaN()); got != 0 {
				t.Errorf("NaN gave %v, want 0", got)
			}
			// The infinities are out of range at both ends, so they hold an
			// edge sample rather than giving zero.
			for _, x := range []float64{math.Inf(-1), math.Inf(1)} {
				got := in.f64(s, x)
				if math.IsNaN(got) || math.IsInf(got, 0) {
					t.Errorf("x=%v gave %v", x, got)
				}
			}
		})
	}
}

// TestInterpolateEdgeHold checks the shared out-of-range policy: a position
// past either end holds the nearest sample rather than fading to zero, which
// would put a step discontinuity at both ends of every buffer.
func TestInterpolateEdgeHold(t *testing.T) {
	s := ramp(8)
	for _, in := range interpolators {
		t.Run(in.name, func(t *testing.T) {
			for _, tc := range []struct {
				x    float64
				want float64
			}{
				{-4, s[0]},
				{-1, s[0]},
				{11, s[len(s)-1]},
				{20, s[len(s)-1]},
			} {
				if got := in.f64(s, tc.x); math.Abs(got-tc.want) > in.tol {
					t.Errorf("x=%v gave %v, want %v", tc.x, got, tc.want)
				}
			}
		})
	}
}

// TestInterpolateAtIntegerX checks that Linear and Hermite4p3o agree with each
// other, and with the samples themselves, at integer positions, at both widths.
// The optimal 2x kernels are excluded: they are designed for 2x-oversampled
// input and do not pass through the sample values.
func TestInterpolateAtIntegerX(t *testing.T) {
	s := []float64{3, -1, 4, -1, 5, -9, 2, 6}
	s32 := toF32(s)
	for i, want := range s {
		x := float64(i)
		if got := Linear(s, x); got != want {
			t.Errorf("Linear(%v) = %v, want %v", x, got, want)
		}
		if got := Hermite4p3o(s, x); math.Abs(got-want) > 1e-12 {
			t.Errorf("Hermite4p3o(%v) = %v, want %v", x, got, want)
		}
		if got := Linear(s32, float32(x)); got != float32(want) {
			t.Errorf("Linear[float32](%v) = %v, want %v", x, got, want)
		}
		if got := Hermite4p3o(s32, float32(x)); math.Abs(float64(got-float32(want))) > 1e-5 {
			t.Errorf("Hermite4p3o[float32](%v) = %v, want %v", x, got, want)
		}
	}
}

// TestInterpolateSymmetricAboutNegativeX is the regression test for the index
// bug: int(x) truncates toward zero while math.Floor rounds down, so for a
// negative x the sample window and the fraction disagreed by one and
// Hermite4p3o(s, -0.5) returned the value belonging to +0.5.
func TestInterpolateSymmetricAboutNegativeX(t *testing.T) {
	s := []float64{0, 10, 20, 30}
	for _, in := range interpolators {
		t.Run(in.name, func(t *testing.T) {
			atHalf := in.f64(s, 0.5)
			atMinusHalf := in.f64(s, -0.5)
			if atMinusHalf == atHalf {
				t.Errorf("x=-0.5 and x=0.5 both gave %v", atHalf)
			}
			// -0.5 sits between the held edge and the first sample, so it has
			// to stay near samples[0] rather than jump to the middle of the
			// buffer.
			if math.Abs(atMinusHalf-s[0]) > 5 {
				t.Errorf("x=-0.5 gave %v, expected something near %v", atMinusHalf, s[0])
			}
		})
	}
}

// TestInterpolateExactHalfway pins the interior values of each kernel, so that
// a change to the coefficients or to the fraction shows up.
func TestInterpolateExactHalfway(t *testing.T) {
	s := ramp(8) // a straight line, which every kernel tracks to within its tol
	for _, in := range interpolators {
		t.Run(in.name, func(t *testing.T) {
			// Far enough from both ends that no tap is clamped.
			for _, x := range []float64{3, 3.25, 3.5, 3.75, 4} {
				want := x * 10
				if got := in.f64(s, x); math.Abs(got-want) > in.tol {
					t.Errorf("x=%v gave %v, want %v", x, got, want)
				}
			}
		})
	}
}
