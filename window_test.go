package dsp

import (
	"math"
	"math/cmplx"
	"slices"
	"testing"
)

type windowFuncs struct {
	name string
	f64  func([]float64)
	f32  func([]float32)
	// want5 is the window at length 5, which is short enough to write out and
	// long enough to pin the shape rather than just the endpoints.
	want5 []float64
}

func windowTable() []windowFuncs {
	return []windowFuncs{
		{"triangle", TriangleWindow[float64], TriangleWindow[float32],
			[]float64{1.0 / 3, 2.0 / 3, 1, 2.0 / 3, 1.0 / 3}},
		{"hamming", HammingWindow[float64], HammingWindow[float32],
			[]float64{0.07672, 0.53836, 1, 0.53836, 0.07672}},
		{"hanning", HannWindow[float64], HannWindow[float32],
			[]float64{0, 0.5, 1, 0.5, 0}},
		{"blackman", BlackmanWindow[float64], BlackmanWindow[float32],
			[]float64{0, 0.34, 1, 0.34, 0}},
		{"nuttall", NuttallWindow[float64], NuttallWindow[float32],
			[]float64{0, 0.211536, 1, 0.211536, 0}},
		{"blackman-harris", BlackmanHarrisWindow[float64], BlackmanHarrisWindow[float32],
			[]float64{0.00006, 0.21747, 1, 0.21747, 0.00006}},
		// The flat-top window goes slightly negative at the ends, and its
		// published coefficients sum to 1.000000003 rather than to 1.
		{"flattop", FlatTopWindow[float64], FlatTopWindow[float32],
			[]float64{-0.000421051, -0.05473684, 1.000000003, -0.05473684, -0.000421051}},
		// Kaiser at beta 8, whose values are I0(8*sqrt(1-r*r))/I0(8) for r of
		// -1, -0.5, 0, 0.5, 1.
		{"kaiser8", kaiser8[float64], kaiser8[float32],
			[]float64{0.0023388305, 0.3689727226, 1, 0.3689727226, 0.0023388305}},
	}
}

// kaiser8 pins beta so that KaiserWindow fits the table's fill signature. beta
// 8 is far enough above 5 for the shape to be a real taper rather than close to
// rectangular.
func kaiser8[T Float](dst []T) { KaiserWindow(dst, 8) }

func TestWindowsLength5(t *testing.T) {
	for _, w := range windowTable() {
		t.Run(w.name, func(t *testing.T) {
			got := make([]float64, 5)
			w.f64(got)
			for i, want := range w.want5 {
				if math.Abs(got[i]-want) > 1e-6 {
					t.Errorf("[%d] = %v, want %v (%v)", i, got[i], want, got)
				}
			}

			got32 := make([]float32, 5)
			w.f32(got32)
			for i := range got32 {
				if math.Abs(float64(got32[i])-got[i]) > 1e-6 {
					t.Errorf("float32 [%d] = %v, float64 %v", i, got32[i], got[i])
				}
			}
		})
	}
}

// TestWindowsShort covers the lengths where the general form breaks down. The
// cosine-sum windows divide by len(dst)-1, so length 1 used to give NaN.
func TestWindowsShort(t *testing.T) {
	for _, w := range windowTable() {
		t.Run(w.name, func(t *testing.T) {
			for _, n := range []int{0, 1, 2, 3} {
				got := make([]float64, n)
				w.f64(got)
				got32 := make([]float32, n)
				w.f32(got32)
				for i, v := range got {
					if math.IsNaN(v) || math.IsInf(v, 0) {
						t.Fatalf("n=%d: [%d] = %v", n, i, v)
					}
					if math.IsNaN(float64(got32[i])) {
						t.Fatalf("n=%d float32: [%d] = %v", n, i, got32[i])
					}
				}
				if n == 1 && got[0] != 1 {
					t.Errorf("n=1: got %v, want [1]", got)
				}
			}
		})
	}
}

func TestWindowsSymmetric(t *testing.T) {
	for _, w := range windowTable() {
		t.Run(w.name, func(t *testing.T) {
			for _, n := range []int{2, 3, 16, 17, 205} {
				got := make([]float64, n)
				w.f64(got)
				for i := range n / 2 {
					if math.Abs(got[i]-got[n-1-i]) > 1e-12 {
						t.Errorf("n=%d: [%d] = %v but [%d] = %v", n, i, got[i], n-1-i, got[n-1-i])
					}
				}
			}
		})
	}
}

