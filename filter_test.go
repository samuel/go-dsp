package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

// df1Ref is a direct form 1 IIR filter written out longhand, with no delay
// lines to shift, as the reference for IIRFilter.Process. Samples before the
// start of the input are zero.
func df1Ref(bCoef, aCoef, src []float64) []float64 {
	b := make([]float64, len(bCoef))
	a := make([]float64, len(aCoef))
	for i := range b {
		b[i] = bCoef[i] / aCoef[0]
		a[i] = aCoef[i] / aCoef[0]
	}
	at := func(s []float64, i int) float64 {
		if i < 0 {
			return 0
		}
		return s[i]
	}
	dst := make([]float64, len(src))
	for n := range src {
		var sum float64
		for k := range b {
			sum += b[k] * at(src, n-k)
		}
		for k := 1; k < len(a); k++ {
			sum -= a[k] * at(dst, n-k)
		}
		dst[n] = sum
	}
	return dst
}

func rampAndTone(n int) []float64 {
	src := make([]float64, n)
	for i := range src {
		src[i] = math.Sin(float64(i)*0.3) + float64(i)/float64(n)
	}
	return src
}

func TestIIRFilter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		b, a  []float64
		order int
	}{
		// Order 1 used to panic: the delay lines were len(b)-1 long, so a
		// two-coefficient filter got empty ones and Process indexed pIn[0].
		{"order1", []float64{0.5, 0.3}, []float64{1, -0.7}, 1},
		// A non-unity a[0], to exercise the normalization.
		{"order1-unnormalized", []float64{1, 0.6}, []float64{2, -1.4}, 1},
		{"order2", []float64{0.2929, 0.5858, 0.2929}, []float64{1, 0, 0.1716}, 2},
		{"order4", []float64{0.1, -0.2, 0.3, -0.2, 0.1}, []float64{1, -0.5, 0.4, -0.1, 0.02}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := rampAndTone(64)
			want := df1Ref(tc.b, tc.a, src)

			got := make([]float64, len(src))
			f, err := NewIIRFilter(tc.b, tc.a)
			if err != nil {
				t.Fatal(err)
			}
			f.Process(got, src)
			for i := range want {
				if math.Abs(got[i]-want[i]) > 1e-12 {
					t.Fatalf("sample %d: got %v, want %v", i, got[i], want[i])
				}
			}

			// The same stream in blocks has to give the same answer, since the
			// delay lines carry across calls.
			blocked := make([]float64, len(src))
			f, err = NewIIRFilter(tc.b, tc.a)
			if err != nil {
				t.Fatal(err)
			}
			for off := 0; off < len(src); off += 7 {
				end := min(off+7, len(src))
				f.Process(blocked[off:end], src[off:end])
			}
			for i := range want {
				if blocked[i] != got[i] {
					t.Fatalf("blocked sample %d: got %v, want %v", i, blocked[i], got[i])
				}
			}
		})
	}
}

// TestIIRFilterNormalizesA0 checks that the stored a[0] is 1, as the doc says.
func TestIIRFilterNormalizesA0(t *testing.T) {
	f, err := NewIIRFilter([]float64{1, 0.6}, []float64{2, -1.4})
	if err != nil {
		t.Fatal(err)
	}
	if f.a.native[0] != 1 {
		t.Errorf("a[0] = %v, want 1", f.a.native[0])
	}
	if f.b.native[0] != 0.5 {
		t.Errorf("b[0] = %v, want 0.5", f.b.native[0])
	}
}

