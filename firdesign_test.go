package dsp

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestKaiserAt(t *testing.T) {
	// The sampled window agrees with the pointwise one at its points.
	for _, beta := range []float64{0, 6, 9} {
		for _, n := range []int{2, 15, 33, 144} {
			w := make([]float64, n)
			KaiserWindow(w, beta)
			halfLen := float64(n-1) / 2
			for k, v := range w {
				if got := kaiserAt(float64(k)-halfLen, halfLen, beta); math.Abs(got-v) > 1e-15 {
					t.Fatalf("beta %v n %d point %d: kaiserAt %v, KaiserWindow %v", beta, n, k, got, v)
				}
			}
			for _, x := range []float64{-halfLen - 1e-9, halfLen + 0.5, math.Inf(1), math.NaN()} {
				if got := kaiserAt(x, halfLen, beta); got != 0 {
					t.Errorf("kaiserAt(%v, %v, %v) = %v outside the window, want 0", x, halfLen, beta, got)
				}
			}
		}
	}
	if got := kaiserAt(0, 0, 6); got != 1 {
		t.Errorf("kaiserAt(0, 0, 6) = %v, want 1", got)
	}
}

func TestSinc(t *testing.T) {
	if got := sinc(0); got != 1 {
		t.Errorf("sinc(0) = %v, want 1", got)
	}
	for _, x := range []float64{1, 2, -3} {
		if got := sinc(x); math.Abs(got) > 1e-15 {
			t.Errorf("sinc(%v) = %v, want 0", x, got)
		}
	}
	if got, want := sinc(0.5), 2/math.Pi; math.Abs(got-want) > 1e-15 {
		t.Errorf("sinc(0.5) = %v, want %v", got, want)
	}
}

func TestNormalizeTaps(t *testing.T) {
	taps := []float32{1, 2, 3, 2}
	if err := NormalizeTaps(taps, 2); err != nil {
		t.Fatal(err)
	}
	want := []float32{0.25, 0.5, 0.75, 0.5}
	if !slices.Equal(taps, want) {
		t.Errorf("got %v, want %v", taps, want)
	}

	for _, bad := range [][]float64{nil, {1, -1}, {math.Inf(1), 1}, {math.NaN()}} {
		orig := slices.Clone(bad)
		if err := NormalizeTaps(bad, 1); err == nil {
			t.Errorf("%v: no error", orig)
		}
		for i := range bad {
			if math.Float64bits(bad[i]) != math.Float64bits(orig[i]) {
				t.Errorf("%v: modified to %v on error", orig, bad)
			}
		}
	}

	for _, gain := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		taps := []float64{1, 2}
		if err := NormalizeTaps(taps, gain); err == nil {
			t.Errorf("gain %v: no error", gain)
		}
		if !slices.Equal(taps, []float64{1, 2}) {
			t.Errorf("gain %v: modified to %v on error", gain, taps)
		}
	}
}

func TestLowPassFIRErrors(t *testing.T) {
	for _, c := range []struct {
		n            int
		cutoff, beta float64
	}{
		{0, 0.25, 6},
		{11, 0, 6},
		{11, 0.6, 6},
		{11, math.NaN(), 6},
		{11, 0.25, -1},
		{11, 0.25, math.NaN()},
		{11, 0.25, math.Inf(1)},
		{11, 0.25, 1e4},
	} {
		if _, err := LowPassFIR[float32](c.n, c.cutoff, c.beta); err == nil {
			t.Errorf("LowPassFIR[float32](%d, %v, %v): no error", c.n, c.cutoff, c.beta)
		}
		if _, err := LowPassFIR[float64](c.n, c.cutoff, c.beta); err == nil {
			t.Errorf("LowPassFIR[float64](%d, %v, %v): no error", c.n, c.cutoff, c.beta)
		}
	}
}