// TestWindowsPeak checks that an odd-length window peaks at its center with the
// value the definitions give there, which is 1 for every window in this file:
// the cosine-sum coefficients sum to 1, and a Kaiser's center is I0(beta) over
// itself. The tolerance is 1e-8 rather than tighter because the published
// flat-top coefficients sum to 1.000000003, which is the window's own
// definition and not rounding.
func TestWindowsPeak(t *testing.T) {
	for _, w := range windowTable() {
		t.Run(w.name, func(t *testing.T) {
			const n = 65
			got := make([]float64, n)
			w.f64(got)
			if math.Abs(got[n/2]-1) > 1e-8 {
				t.Errorf("center = %v, want 1", got[n/2])
			}
			for i, v := range got {
				if v > got[n/2]+1e-12 {
					t.Errorf("[%d] = %v exceeds the center %v", i, v, got[n/2])
				}
			}
		})
	}
}

func TestWindowRejectsBadCoefficientCount(t *testing.T) {
	for _, a := range [][]float64{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected a panic for %d coefficients", len(a))
				}
			}()
			window(make([]float64, 8), a)
		}()
	}
}

// TestFreqCoeffAreCopies checks that the frequency-domain windows cannot be
// corrupted through the slice they hand out, which is why they are functions
// rather than package-level variables.
func TestFreqCoeffAreCopies(t *testing.T) {
	for name, fn := range map[string]func() []float64{
		"blackman": BlackmanFreqCoeff,
		"hamming":  HammingFreqCoeff,
		"hanning":  HannFreqCoeff,
	} {
		first := fn()
		want := slices.Clone(first)
		for i := range first {
			first[i] = 99
		}
		if got := fn(); !slices.Equal(got, want) {
			t.Errorf("%s: writing to the returned slice changed it to %v, want %v", name, got, want)
		}
	}
}

// sidelobeDB returns the peak sidelobe of a window, in dB below its main lobe.
// The window is zero-padded sixteenfold before the transform, so the peak is
// resolved between the bins of the unpadded spectrum rather than being missed
// between them.
func sidelobeDB(fill func([]float64)) float64 {
	const n, pad = 4096, 16
	w := make([]float64, n)
	fill(w)

	f, err := NewFFT[complex128](n * pad)
	if err != nil {
		panic(err)
	}
	spec := make([]complex128, n*pad)
	ForwardReal(f, spec, w)

	mag := make([]float64, n*pad/2)
	for i := range mag {
		mag[i] = cmplx.Abs(spec[i])
	}

	// Walk out to the first null. The magnitude turning back up is not enough
	// on its own: a flat-top window's main lobe is flat by construction, so it
	// ripples, and the null has to be identified by being deep as well.
	i := 1
	for i+1 < len(mag) && !(mag[i] <= mag[i+1] && mag[i] < mag[0]*0.02) {
		i++
	}
	var peak float64
	for _, v := range mag[i:] {
		peak = math.Max(peak, v)
	}
	return 20 * math.Log10(peak/mag[0])
}

// TestWindowSidelobes measures what every window in the package actually
// leaks. The numbers matter beyond documenting the windows: a spectrum
// measurement cannot see anything below its window's sidelobes, so this is what
// establishes that analyzing a stopband past 100 dB needs a Kaiser and that
// beta has to be near 19.4 to reach 150. Kaiser at beta 0 is a rectangular
// window, and its -13 dB is the check that the measurement itself is right.
func TestWindowSidelobes(t *testing.T) {
	for _, c := range []struct {
		name string
		max  float64
		fill func([]float64)
	}{
		{"triangle", -26, TriangleWindow[float64]},
		{"hanning", -31, HannWindow[float64]},
		{"hamming", -42, HammingWindow[float64]},
		{"blackman", -57, BlackmanWindow[float64]},
		{"blackman-harris", -91, BlackmanHarrisWindow[float64]},
		{"nuttall", -92, NuttallWindow[float64]},
		{"flattop", -92, FlatTopWindow[float64]},
		{"kaiser0", -13, func(w []float64) { KaiserWindow(w, 0) }},
		{"kaiser12.6", -94, func(w []float64) { KaiserWindow(w, 12.6) }},
		{"kaiser19.4", -150, func(w []float64) { KaiserWindow(w, 19.4) }},
		{"kaiser25", -200, func(w []float64) { KaiserWindow(w, 25) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := sidelobeDB(c.fill)
			t.Logf("peak sidelobe %.1f dB", got)
			if got > c.max {
				t.Errorf("peak sidelobe %.1f dB, want at most %.1f", got, c.max)
			}
		})
	}
}

// TestKaiserBetaIsNotASidelobeLevel guards the trap the doc comment warns
// about: the design formula selects a window far noisier than the attenuation
// it was asked for, so anyone reaching for KaiserBeta to pick an analysis
// window loses about 30 dB without being told.
func TestKaiserBetaIsNotASidelobeLevel(t *testing.T) {
	beta := KaiserBeta(150)
	if math.Abs(beta-15.573) > 0.01 {
		t.Errorf("KaiserBeta(150) = %v, want about 15.57", beta)
	}
	got := sidelobeDB(func(w []float64) { KaiserWindow(w, beta) })
	if got < -140 {
		t.Errorf("a window at beta %v leaks %.1f dB; the doc comment claims it falls well short of -150", beta, got)
	}
	if KaiserBeta(10) != 0 {
		t.Errorf("KaiserBeta(10) = %v, want 0 below 21 dB", KaiserBeta(10))
	}
	if b := KaiserBeta(40); math.Abs(b-3.395) > 0.01 {
		t.Errorf("KaiserBeta(40) = %v, want about 3.40", b)
	}
}

