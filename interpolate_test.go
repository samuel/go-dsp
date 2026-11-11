package dsp

import (
	"math"
	"testing"
)

var interpolators = []struct {
	name string
	f64  func([]float64, float64) float64
	f32  func([]float32, float32) float32
	// tol is the absolute error allowed on a signal spanning 0 to 70. InterpLinear
	// and Hermite reproduce a straight line exactly; the optimal 2x kernels are
	// least-squares fits meant for 2x-oversampled input, so they only come
	// close, and the 6-point one trades interpolation error for stopband
	// rejection more aggressively than the 4-point one.
	tol float64
}{
	{"Linear", InterpLinear[float64], InterpLinear[float32], 1e-12},
	{"Hermite4p3o", InterpHermite4p3o[float64], InterpHermite4p3o[float32], 1e-9},
	{"Optimal2x4p4o", InterpOptimal2x4p4o[float64], InterpOptimal2x4p4o[float32], 0.02},
	{"Optimal2x6p5o", InterpOptimal2x6p5o[float64], InterpOptimal2x6p5o[float32], 0.25},
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
// it, including a negative x, where the index and the fraction must both floor.
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

// TestInterpolateAtIntegerX checks that InterpLinear and InterpHermite4p3o agree with each
// other, and with the samples themselves, at integer positions, at both widths.
// The optimal 2x kernels are excluded: they are designed for 2x-oversampled
// input and do not pass through the sample values.
func TestInterpolateAtIntegerX(t *testing.T) {
	s := []float64{3, -1, 4, -1, 5, -9, 2, 6}
	s32 := toF32(s)
	for i, want := range s {
		x := float64(i)
		if got := InterpLinear(s, x); got != want {
			t.Errorf("Linear(%v) = %v, want %v", x, got, want)
		}
		if got := InterpHermite4p3o(s, x); math.Abs(got-want) > 1e-12 {
			t.Errorf("Hermite4p3o(%v) = %v, want %v", x, got, want)
		}
		if got := InterpLinear(s32, float32(x)); got != float32(want) {
			t.Errorf("Linear[float32](%v) = %v, want %v", x, got, want)
		}
		if got := InterpHermite4p3o(s32, float32(x)); math.Abs(float64(got-float32(want))) > 1e-5 {
			t.Errorf("Hermite4p3o[float32](%v) = %v, want %v", x, got, want)
		}
	}
}

// TestInterpolateSymmetricAboutNegativeX is the regression test for the index
// bug: int(x) truncates toward zero while math.Floor rounds down, so for a
// negative x the sample window and the fraction disagreed by one and
// InterpHermite4p3o(s, -0.5) returned the value belonging to +0.5.
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

// The reference sinc tables below spell out the table-building definition
// SincInterpolator has to reproduce bit for bit, with its parameters as
// constants.
const (
	refSincTaps   = 16
	refSincHalf   = 8
	refSincPhases = 512
	refSincBeta   = 6.0
	refSincCutoff = 0.454
)

// refSincTables returns each phase's coefficients before rounding to the
// sample width, and the window behind them: a sinc and a Kaiser window both
// centered on the phase's position, the window reaching refSincHalf either
// side of it.
func refSincTables() (table, window [refSincPhases][refSincTaps]float64) {
	for p := range refSincPhases {
		alpha := float64(p) / float64(refSincPhases)
		var sum float64
		var taps [refSincTaps]float64
		for k := range refSincTaps {
			t := float64(k) - float64(refSincHalf-1) - alpha
			window[p][k] = kaiserAt(t, refSincHalf, refSincBeta)
			taps[k] = sinc(2*refSincCutoff*t) * window[p][k]
			sum += taps[k]
		}
		for k, v := range taps {
			table[p][k] = v / sum
		}
	}
	return table, window
}

func refSincDownCoeffs(window *[refSincPhases][refSincTaps]float64, phase int, fc float64) (out [refSincTaps]float64) {
	w := 2 * math.Pi * fc
	t := -float64(refSincHalf-1) - float64(phase)/float64(refSincPhases)
	sin, cos := math.Sincos(w * t)
	stepSin, stepCos := math.Sincos(w)
	var taps [refSincTaps]float64
	var sum float64
	for k := range refSincTaps {
		v := 1.0
		if wt := w * t; math.Abs(wt) > 1e-9 {
			v = sin / wt
		}
		taps[k] = v * window[phase][k]
		sum += taps[k]
		sin, cos = sin*stepCos+cos*stepSin, cos*stepCos-sin*stepSin
		t++
	}
	for k, v := range taps {
		out[k] = v / sum
	}
	return out
}

func testSincInterpolatorMatchesReference[T Float](t *testing.T) {
	s, err := NewSincInterpolator[T](refSincTaps, refSincPhases, refSincCutoff, refSincBeta)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != refSincTaps || s.Phases() != refSincPhases {
		t.Fatalf("Len %d, Phases %d", s.Len(), s.Phases())
	}
	same := func(got T, want float64) bool {
		return math.Float64bits(float64(got)) == math.Float64bits(float64(T(want)))
	}
	table, window := refSincTables()
	for p := range refSincPhases {
		got := s.PhaseCoeffs(p)
		for k, want := range table[p] {
			if !same(got[k], want) {
				t.Fatalf("phase %d tap %d = %v, want %v", p, k, got[k], T(want))
			}
		}
	}
	got := make([]T, refSincTaps)
	for _, fc := range []float64{refSincCutoff / 1.7, refSincCutoff / 3.3} {
		for _, p := range []int{0, 1, 100, 255, 256, 511} {
			want := refSincDownCoeffs(&window, p, fc)
			s.DownsampleCoeffs(got, p, fc)
			for k := range want {
				if !same(got[k], want[k]) {
					t.Fatalf("cutoff %v phase %d tap %d = %v, want %v", fc, p, k, got[k], T(want[k]))
				}
			}
		}
	}
}

func TestSincInterpolatorMatchesReference(t *testing.T) {
	t.Run("float32", testSincInterpolatorMatchesReference[float32])
	t.Run("float64", testSincInterpolatorMatchesReference[float64])
}

func TestNewSincInterpolatorErrors(t *testing.T) {
	for _, c := range []struct {
		taps, phases int
		cutoff, beta float64
	}{
		{0, 8, 0.45, 6},
		{15, 8, 0.45, 6},
		{16, 0, 0.45, 6},
		{16, 8, 0, 6},
		{16, 8, 0.51, 6},
		{16, 8, 0.45, -1},
		{16, 8, 0.45, math.NaN()},
		{16, 8, 0.45, math.Inf(1)},
		{16, 8, 0.45, 1e4},
	} {
		if _, err := NewSincInterpolator[float32](c.taps, c.phases, c.cutoff, c.beta); err == nil {
			t.Errorf("NewSincInterpolator[float32](%d, %d, %v, %v): no error", c.taps, c.phases, c.cutoff, c.beta)
		}
		if _, err := NewSincInterpolator[float64](c.taps, c.phases, c.cutoff, c.beta); err == nil {
			t.Errorf("NewSincInterpolator[float64](%d, %d, %v, %v): no error", c.taps, c.phases, c.cutoff, c.beta)
		}
	}
}

func TestSincInterpolatorPhaseIndex(t *testing.T) {
	s, err := NewSincInterpolator[float32](8, 64, 0.45, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		frac float64
		want int
	}{
		{0, 0}, {0.5, 32}, {1.0 / 64, 1}, {0.999999, 63}, {1, 63}, {5, 63},
		{-0.1, 0}, {math.NaN(), 0}, {math.Inf(1), 63},
	} {
		if got := s.PhaseIndex(c.frac); got != c.want {
			t.Errorf("PhaseIndex(%v) = %d, want %d", c.frac, got, c.want)
		}
	}
}

func TestSincInterpolatorDownsampleCoeffsPanics(t *testing.T) {
	s, err := NewSincInterpolator[float32](8, 4, 0.45, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		n      int
		cutoff float64
	}{{7, 0.2}, {8, 0}, {8, 0.6}, {8, math.NaN()}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("len %d cutoff %v: no panic", c.n, c.cutoff)
				}
			}()
			s.DownsampleCoeffs(make([]float32, c.n), 0, c.cutoff)
		}()
	}
}

