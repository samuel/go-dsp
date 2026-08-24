package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

// df1Ref is a direct form 1 IIR filter written out longhand, with no delay
// lines to shift, as the reference for IIRFilter.Filter. Samples before the
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
		// two-coefficient filter got empty ones and Filter indexed pIn[0].
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
			f.Filter(got, src)
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
				f.Filter(blocked[off:end], src[off:end])
			}
			for i := range want {
				if blocked[i] != got[i] {
					t.Fatalf("blocked sample %d: got %v, want %v", i, blocked[i], got[i])
				}
			}
		})
	}
}

// TestIIRFilterNormalizesA0 checks that the stored aCoef[0] is 1, as the doc
// says. It used to keep the caller's value, which then went unused for float64
// but was widened into the complex variants' coefficient slices.
func TestIIRFilterNormalizesA0(t *testing.T) {
	f, err := NewIIRFilter([]float64{1, 0.6}, []float64{2, -1.4})
	if err != nil {
		t.Fatal(err)
	}
	if f.aCoef[0] != 1 {
		t.Errorf("aCoef[0] = %v, want 1", f.aCoef[0])
	}
	if f.bCoef[0] != 0.5 {
		t.Errorf("bCoef[0] = %v, want 0.5", f.bCoef[0])
	}
}

// TestIIRFilterCoefficients covers the accessor dspviz measures a filter
// through. It hands back the normalized coefficients, and it has to hand back
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
}

func TestNewIIRFilterRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		b, a []float64
	}{
		{"empty", nil, nil},
		{"mismatched", []float64{1, 2}, []float64{1}},
		// One coefficient each is a gain, not a filter, and leaves no delay
		// line for Filter to write into.
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
	f.Filter(c128out, c128in)
	c64out := make([]complex64, len(src))
	f2, err := NewRealCoefIIRFilter[complex64](b32, a32)
	if err != nil {
		t.Fatal(err)
	}
	f2.Filter(c64out, c64in)

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
// w[n] = x[n] + a*w[n-1] and y[n] = w[n] - w[n-1], a unit step from n = 0 gives
// y[n] = a^n exactly.
func TestDCFilterStepResponse(t *testing.T) {
	const a = 0.95
	src := make([]float64, 32)
	for i := range src {
		src[i] = 1
	}
	dst := make([]float64, len(src))
	f, err := NewDCFilter[float64](a)
	if err != nil {
		t.Fatal(err)
	}
	f.Filter(dst, src)
	for n := range dst {
		want := math.Pow(a, float64(n))
		if math.Abs(dst[n]-want) > 1e-12 {
			t.Fatalf("sample %d: got %v, want %v", n, dst[n], want)
		}
	}

	// FilterOne has to walk the same trajectory.
	f, err = NewDCFilter[float64](a)
	if err != nil {
		t.Fatal(err)
	}
	for n := range dst {
		if got := f.FilterOne(src[n]); got != dst[n] {
			t.Fatalf("FilterOne sample %d: got %v, want %v", n, got, dst[n])
		}
	}

	out32 := make([]float32, len(src))
	in32 := make([]float32, len(src))
	for i := range in32 {
		in32[i] = 1
	}
	f32, err := NewDCFilter[float32](a)
	if err != nil {
		t.Fatal(err)
	}
	f32.Filter(out32, in32)
	f32, err = NewDCFilter[float32](a)
	if err != nil {
		t.Fatal(err)
	}
	for n := range out32 {
		want := float32(math.Pow(a, float64(n)))
		if math.Abs(float64(out32[n]-want)) > 1e-5 {
			t.Fatalf("float32 sample %d: got %v, want %v", n, out32[n], want)
		}
		if got := f32.FilterOne(in32[n]); got != out32[n] {
			t.Fatalf("FilterOne32 sample %d: got %v, want %v", n, got, out32[n])
		}
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
	f.Filter(dst, src)

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
	f.Filter(first, src)
	f.Reset()
	again := make([]float64, len(src))
	f.Filter(again, src)
	for i := range first {
		if again[i] != first[i] {
			t.Fatalf("sample %d after Reset: %v, want %v", i, again[i], first[i])
		}
	}
}

func TestDCFilterReset(t *testing.T) {
	f, err := NewDCFilter[float64](0.9)
	if err != nil {
		t.Fatal(err)
	}
	first := f.FilterOne(1)
	f.Reset()
	if again := f.FilterOne(1); again != first {
		t.Errorf("after Reset got %v, want %v", again, first)
	}
}

func TestNewDCFilterRejects(t *testing.T) {
	for _, a := range []float64{-0.1, 1, 1.5} {
		func() {
			_, err := NewDCFilter[float64](a)
			if err == nil {
				t.Errorf("did not return error for pole %v", a)
			}
		}()
	}
}

func TestNewRealCoefIIRFilterRejects(t *testing.T) {
	_, err := NewRealCoefIIRFilter[complex64]([]float32{1, 2}, []float32{1})
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestIIRFilterOneMatchesFilter is the claim the shared step exists for: the
// block form and the single-sample form are the same arithmetic in the same
// order, so filtering a stream either way gives bit-identical output. Nothing
// else compares them, and they used to be two copies of the recursion.
func TestIIRFilterOneMatchesFilter(t *testing.T) {
	src := rampAndTone(64)
	b := []float64{0.25, 0.5, 0.25}
	a := []float64{1, -0.3, 0.1}

	block := must(NewIIRFilter(b, a))
	dst := make([]float64, len(src))
	block.Filter(dst, src)

	single := must(NewIIRFilter(b, a))
	for i, v := range src {
		if got := single.FilterOne(v); got != dst[i] {
			t.Fatalf("sample %d: FilterOne %v, Filter %v (differ by %g)", i, got, dst[i], got-dst[i])
		}
	}

	// And at the complex instantiation, where the coefficients are widened.
	csrc := make([]complex128, len(src))
	for i, v := range src {
		csrc[i] = complex(v, -v/2)
	}
	cblock := must(NewRealCoefIIRFilter[complex128](b, a))
	cdst := make([]complex128, len(csrc))
	cblock.Filter(cdst, csrc)
	csingle := must(NewRealCoefIIRFilter[complex128](b, a))
	for i, v := range csrc {
		if got := csingle.FilterOne(v); got != cdst[i] {
			t.Fatalf("complex sample %d: FilterOne %v, Filter %v", i, got, cdst[i])
		}
	}
}