func TestLowPassFIRResponse(t *testing.T) {
	h, err := LowPassFIR[float64](101, 0.1, KaiserBeta(80))
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for k, v := range h {
		sum += v
		if v != h[len(h)-1-k] {
			t.Fatalf("tap %d is %v but its mirror is %v", k, v, h[len(h)-1-k])
		}
	}
	if math.Abs(sum-1) > 1e-12 {
		t.Errorf("DC gain %v, want 1", sum)
	}
	gain := func(f float64) float64 {
		var re, im float64
		for k, v := range h {
			s, c := math.Sincos(-2 * math.Pi * f * float64(k))
			re += v * c
			im += v * s
		}
		return math.Hypot(re, im)
	}
	if g := gain(0.05); math.Abs(g-1) > 1e-3 {
		t.Errorf("passband gain at 0.05 = %v", g)
	}
	if g := gain(0.1); math.Abs(g-0.5) > 0.01 {
		t.Errorf("gain at the cutoff = %v, want about 0.5", g)
	}
	for f := 0.15; f <= 0.5; f += 0.01 {
		if g := gain(f); g > 1e-3 {
			t.Errorf("stopband gain at %v = %v", f, g)
		}
	}
}

// refLowPass is the definition LowPassFIR has to reproduce: a sinc times
// KaiserWindow of the same length, normalized to a DC gain of 1.
func refLowPass(numTaps int, cutoff, beta float64) []float64 {
	h := make([]float64, numTaps)
	KaiserWindow(h, beta)
	center := float64(numTaps-1) / 2
	for k := range h {
		h[k] *= sinc(2 * cutoff * (float64(k) - center))
	}
	var sum float64
	for _, v := range h {
		sum += v
	}
	for k := range h {
		h[k] /= sum
	}
	return h
}

// The design rounds to T once, so each width matches the float64 reference
// rounded to that width.
func testLowPassFIRIsWindowedSinc[T Float](t *testing.T) {
	for _, c := range []struct {
		n            int
		cutoff, beta float64
	}{{1, 0.25, 9}, {2, 0.5, 0}, {143, 0.25, 9}, {64, 0.1, 6}} {
		h, err := LowPassFIR[T](c.n, c.cutoff, c.beta)
		if err != nil {
			t.Fatal(err)
		}
		for i, w := range refLowPass(c.n, c.cutoff, c.beta) {
			if want := T(w); math.Float64bits(float64(h[i])) != math.Float64bits(float64(want)) {
				t.Fatalf("%+v: tap %d = %v, want %v", c, i, h[i], want)
			}
		}
	}
}

func TestLowPassFIRIsWindowedSinc(t *testing.T) {
	t.Run("float32", testLowPassFIRIsWindowedSinc[float32])
	t.Run("float64", testLowPassFIRIsWindowedSinc[float64])
}

// halfBand returns LowPassFIR's 2x half-band filter scaled to a DC gain of 2.
// Doubling is exact, so the gain can be applied after normalizing to 1.
func halfBand[T Float](t *testing.T) []T {
	t.Helper()
	h, err := LowPassFIR[T](143, 0.25, 9)
	if err != nil {
		t.Fatal(err)
	}
	for i := range h {
		h[i] *= 2
	}
	return h
}

// refUpsample zero-stuffs src and convolves with every tap, zeros included,
// centered on tap len(taps)/2.
func refUpsample[T Sample](src []T, factor int, taps []T) []T {
	z := make([]T, len(src)*factor)
	for i, v := range src {
		z[i*factor] = v
	}
	out := make([]T, len(z))
	for i := range z {
		var sum T
		for k, c := range taps {
			j := i - k + len(taps)/2
			if j >= 0 && j < len(z) {
				sum += z[j] * c
			}
		}
		out[i] = sum
	}
	return out
}