func testSincInterpolatorBehavior[T Float](t *testing.T) {
	s, err := NewSincInterpolator[T](refSincTaps, refSincPhases, refSincCutoff, refSincBeta)
	if err != nil {
		t.Fatal(err)
	}
	down := make([]T, s.Len())
	for p := range s.Phases() {
		var sum float64
		for _, c := range s.PhaseCoeffs(p) {
			sum += float64(c)
		}
		if math.Abs(sum-1) > 1e-6 {
			t.Fatalf("phase %d: DC gain %v", p, sum)
		}
		s.DownsampleCoeffs(down, p, refSincCutoff/2.5)
		sum = 0
		for _, c := range down {
			sum += float64(c)
		}
		if math.Abs(sum-1) > 1e-6 {
			t.Fatalf("phase %d: downsampling DC gain %v", p, sum)
		}
	}

	// Phase zero is centered on the base sample, so it is symmetric about it,
	// and its last tap sits on the window's edge rather than outside it.
	c0 := s.PhaseCoeffs(0)
	base := s.Len()/2 - 1
	for k := 1; k <= base; k++ {
		if c0[base-k] != c0[base+k] {
			t.Errorf("phase 0 taps %d and %d differ: %v, %v", base-k, base+k, c0[base-k], c0[base+k])
		}
	}
	if c0[s.Len()-1] == 0 {
		t.Error("phase 0 last tap is zero")
	}

	// A slow sine is reproduced between its samples, to within the passband
	// ripple of 16 windowed taps, and At agrees with Interpolate exactly away
	// from the edges.
	const f = 0.03
	src := make([]T, 64)
	for i := range src {
		src[i] = T(math.Sin(2 * math.Pi * f * float64(i)))
	}
	for n := 20; n < 40; n++ {
		for _, frac := range []float64{0, 0.125, 0.5, 0.8} {
			got := s.Interpolate(src[n-base:n-base+s.Len()], frac)
			want := math.Sin(2 * math.Pi * f * (float64(n) + frac))
			if d := math.Abs(float64(got) - want); d > 1e-3 {
				t.Fatalf("n=%d frac=%v: %v, want %v", n, frac, got, want)
			}
			if at := s.At(src, float64(n)+frac); at != got {
				t.Fatalf("n=%d frac=%v: At %v, Interpolate %v", n, frac, at, got)
			}
		}
	}

	// At holds the edges, so a constant signal stays constant everywhere.
	flat := []T{0.75, 0.75, 0.75}
	for _, x := range []float64{-50, -1, -0.5, 0, 0.3, 1.7, 2, 2.5, 3, 100} {
		if got := s.At(flat, x); math.Abs(float64(got)-0.75) > 1e-6 {
			t.Errorf("At(flat, %v) = %v, want 0.75", x, got)
		}
	}
	if got := s.At([]T(nil), 1); got != 0 {
		t.Errorf("At(nil) = %v, want 0", got)
	}
	if got := s.At(flat, math.NaN()); got != 0 {
		t.Errorf("At(NaN) = %v, want 0", got)
	}
}

