package dsp

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"slices"
	"testing"
)

func fftInput(n int, seed uint64) []complex128 {
	r := rand.New(rand.NewPCG(seed, 0x5eed))
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	return x
}

func narrow(x []complex128) []complex64 {
	y := make([]complex64, len(x))
	for i, v := range x {
		y[i] = complex64(v)
	}
	return y
}

// TestFFTMatchesDFT is the test that pins the butterfly ordering and the
// bit-reversal permutation: the fast transform has to agree with the definition
// at every length, not only at the one someone happened to try.
func TestFFTMatchesDFT(t *testing.T) {
	for n := 1; n <= 1024; n <<= 1 {
		x := fftInput(n, uint64(n))
		want := make([]complex128, n)
		DFT(want, x)

		f, err := NewFFT[complex128](n)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]complex128, n)
		f.Forward(got, x)
		for k := range got {
			if cmplx.Abs(got[k]-want[k]) > 1e-11*float64(n) {
				t.Fatalf("n=%d bin %d: got %v, want %v", n, k, got[k], want[k])
			}
		}

		// complex64 has to work too, at its own much coarser tolerance.
		f32, err := NewFFT[complex64](n)
		if err != nil {
			t.Fatal(err)
		}
		got32 := make([]complex64, n)
		f32.Forward(got32, narrow(x))
		for k := range got32 {
			if cmplx.Abs(complex128(got32[k])-want[k]) > 1e-4*float64(n) {
				t.Fatalf("complex64 n=%d bin %d: got %v, want %v", n, k, got32[k], want[k])
			}
		}
	}
}

// TestFFTInPlace checks the documented promise that input and output may be the
// same slice, which the bit-reversal permutation is what makes delicate.
func TestFFTInPlace(t *testing.T) {
	const n = 256
	x := fftInput(n, 7)
	want := make([]complex128, n)
	DFT(want, x)

	f, _ := NewFFT[complex128](n)
	got := append([]complex128(nil), x...)
	f.Forward(got, got)
	for k := range got {
		if cmplx.Abs(got[k]-want[k]) > 1e-9 {
			t.Fatalf("bin %d: got %v, want %v", k, got[k], want[k])
		}
	}
}

// TestFFTRoundTrip checks the 1/n scaling and the reversal the inverse is built
// from, which is the part that would survive a wrong sign unnoticed.
func TestFFTRoundTrip(t *testing.T) {
	for n := 1; n <= 512; n <<= 1 {
		x := fftInput(n, uint64(n)+99)
		f, _ := NewFFT[complex128](n)
		spec := make([]complex128, n)
		back := make([]complex128, n)
		f.Forward(spec, x)
		f.Inverse(back, spec)
		for i := range back {
			if cmplx.Abs(back[i]-x[i]) > 1e-13*float64(n) {
				t.Fatalf("n=%d sample %d: got %v, want %v", n, i, back[i], x[i])
			}
		}
	}
}

// TestFFTParseval checks the normalization against a property of the transform
// itself, so it needs no reference implementation to be right.
func TestFFTParseval(t *testing.T) {
	const n = 1024
	x := fftInput(n, 3)
	f, _ := NewFFT[complex128](n)
	spec := make([]complex128, n)
	f.Forward(spec, x)

	var time, freq float64
	for _, v := range x {
		time += real(v)*real(v) + imag(v)*imag(v)
	}
	for _, v := range spec {
		freq += real(v)*real(v) + imag(v)*imag(v)
	}
	freq /= n
	if math.Abs(time-freq) > 1e-9*time {
		t.Errorf("sum|x|^2 = %v, (1/n)sum|X|^2 = %v", time, freq)
	}
}

