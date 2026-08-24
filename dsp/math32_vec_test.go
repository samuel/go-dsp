package dsp

import (
	"math"
	"math/cmplx"
	"slices"
	"testing"
)

// TestVCMulRealC64 exercises the windowing multiply and its length rules.
func TestVCMulRealC64(t *testing.T) {
	src := make([]complex64, 40)
	mul := make([]float32, 40)
	for i := range src {
		src[i] = complex(float32(i)-20, float32(20-i))
		mul[i] = float32(i) / 40
	}
	// Mismatched lengths: the shortest of the three wins.
	for _, n := range []int{0, 1, 3, 4, 7, 16, 33, 40} {
		got := make([]complex64, n+1)
		VCMulReal(got[:n], src[:n], mul)
		for i := range n {
			want := complex(real(src[i])*mul[i], imag(src[i])*mul[i])
			if got[i] != want {
				t.Fatalf("n=%d [%d] = %v, want %v", n, i, got[i], want)
			}
		}
		if got[n] != 0 {
			t.Fatalf("n=%d wrote past the end: %v", n, got[n])
		}
	}

	// A short multiplier truncates rather than overruns.
	got := make([]complex64, len(src))
	VCMulReal(got, src, mul[:5])
	for i, v := range got[5:] {
		if v != 0 {
			t.Fatalf("wrote past the multiplier at %d: %v", i+5, v)
		}
	}
}

// TestVCMul checks the product against values computed here rather than
// against src re-read after the call.
func TestVCMul(t *testing.T) {
	src := []complex64{complex(1, 2), complex(-3, 4), complex(5, -6)}
	mul := []complex64{complex(0, 1), complex(2, 0), complex(1, 1)}
	want := []complex64{complex(-2, 1), complex(-6, 8), complex(11, -1)}
	dst := make([]complex64, len(src)+1)
	VCMul(dst, src, mul)
	for i, v := range want {
		if dst[i] != v {
			t.Errorf("[%d] = %v, want %v", i, dst[i], v)
		}
	}
	if dst[len(src)] != 0 {
		t.Errorf("wrote past the end: %v", dst)
	}
	// src is the source and must come back untouched.
	if src[0] != complex(1, 2) {
		t.Errorf("wrote into src: %v", src)
	}

	// The shortest of the three bounds the work.
	short := make([]complex64, len(src))
	VCMul(short, src, mul[:1])
	if short[1] != 0 {
		t.Errorf("ignored a short multiplier: %v", short)
	}

	// Multiplying in place is the same operation with dst aliasing src.
	inPlace := slices.Clone(src)
	VCMul(inPlace, inPlace, mul)
	if !slices.Equal(inPlace, want) {
		t.Errorf("in place = %v, want %v", inPlace, want)
	}
}

func TestVAdd(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{10, 20, 30, 40, 50}
	dst := make([]float32, len(b))
	VAdd(dst, a, b)
	want := []float32{11, 22, 33, 44, 0}
	if !slices.Equal(dst, want) {
		t.Errorf("= %v, want %v", dst, want)
	}

	// Accumulating in place is the three-operand form with dst aliasing an input.
	acc := slices.Clone(b)
	VAdd(acc, acc, a)
	if got := []float32{11, 22, 33, 44, 50}; !slices.Equal(acc, got) {
		t.Errorf("in place = %v, want %v", acc, got)
	}

	// A short output stops early rather than panicking.
	VAdd(dst[:2], a, b)
}

func TestVCAdd(t *testing.T) {
	a := []complex64{complex(1, 2), complex(3, 4)}
	b := []complex64{complex(10, 20), complex(30, 40), complex(50, 60)}
	dst := make([]complex64, len(b))
	VCAdd(dst, a, b)
	want := []complex64{complex(11, 22), complex(33, 44), 0}
	if !slices.Equal(dst, want) {
		t.Errorf("= %v, want %v", dst, want)
	}

	acc := slices.Clone(b)
	VCAdd(acc, acc, a)
	if got := []complex64{complex(11, 22), complex(33, 44), complex(50, 60)}; !slices.Equal(acc, got) {
		t.Errorf("in place = %v, want %v", acc, got)
	}

	VCAdd(dst[:1], a, b)
}

// TestVScaleC64 checks the reinterpretation of complex64 as float32 pairs that
// lets vscaleF32 do the work, including for an empty slice, where taking the
// address of element zero would not be safe.
func TestVCScaleReal(t *testing.T) {
	src := make([]complex64, 37)
	for i := range src {
		src[i] = complex(float32(i)-18, float32(3*i)-10)
	}
	for _, n := range []int{0, 1, 5, 37} {
		dst := make([]complex64, n+1)
		VCScaleReal(dst[:n], src[:n], 0.25)
		for i := range n {
			want := complex(real(src[i])*0.25, imag(src[i])*0.25)
			if dst[i] != want {
				t.Fatalf("n=%d [%d] = %v, want %v", n, i, dst[i], want)
			}
		}
		if dst[n] != 0 {
			t.Fatalf("n=%d wrote past the end: %v", n, dst[n])
		}
	}
	VCScaleReal[complex64, float64](nil, nil, 2)
}