func TestBesselI0(t *testing.T) {
	for _, c := range []struct{ x, want float64 }{
		{0, 1},
		{1, 1.2660658777520084},
		{3.75, 9.118945860844562},
		{10, 2815.716628466254},
		{20, 4.355828255955353e7},
	} {
		got := besselI0(c.x)
		if math.Abs(got-c.want) > 1e-12*math.Abs(c.want) {
			t.Errorf("besselI0(%v) = %v, want %v", c.x, got, c.want)
		}
	}
}

func TestKaiserWindowShape(t *testing.T) {
	w := make([]float64, 65)
	KaiserWindow(w, 8)
	if math.Abs(w[32]-1) > 1e-12 {
		t.Errorf("center = %v, want 1", w[32])
	}
	for i := range w {
		if math.Abs(w[i]-w[len(w)-1-i]) > 1e-15 {
			t.Errorf("not symmetric at %d: %v vs %v", i, w[i], w[len(w)-1-i])
		}
		if w[i] < 0 || w[i] > 1 {
			t.Errorf("[%d] = %v, outside [0, 1]", i, w[i])
		}
	}

	// beta of 0 is a rectangular window.
	KaiserWindow(w, 0)
	for i, v := range w {
		if v != 1 {
			t.Fatalf("beta 0 at %d: %v, want exactly 1", i, v)
		}
	}

	one := []float64{0}
	KaiserWindow(one, 5)
	if one[0] != 1 {
		t.Errorf("one point = %v, want 1", one[0])
	}
	KaiserWindow([]float64{}, 5) // must not panic

	defer func() {
		if recover() == nil {
			t.Error("expected a panic for a negative beta")
		}
	}()
	KaiserWindow(make([]float64, 8), -1)
}

// TestWindowGain checks the two sums against the equivalent noise bandwidth
// they exist to compute, over every window in the table.
//
// The check is not that the numbers match a table: it is that the ENBW they
// give is at least one bin -- a window cannot pass less noise than a
// rectangular one -- and that the rectangular window comes out at exactly one,
// which is the case where both sums are known in closed form. Anything that
// swaps sum for sumSq fails that, and no plot would reveal it, because every
// level would be wrong by the same factor.
func TestWindowGain(t *testing.T) {
	const n = 512

	rect := make([]float64, n)
	for i := range rect {
		rect[i] = 1
	}
	sum, sumSq := WindowGain(rect)
	if sum != n || sumSq != n {
		t.Errorf("rectangular: sum %v, sumSq %v, want %d and %d", sum, sumSq, n, n)
	}
	if enbw := sumSq * n / (sum * sum); enbw != 1 {
		t.Errorf("rectangular ENBW = %v, want exactly 1", enbw)
	}

	for _, w := range windowTable() {
		t.Run(w.name, func(t *testing.T) {
			buf := make([]float64, n)
			w.f64(buf)
			sum, sumSq := WindowGain(buf)

			var wantSum, wantSumSq float64
			for _, v := range buf {
				wantSum += v
				wantSumSq += v * v
			}
			if sum != wantSum || sumSq != wantSumSq {
				t.Errorf("sum %v sumSq %v, want %v and %v", sum, sumSq, wantSum, wantSumSq)
			}
			if sumSq <= 0 {
				t.Fatalf("sumSq = %v, which divides into every level measured through it", sumSq)
			}
			enbw := sumSq * n / (sum * sum)
			if enbw < 1 || enbw > 5 {
				t.Errorf("ENBW = %v bins, which is outside anything a window of this kind gives", enbw)
			}
			// float32 has to agree to its own precision, since the sums are
			// accumulated in float64 either way.
			buf32 := make([]float32, n)
			w.f32(buf32)
			s32, sq32 := WindowGain(buf32)
			if math.Abs(s32-sum) > 1e-4*math.Abs(sum)+1e-4 ||
				math.Abs(sq32-sumSq) > 1e-4*sumSq {
				t.Errorf("float32 gains (%v, %v) against float64 (%v, %v)", s32, sq32, sum, sumSq)
			}
		})
	}

	// An empty window has no gain rather than a NaN.
	if sum, sumSq := WindowGain([]float64{}); sum != 0 || sumSq != 0 {
		t.Errorf("empty: (%v, %v), want (0, 0)", sum, sumSq)
	}
}