// TestFFTKnownSignals checks the three cases whose transforms are exact. The
// bin-centered sinusoid is the important one: it is the direct experimental
// check that a coherent tone leaks nothing, which is the whole basis of
// measuring a stopband far below any window's sidelobes.
func TestFFTKnownSignals(t *testing.T) {
	const n = 1024
	f, _ := NewFFT[complex128](n)
	spec := make([]complex128, n)

	impulse := make([]complex128, n)
	impulse[0] = 1
	f.Forward(spec, impulse)
	for k, v := range spec {
		if v != 1 {
			t.Fatalf("impulse bin %d: got %v, want exactly 1", k, v)
		}
	}

	constant := make([]complex128, n)
	for i := range constant {
		constant[i] = 1
	}
	f.Forward(spec, constant)
	if cmplx.Abs(spec[0]-complex(n, 0)) > 1e-9 {
		t.Errorf("constant bin 0: got %v, want %d", spec[0], n)
	}
	for k := 1; k < n; k++ {
		if cmplx.Abs(spec[k]) > 1e-10 {
			t.Errorf("constant bin %d: got %v, want 0", k, spec[k])
		}
	}

	// A tone at an exact bin, with the phase argument reduced in integers so
	// the stimulus itself is not the limit. Everything off the two occupied
	// bins has to fall to the arithmetic floor rather than to a window's
	// sidelobe.
	const k = 137
	tone := make([]complex128, n)
	for i := range tone {
		_, c := math.Sincos(2 * math.Pi * float64(k*i%n) / float64(n))
		tone[i] = complex(c, 0) // a real cosine, so bins k and n-k
	}
	f.Forward(spec, tone)
	peak := cmplx.Abs(spec[k])
	for j, v := range spec {
		if j == k || j == n-k {
			continue
		}
		if db := 20 * math.Log10(cmplx.Abs(v)/peak); db > -280 {
			t.Errorf("coherent tone leaked %.1f dB into bin %d", db, j)
		}
	}
}

func TestFFTValidation(t *testing.T) {
	for _, n := range []int{0, -1, 3, 6, 100} {
		if _, err := NewFFT[complex128](n); err == nil {
			t.Errorf("NewFFT(%d) did not report an error", n)
		}
	}
	f, _ := NewFFT[complex128](8)
	if f.Len() != 8 {
		t.Errorf("Len = %d, want 8", f.Len())
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected a panic for a short slice")
			}
		}()
		f.Forward(make([]complex128, 8), make([]complex128, 4))
	}()
}

// TestTransformRealZeroPads checks both that the widening is right and that a
// short input is padded, which is how the window sidelobe test resolves a peak
// that falls between bins.
func TestTransformRealZeroPads(t *testing.T) {
	const n, m = 64, 16
	x := make([]float32, m)
	for i := range x {
		x[i] = float32(math.Sin(float64(i) * 0.3))
	}
	padded := make([]complex128, n)
	for i, v := range x {
		padded[i] = complex(float64(v), 0)
	}
	want := make([]complex128, n)
	DFT(want, padded)

	f, _ := NewFFT[complex128](n)
	got := make([]complex128, n)
	ForwardReal(f, got, x)
	for k := range got {
		if cmplx.Abs(got[k]-want[k]) > 1e-6 {
			t.Fatalf("bin %d: got %v, want %v", k, got[k], want[k])
		}
	}
}

func TestDFTBin(t *testing.T) {
	const n = 48 // not a power of two, which is the point
	x := fftInput(n, 11)
	want := make([]complex128, n)
	DFT(want, x)
	for k := range n {
		if got := DFTBin(x, k); cmplx.Abs(got-want[k]) > 1e-12 {
			t.Errorf("bin %d: got %v, want %v", k, got, want[k])
		}
	}
	// Bins repeat, so an index outside [0, n) names an existing one.
	if got := DFTBin(x, n+5); cmplx.Abs(got-want[5]) > 1e-12 {
		t.Errorf("bin n+5: got %v, want %v", got, want[5])
	}
	if got := DFTBin(x, -1); cmplx.Abs(got-want[n-1]) > 1e-12 {
		t.Errorf("bin -1: got %v, want %v", got, want[n-1])
	}
	if got := DFTBin([]complex128{}, 0); got != 0 {
		t.Errorf("empty src: got %v, want 0", got)
	}
}