func TestConj(t *testing.T) {
	for _, x := range []complex64{complex(1, 2), complex(-3, 0), complex(0, -4), 0} {
		if got, want := Conj(x), complex64(cmplx.Conj(complex128(x))); got != want {
			t.Errorf("Conj(%v) = %v, want %v", x, got, want)
		}
	}
}

func TestPhase(t *testing.T) {
	for _, x := range []complex64{complex(1, 0), complex(0, 1), complex(-1, 0), complex(0, -1), complex(3, -4)} {
		want := math.Atan2(float64(imag(x)), float64(real(x)))
		if got := Phase(x); math.Abs(got-want) > 1e-6 {
			t.Errorf("Phase(%v) = %v, want %v", x, got, want)
		}
		// FastPhase shares FastAtan2's error bound.
		if got := FastPhase(x); math.Abs(float64(got)-want) > 0.02 {
			t.Errorf("FastPhase(%v) = %v, want %v", x, got, want)
		}
	}
}

// TestPolarDiscriminator checks both widths against arg(a * conj(b)), which is
// the definition their doc comments give.
func TestPolarDiscriminator(t *testing.T) {
	for _, tc := range [][2]complex128{
		{complex(1, 0), complex(1, 0)},
		{complex(0, 1), complex(1, 0)},
		{complex(-1, 0), complex(0, 1)},
		{complex(3, -4), complex(-2, 5)},
	} {
		a, b := tc[0], tc[1]
		want := cmplx.Phase(a * cmplx.Conj(b))
		if got := PolarDiscriminator(a, b); math.Abs(got-want) > 1e-12 {
			t.Errorf("PolarDiscriminator(%v, %v) = %v, want %v", a, b, got, want)
		}
		if got := FastPolarDiscriminator(complex64(a), complex64(b)); math.Abs(float64(got)-want) > 0.02 {
			t.Errorf("FastPolarDiscriminator(%v, %v) = %v, want %v", a, b, got, want)
		}
	}
}

// TestFMDemodMethod covers the exported method, which the other tests
// reach past by calling fmDemodulateAsm directly.
func TestFMDemodMethod(t *testing.T) {
	src := []complex64{complex(0, 2), complex(1, 2), complex(-3, 7), complex(4, -9)}
	dst := make([]float32, len(src)+1)
	var f FMDemod
	f.Demodulate(dst, src)
	if dst[len(src)] != 0 {
		t.Errorf("wrote past the shorter slice: %v", dst[len(src)])
	}
	want := make([]float32, len(src))
	fmDemodulate(&FMDemod{}, want, src)
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("[%d] = %v, want %v", i, dst[i], want[i])
		}
	}
}

// TestExportedWrappers covers the exported entry points, which the tests
// otherwise reach past by calling the asm symbols directly.
func TestExportedWrappers(t *testing.T) {
	cplx := make([]complex64, 8)
	for i := range cplx {
		cplx[i] = complex(float32(i), float32(-i))
	}
	want := slices.Clone(cplx)
	rotate90(want)
	NewRotator90[complex64]().Rotate(cplx)
	if !slices.Equal(cplx, want) {
		t.Errorf("Rotator90.Rotate gave %v, want %v", cplx, want)
	}
	f, err := NewBoxcarDecimator(2)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.FilterInPlace(cplx); len(got) != len(cplx)/2 {
		t.Errorf("BoxcarDecimator.FilterInPlace returned %d samples, want %d", len(got), len(cplx)/2)
	}
}

// TestFastAtan2FineAxes covers the branches for a vector on an axis, where the
// ratio the approximation divides by is zero or infinite.
func TestFastAtan2FineAxes(t *testing.T) {
	for _, tc := range []struct {
		y, x float32
		want float64
	}{
		{0, 0, 0},
		{1, 0, math.Pi / 2},
		{-1, 0, -math.Pi / 2},
		{0, 1, 0},
		{0, -1, math.Pi},
		{-0.0001, -1, -math.Pi},
		{1, 1, math.Pi / 4},
		{-1, -1, -3 * math.Pi / 4},
		{1, -1, 3 * math.Pi / 4},
		{-1, 1, -math.Pi / 4},
	} {
		if got := FastAtan2Fine(tc.y, tc.x); math.Abs(float64(got)-tc.want) > 0.005 {
			t.Errorf("FastAtan2Fine(%v, %v) = %v, want %v", tc.y, tc.x, got, tc.want)
		}
		if got := FastAtan2(tc.y, tc.x); math.Abs(float64(got)-tc.want) > 0.02 {
			t.Errorf("FastAtan2(%v, %v) = %v, want %v", tc.y, tc.x, got, tc.want)
		}
	}
}