func TestSincInterpolatorBehavior(t *testing.T) {
	t.Run("float32", testSincInterpolatorBehavior[float32])
	t.Run("float64", testSincInterpolatorBehavior[float64])
}

// testSincInterpolatorComplex checks that one table reads a complex signal's
// real and imaginary parts exactly as it reads each of them alone.
func testSincInterpolatorComplex[C Complex, F Float](t *testing.T) {
	sf, err := NewSincInterpolator[F](8, 32, 0.45, 6)
	if err != nil {
		t.Fatal(err)
	}
	re := make([]F, 40)
	im := make([]F, 40)
	src := make([]C, 40)
	for i := range src {
		re[i] = F(math.Sin(0.2 * float64(i)))
		im[i] = F(math.Cos(0.7 * float64(i)))
		src[i] = C(complex(float64(re[i]), float64(im[i])))
	}
	for _, x := range []float64{-2, 0, 3.25, 17.5, 30.9, 39, 45} {
		got := complex128(sf.At(src, x))
		wantRe, wantIm := float64(sf.At(re, x)), float64(sf.At(im, x))
		if real(got) != wantRe || imag(got) != wantIm {
			t.Errorf("x=%v: %v, want (%v%+vi)", x, got, wantRe, wantIm)
		}
	}
}

// TestSincInterpolatorMixedWidths reads float32 samples through a float64
// table and the reverse. The sum accumulates at the sample's width, so the
// result is the sample-width table's answer to within that width's rounding.
func TestSincInterpolatorMixedWidths(t *testing.T) {
	s32 := must(NewSincInterpolator[float32](8, 32, 0.45, 6))
	s64 := must(NewSincInterpolator[float64](8, 32, 0.45, 6))
	src32 := make([]float32, 40)
	src64 := make([]float64, 40)
	c64 := make([]complex64, 40)
	for i := range src64 {
		src64[i] = math.Sin(0.2 * float64(i))
		src32[i] = float32(src64[i])
		c64[i] = complex(src32[i], -src32[i])
	}
	for _, x := range []float64{-2, 0, 3.25, 17.5, 30.9, 39, 45} {
		if got, want := s64.At(src32, x), s32.At(src32, x); math.Abs(float64(got-want)) > 1e-6 {
			t.Errorf("float32 samples, float64 table, x=%v: %v, want %v", x, got, want)
		}
		if got, want := s32.At(src64, x), s64.At(src64, x); math.Abs(got-want) > 1e-6 {
			t.Errorf("float64 samples, float32 table, x=%v: %v, want %v", x, got, want)
		}
		if got, want := s64.At(c64, x), s64.At(src32, x); real(got) != want || imag(got) != -want {
			t.Errorf("complex64 samples, float64 table, x=%v: %v, want %v%+vi", x, got, want, -want)
		}
	}
}

func TestSincInterpolatorComplex(t *testing.T) {
	t.Run("complex64", testSincInterpolatorComplex[complex64, float32])
	t.Run("complex128", testSincInterpolatorComplex[complex128, float64])
}