// TestDFTBinReal checks the scaling that is the whole difference between the
// two: a real cosine splits evenly between bins k and n-k, so the bin carries
// half of it and DFTBinReal doubles it back. The test is that a cosine of a
// known amplitude reads that amplitude, which is what dspviz measures every
// level against.
func TestDFTBinReal(t *testing.T) {
	// Not a power of two, and an odd bin, so nothing lines up conveniently.
	const n, k = 48, 7
	const amp = 0.375
	for _, phase := range []float64{0, math.Pi / 4, -math.Pi / 3} {
		x := make([]float64, n)
		for i := range x {
			x[i] = amp * math.Cos(2*math.Pi*float64((k*i)%n)/n+phase)
		}
		got := DFTBinReal(x, k)
		if d := math.Abs(cmplx.Abs(got) - amp); d > 1e-12 {
			t.Errorf("phase %v: magnitude %v, want %v", phase, cmplx.Abs(got), amp)
		}
		if d := math.Abs(cmplx.Phase(got) - phase); d > 1e-12 {
			t.Errorf("phase %v: got %v", phase, cmplx.Phase(got))
		}

		// It is exactly twice DFTBin, which is the relationship the doc states.
		cx := make([]complex128, n)
		for i, v := range x {
			cx[i] = complex(v, 0)
		}
		want := 2 * DFTBin(cx, k) / complex(n, 0)
		if cmplx.Abs(got-want) > 1e-12 {
			t.Errorf("phase %v: %v, want twice the scaled DFTBin %v", phase, got, want)
		}
	}

	// A tone at another bin does not leak into this one, since it completes a
	// whole number of cycles too.
	x := make([]float64, n)
	for i := range x {
		x[i] = math.Cos(2 * math.Pi * float64((11*i)%n) / n)
	}
	if got := cmplx.Abs(DFTBinReal(x, k)); got > 1e-14 {
		t.Errorf("a tone at bin 11 reads %v at bin %d", got, k)
	}

	// float32 samples, and the empty case.
	x32 := make([]float32, n)
	for i := range x32 {
		x32[i] = float32(amp * math.Cos(2*math.Pi*float64((k*i)%n)/n))
	}
	if got := cmplx.Abs(DFTBinReal(x32, k)); math.Abs(got-amp) > 1e-6 {
		t.Errorf("float32 magnitude %v, want %v", got, amp)
	}
	if got := DFTBinReal([]float64{}, 0); got != 0 {
		t.Errorf("empty src: got %v, want 0", got)
	}
}

func BenchmarkFFT(b *testing.B) {
	for _, sz := range []struct {
		name string
		n    int
	}{{"256", 256}, {"1K", 1024}, {"64K", 65536}} {
		b.Run(sz.name, func(b *testing.B) {
			f, _ := NewFFT[complex128](sz.n)
			x := fftInput(sz.n, 1)
			out := make([]complex128, sz.n)
			b.SetBytes(int64(sz.n * 16))
			b.ResetTimer()
			for range b.N {
				f.Forward(out, x)
			}
		})
	}
}

// TestDFTInPlace checks that transforming a slice onto itself gives what
// transforming it into a separate one does.
//
// Every bin reads the whole of src, so an in-place transform that wrote bins as
// it computed them would feed the later ones their own output. DFT used to
// stage through scratch unconditionally, which made that safe and made every
// separate-slice caller pay for an allocation and a copy; the scratch is now a
// branch, so the aliased half needs a test of its own.
func TestDFTInPlace(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5, 8, 16, 31} {
		src := make([]complex128, n)
		for i := range src {
			src[i] = complex(math.Cos(float64(i)*0.7), math.Sin(float64(i)*0.31))
		}

		want := make([]complex128, n)
		DFT(want, src)

		got := slices.Clone(src)
		DFT(got, got)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("n=%d bin %d: in place %v, separate %v", n, i, got[i], want[i])
			}
		}

		// A separate destination must not have been disturbed by the change:
		// src comes back untouched and dst holds the transform.
		keep := slices.Clone(src)
		dst := make([]complex128, n)
		DFT(dst, src)
		if !slices.Equal(src, keep) {
			t.Fatalf("n=%d: DFT modified its source", n)
		}
		if !slices.Equal(dst, want) {
			t.Fatalf("n=%d: separate destination gave %v, want %v", n, dst, want)
		}
	}
}