// TestIIRFilterCoefficients covers the accessor the analysis package measures a
// filter through. It hands back the normalized coefficients, and it has to hand back
// copies: the same filter is asked for them once per frequency grid, so an
// aliased slice would let one caller's scaling reach the next.
func TestIIRFilterCoefficients(t *testing.T) {
	// Powers of two, so normalizing by a[0] is exact and the expectations can
	// be written out rather than recomputed the way the constructor does.
	f, err := NewIIRFilter([]float64{1, 0.5, -0.25}, []float64{4, 2, -1})
	if err != nil {
		t.Fatal(err)
	}
	b, a := f.Coefficients()
	wantSlice(t, "b", b, []float64{0.25, 0.125, -0.0625})
	wantSlice(t, "a", a, []float64{1, 0.5, -0.25})

	b[0], a[1] = 99, 99
	b2, a2 := f.Coefficients()
	wantSlice(t, "b after writing to the copy", b2, []float64{0.25, 0.125, -0.0625})
	wantSlice(t, "a after writing to the copy", a2, []float64{1, 0.5, -0.25})

	// The complex instantiation reports the same numbers with a zero
	// imaginary part, which is what lets a real filter be measured over a
	// complex signal.
	cf, err := NewRealCoefIIRFilter[complex128]([]float64{1, 0.5, -0.25}, []float64{4, 2, -1})
	if err != nil {
		t.Fatal(err)
	}
	cb, ca := cf.Coefficients()
	wantSlice(t, "complex b", cb, []complex128{0.25, 0.125, -0.0625})
	wantSlice(t, "complex a", ca, []complex128{1, 0.5, -0.25})
	rb, ra, ok := cf.RealCoefficients()
	if !ok {
		t.Fatal("real-coefficient filter: RealCoefficients ok false")
	}
	wantSlice(t, "real b", rb, []float64{0.25, 0.125, -0.0625})
	wantSlice(t, "real a", ra, []float64{1, 0.5, -0.25})

	// The answer depends on the values: complex coefficients whose imaginary
	// parts are all zero are real, and one non-zero part in a or b is not.
	zf := must(NewIIRFilter([]complex64{1, 0.5}, []complex64{2, -1}))
	zb, za, ok := zf.RealCoefficients()
	if !ok {
		t.Error("complex type, real values: RealCoefficients ok false")
	}
	wantSlice(t, "zero-imaginary b", zb, []float64{0.5, 0.25})
	wantSlice(t, "zero-imaginary a", za, []float64{1, -0.5})
	for _, c := range []struct {
		name string
		b, a []complex128
	}{
		{"complex a", []complex128{1, 0.5}, []complex128{1, -0.5i}},
		{"complex b", []complex128{1, 0.5i}, []complex128{1, -0.5}},
	} {
		if b, a, ok := must(NewIIRFilter(c.b, c.a)).RealCoefficients(); ok || b != nil || a != nil {
			t.Errorf("%s: RealCoefficients = (%v, %v, %v), want (nil, nil, false)", c.name, b, a, ok)
		}
	}
}

func TestNewIIRFilterRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		b, a []float64
	}{
		{"empty", nil, nil},
		{"mismatched", []float64{1, 2}, []float64{1}},
		// One coefficient each is a gain, not a filter, and leaves no delay
		// line for Process to write into.
		{"single", []float64{1}, []float64{1}},
		// a[0] divides every coefficient, so a zero there makes the whole
		// filter 0/0 rather than a filter with a zero coefficient.
		{"zero a0", []float64{1, 2}, []float64{0, 0.5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewIIRFilter(tc.b, tc.a)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestComplexIIRFilterMatchesReal checks the complex instantiations against the
// real one on an input whose imaginary part is zero, which is the only way the
// coefficient widening can be compared directly.
func TestComplexIIRFilterMatchesReal(t *testing.T) {
	b := []float64{0.2929, 0.5858, 0.2929}
	a := []float64{2, 0, 0.3432}
	src := rampAndTone(64)
	want := df1Ref(b, a, src)

	c128in := make([]complex128, len(src))
	c64in := make([]complex64, len(src))
	b32 := make([]float32, len(b))
	a32 := make([]float32, len(a))
	for i := range src {
		c128in[i] = complex(src[i], 0)
		c64in[i] = complex(float32(src[i]), 0)
	}
	for i := range b {
		b32[i], a32[i] = float32(b[i]), float32(a[i])
	}

	c128out := make([]complex128, len(src))
	f, err := NewRealCoefIIRFilter[complex128](b, a)
	if err != nil {
		t.Fatal(err)
	}
	f.Process(c128out, c128in)
	c64out := make([]complex64, len(src))
	f2, err := NewRealCoefIIRFilter[complex64](b32, a32)
	if err != nil {
		t.Fatal(err)
	}
	f2.Process(c64out, c64in)

	for i := range want {
		if math.Abs(real(c128out[i])-want[i]) > 1e-12 || imag(c128out[i]) != 0 {
			t.Fatalf("complex128 sample %d: got %v, want %v", i, c128out[i], want[i])
		}
		if math.Abs(float64(real(c64out[i]))-want[i]) > 1e-4 || imag(c64out[i]) != 0 {
			t.Fatalf("complex64 sample %d: got %v, want %v", i, c64out[i], want[i])
		}
	}
}

// TestDCFilterStepResponse checks the closed form of the step response. With
// y[n] = x[n] - x[n-1] + a*y[n-1], a unit step from n = 0 gives y[n] = a^n. A complex step runs each part through it independently.
func TestDCFilterStepResponse(t *testing.T) {
	const cutoff = 0.008
	const n = 32
	want := func(f interface{ Pole() float64 }, k int) float64 { return math.Pow(f.Pole(), float64(k)) }

	t.Run("float64", func(t *testing.T) {
		src := make([]float64, n)
		for i := range src {
			src[i] = 1
		}
		got := runBoth(t, must(NewDCFilter[float64](cutoff)), must(NewDCFilter[float64](cutoff)), src)
		f := must(NewDCFilter[float64](cutoff))
		for k, v := range got {
			if math.Abs(v-want(f, k)) > 1e-12 {
				t.Fatalf("sample %d: got %v, want %v", k, v, want(f, k))
			}
		}
	})
	t.Run("float32", func(t *testing.T) {
		src := make([]float32, n)
		for i := range src {
			src[i] = 1
		}
		got := runBoth(t, must(NewDCFilter[float32](cutoff)), must(NewDCFilter[float32](cutoff)), src)
		f := must(NewDCFilter[float32](cutoff))
		for k, v := range got {
			if math.Abs(float64(v)-want(f, k)) > 1e-5 {
				t.Fatalf("sample %d: got %v, want %v", k, v, want(f, k))
			}
		}
	})
	t.Run("complex64", func(t *testing.T) {
		src := make([]complex64, n)
		for i := range src {
			src[i] = complex(1, -2)
		}
		got := runBoth(t, must(NewDCFilter[complex64](cutoff)), must(NewDCFilter[complex64](cutoff)), src)
		f := must(NewDCFilter[complex64](cutoff))
		for k, v := range got {
			w := want(f, k)
			if math.Abs(float64(real(v))-w) > 1e-5 || math.Abs(float64(imag(v))+2*w) > 1e-5 {
				t.Fatalf("sample %d: got %v, want %v", k, v, complex(w, -2*w))
			}
		}
	})
	t.Run("complex128", func(t *testing.T) {
		src := make([]complex128, n)
		for i := range src {
			src[i] = complex(1, -2)
		}
		got := runBoth(t, must(NewDCFilter[complex128](cutoff)), must(NewDCFilter[complex128](cutoff)), src)
		f := must(NewDCFilter[complex128](cutoff))
		for k, v := range got {
			w := want(f, k)
			if math.Abs(real(v)-w) > 1e-12 || math.Abs(imag(v)+2*w) > 1e-12 {
				t.Fatalf("sample %d: got %v, want %v", k, v, complex(w, -2*w))
			}
		}
	})
}

// runBoth runs src through block with Process and through one with ProcessOne,
// requires the two to be bit-identical, and returns the output.
func runBoth[T Sample](t *testing.T, block, one *DCFilter[T], src []T) []T {
	t.Helper()
	dst := make([]T, len(src))
	block.Process(dst, src)
	for i, x := range src {
		if got := one.ProcessOne(x); got != dst[i] {
			t.Fatalf("ProcessOne sample %d: got %v, Process gave %v", i, got, dst[i])
		}
	}
	return dst
}

// TestDCFilterComplexMatchesParts checks that a complex filter is exactly two
// real filters run over the parts.
func TestDCFilterComplexMatchesParts(t *testing.T) {
	const cutoff = 0.01
	src := make([]complex128, 200)
	for i := range src {
		x := float64(i)
		src[i] = complex(0.3+math.Sin(0.37*x), -0.7+math.Cos(0.11*x))
	}
	dst := make([]complex128, len(src))
	// Two calls, so the state carries across a block boundary.
	c := must(NewDCFilter[complex128](cutoff))
	c.Process(dst[:77], src[:77])
	c.Process(dst[77:], src[77:])
	re, im := must(NewDCFilter[float64](cutoff)), must(NewDCFilter[float64](cutoff))
	for i, x := range src {
		want := complex(re.ProcessOne(real(x)), im.ProcessOne(imag(x)))
		if dst[i] != want {
			t.Fatalf("sample %d: got %v, want %v", i, dst[i], want)
		}
	}
}

// TestDCFilterCutoff checks that the response is down 3 dB at the cutoff.
func TestDCFilterCutoff(t *testing.T) {
	for _, fc := range []float64{1e-6, 1e-4, 0.001, 0.01, 0.05, 0.1, MaxDCFilterCutoff} {
		f := must(NewDCFilter[float64](fc))
		if f.Cutoff() != fc {
			t.Errorf("Cutoff() = %v, want %v", f.Cutoff(), fc)
		}
		b, a := realBA(f.RealCoefficients())
		got := cmplx.Abs(Freqz(b, a, []float64{fc})[0])
		if math.Abs(got-1/math.Sqrt2) > 1e-9 {
			t.Errorf("cutoff %v: |H| = %v, want %v", fc, got, 1/math.Sqrt2)
		}
	}
	if p := dcPole(MaxDCFilterCutoff); math.Abs(p) > 1e-12 {
		t.Errorf("pole at MaxDCFilterCutoff = %v, want 0", p)
	}
	if want := math.Acos(0.75) / (2 * math.Pi); MaxDCFilterCutoff != want {
		t.Errorf("MaxDCFilterCutoff = %v, want %v", MaxDCFilterCutoff, want)
	}
}

// TestDCFilterSetCutoff checks that a cutoff change keeps the state: the stream
// matches the recursion with the pole changed at that sample and nothing reset.
func TestDCFilterSetCutoff(t *testing.T) {
	const fc1, fc2, split = 0.01, 0.002, 40
	src := make([]float64, 100)
	for i := range src {
		src[i] = 1 + math.Sin(0.2*float64(i))
	}
	f := must(NewDCFilter[float64](fc1))
	dst := make([]float64, len(src))
	f.Process(dst[:split], src[:split])
	if err := f.SetCutoff(fc2); err != nil {
		t.Fatal(err)
	}
	f.Process(dst[split:], src[split:])

	a1, a2 := must(NewDCFilter[float64](fc1)).Pole(), must(NewDCFilter[float64](fc2)).Pole()
	var xp, yp float64
	for i, x := range src {
		a := a1
		if i >= split {
			a = a2
		}
		yp = x - xp + a*yp
		xp = x
		if dst[i] != yp {
			t.Fatalf("sample %d: got %v, want %v", i, dst[i], yp)
		}
	}

	// Once a constant input has settled, retuning leaves the output at zero.
	settled := must(NewDCFilter[complex128](0.016))
	dc := make([]complex128, 4000)
	for i := range dc {
		dc[i] = complex(1, -0.5)
	}
	settled.Process(dc, dc)
	if err := settled.SetCutoff(0.002); err != nil {
		t.Fatal(err)
	}
	if y := settled.ProcessOne(complex(1, -0.5)); cmplx.Abs(y) > 1e-12 {
		t.Errorf("output after retuning a settled filter = %v, want 0", y)
	}

	// A rejected cutoff changes nothing.
	pole, before := f.Pole(), *f
	if err := f.SetCutoff(1); err == nil {
		t.Fatal("SetCutoff(1) returned no error")
	}
	if *f != before || f.Pole() != pole || f.Cutoff() != fc2 {
		t.Errorf("rejected SetCutoff changed the filter: %+v, was %+v", *f, before)
	}
}

// TestIIRFilterOverComplexCoefficients checks that the filter runs with genuinely
// complex coefficients, which the widening constructor cannot produce.
func TestIIRFilterOverComplexCoefficients(t *testing.T) {
	// A one-pole filter with its pole off the real axis, so the output is
	// complex even for a real impulse.
	b := []complex128{1, 0}
	a := []complex128{1, complex(-0.5, -0.5)}
	f, err := NewIIRFilter(b, a)
	if err != nil {
		t.Fatal(err)
	}
	src := make([]complex128, 8)
	src[0] = 1
	dst := make([]complex128, len(src))
	f.Process(dst, src)

	// y[n] = x[n] - a1*y[n-1], so the impulse response is (-a1)^n.
	want := complex128(1)
	for n := range dst {
		if cmplx.Abs(dst[n]-want) > 1e-12 {
			t.Fatalf("sample %d: got %v, want %v", n, dst[n], want)
		}
		want *= -a[1]
	}
}

// TestIIRFilterReset checks that a reset filter reproduces its first output.
func TestIIRFilterReset(t *testing.T) {
	b := []float64{0.2929, 0.5858, 0.2929}
	a := []float64{1, 0, 0.1716}
	src := rampAndTone(32)
	f, err := NewIIRFilter(b, a)
	if err != nil {
		t.Fatal(err)
	}
	first := make([]float64, len(src))
	f.Process(first, src)
	f.Reset()
	again := make([]float64, len(src))
	f.Process(again, src)
	for i := range first {
		if again[i] != first[i] {
			t.Fatalf("sample %d after Reset: %v, want %v", i, again[i], first[i])
		}
	}
}

func TestDCFilterReset(t *testing.T) {
	f := must(NewDCFilter[complex64](0.01))
	first := f.ProcessOne(complex(1, 2))
	f.Reset()
	if again := f.ProcessOne(complex(1, 2)); again != first {
		t.Errorf("after Reset got %v, want %v", again, first)
	}
}

func TestNewDCFilterRejects(t *testing.T) {
	for _, fc := range []float64{0, -0.1, math.NaN(), math.Inf(1), math.Inf(-1), MaxDCFilterCutoff * (1 + 1e-12), 0.5} {
		if _, err := NewDCFilter[float64](fc); err == nil {
			t.Errorf("did not return error for cutoff %v", fc)
		}
	}
}

func TestNewRealCoefIIRFilterRejects(t *testing.T) {
	_, err := NewRealCoefIIRFilter[complex64]([]float32{1, 2}, []float32{1})
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestIIRProcessOneMatchesProcess is the claim the shared step exists for: the
// block form and the single-sample form are the same arithmetic in the same
// order, so filtering a stream either way gives bit-identical output. Nothing
// else compares them.
func TestIIRProcessOneMatchesProcess(t *testing.T) {
	src := rampAndTone(64)
	b := []float64{0.25, 0.5, 0.25}
	a := []float64{1, -0.3, 0.1}

	block := must(NewIIRFilter(b, a))
	dst := make([]float64, len(src))
	block.Process(dst, src)

	single := must(NewIIRFilter(b, a))
	for i, v := range src {
		if got := single.ProcessOne(v); got != dst[i] {
			t.Fatalf("sample %d: ProcessOne %v, Process %v (differ by %g)", i, got, dst[i], got-dst[i])
		}
	}

	// And at the complex instantiation, where the coefficients are widened.
	csrc := make([]complex128, len(src))
	for i, v := range src {
		csrc[i] = complex(v, -v/2)
	}
	cblock := must(NewRealCoefIIRFilter[complex128](b, a))
	cdst := make([]complex128, len(csrc))
	cblock.Process(cdst, csrc)
	csingle := must(NewRealCoefIIRFilter[complex128](b, a))
	for i, v := range csrc {
		if got := csingle.ProcessOne(v); got != cdst[i] {
			t.Fatalf("complex sample %d: ProcessOne %v, Process %v", i, got, cdst[i])
		}
	}
}