func testUpsampleMatchesReference[T Sample](t *testing.T, tol float64) {
	r := rand.New(rand.NewPCG(1, 2))
	random := func(n int) []T {
		h := make([]T, n)
		for i := range h {
			h[i] = mkSample[T](r.NormFloat64(), 0)
		}
		return h
	}
	half := make([]T, 0, 143)
	for _, v := range halfBand[float64](t) {
		half = append(half, mkSample[T](v, 0))
	}
	for _, tc := range []struct {
		name   string
		factor int
		taps   []T
	}{
		{"halfband", 2, half},
		{"factor1", 1, random(7)},
		{"factor3-odd", 3, random(31)},
		{"factor3-even", 3, random(30)},
		{"factor4-short", 4, random(2)},
		{"single-tap", 2, []T{mkSample[T](1.5, 0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range []int{0, 1, 2, 5, 37, 200} {
				src := make([]T, n)
				for i := range src {
					src[i] = mkSample[T](r.Float64()*2-1, r.Float64()*2-1)
				}
				want := refUpsample(src, tc.factor, tc.taps)
				got := make([]T, n*tc.factor+3)
				got[len(got)-1] = 42
				if err := Upsample(got, src, tc.factor, tc.taps); err != nil {
					t.Fatal(err)
				}
				for i := range want {
					if d := sampleDiff(got[i], want[i]); d > tol {
						t.Fatalf("n=%d: sample %d = %v, want %v", n, i, got[i], want[i])
					}
				}
				if got[len(got)-1] != 42 {
					t.Fatalf("n=%d: wrote past len(src)*factor", n)
				}
			}
		})
	}
}

// The reference sums in a different order around the stuffed zeros, so the
// widths agree to rounding rather than exactly.
func TestUpsampleMatchesReference(t *testing.T) {
	t.Run("float32", func(t *testing.T) { testUpsampleMatchesReference[float32](t, 1e-5) })
	t.Run("float64", func(t *testing.T) { testUpsampleMatchesReference[float64](t, 1e-13) })
	t.Run("complex64", func(t *testing.T) { testUpsampleMatchesReference[complex64](t, 1e-5) })
	t.Run("complex128", func(t *testing.T) { testUpsampleMatchesReference[complex128](t, 1e-13) })
}

func TestUpsampleErrors(t *testing.T) {
	src := make([]float32, 4)
	if err := Upsample(make([]float32, 8), src, 0, []float32{1}); err == nil {
		t.Error("factor 0: no error")
	}
	if err := Upsample(make([]float32, 8), src, 2, nil); err == nil {
		t.Error("no taps: no error")
	}
	if err := Upsample(make([]float32, 7), src, 2, []float32{1}); err == nil {
		t.Error("short dst: no error")
	}
	if err := Upsample(make([]float32, 8), make([]float32, 2), math.MaxInt/2+1, []float32{1}); err == nil {
		t.Error("overflowing len(src)*factor: no error")
	}
}

// testUpsampleHalfBandPassesSamples checks that the 2x half-band filter keeps
// the original samples, to within the filter's rounding, away from the ends.
func testUpsampleHalfBandPassesSamples[T Float](t *testing.T) {
	h := halfBand[T](t)
	src := make([]T, 400)
	for i := range src {
		src[i] = T(math.Sin(2 * math.Pi * 0.05 * float64(i)))
	}
	dst := make([]T, 2*len(src))
	if err := Upsample(dst, src, 2, h); err != nil {
		t.Fatal(err)
	}
	for i := 100; i < 300; i++ {
		if d := math.Abs(float64(dst[2*i] - src[i])); d > 1e-5 {
			t.Fatalf("sample %d: %v, want %v", i, dst[2*i], src[i])
		}
		want := math.Sin(2 * math.Pi * 0.05 * (float64(i) + 0.5))
		if d := math.Abs(float64(dst[2*i+1]) - want); d > 1e-4 {
			t.Fatalf("midpoint %d: %v, want %v", i, dst[2*i+1], want)
		}
	}
}

func TestUpsampleHalfBandPassesSamples(t *testing.T) {
	t.Run("float32", testUpsampleHalfBandPassesSamples[float32])
	t.Run("float64", testUpsampleHalfBandPassesSamples[float64])
}
