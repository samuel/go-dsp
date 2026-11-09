package dsp

import (
	"math"
	"math/cmplx"
	"slices"
	"testing"
)

// TestFFTShiftOrder checks the rotation against the frequency each bin is
// supposed to carry afterwards, which is the whole reason the function exists.
// A shifted transform of length n has bin i at (i - n/2)*rate/n, so bin n/2 is
// DC and the bins increase monotonically across the slice.
func TestFFTShiftOrder(t *testing.T) {
	for _, n := range []int{2, 4, 8, 16, 64, 1024} {
		// Fill each bin with the frequency index the unshifted transform gives
		// it: 0..n/2-1 for the positive half, then -n/2..-1.
		x := make([]complex128, n)
		for i := range x {
			k := i
			if i >= n/2 {
				k = i - n
			}
			x[i] = complex(float64(k), 0)
		}
		FFTShift(x)

		for i, v := range x {
			want := float64(i - n/2)
			if real(v) != want {
				t.Fatalf("n=%d: bin %d carries %g, want %g", n, i, real(v), want)
			}
		}
		if real(x[n/2]) != 0 {
			t.Errorf("n=%d: DC is not at the middle bin", n)
		}
		if !slices.IsSortedFunc(x, func(a, b complex128) int {
			switch {
			case real(a) < real(b):
				return -1
			case real(a) > real(b):
				return 1
			}
			return 0
		}) {
			t.Errorf("n=%d: the shifted bins are not monotonic", n)
		}
	}
}

// TestFFTShiftRoundTrip covers the odd lengths too, where FFTShift and
// IFFTShift are genuinely different rotations and neither is its own inverse.
func TestFFTShiftRoundTrip(t *testing.T) {
	for n := range 65 {
		want := make([]complex64, n)
		for i := range want {
			want[i] = complex(float32(i+1), float32(-i))
		}
		got := slices.Clone(want)

		FFTShift(got)
		IFFTShift(got)
		if !slices.Equal(got, want) {
			t.Fatalf("n=%d: shift then unshift did not round-trip", n)
		}

		IFFTShift(got)
		FFTShift(got)
		if !slices.Equal(got, want) {
			t.Fatalf("n=%d: unshift then shift did not round-trip", n)
		}

		// An even length rotates by n/2 either way, so the two are the same
		// operation and each undoes itself. An odd one must not claim that,
		// except that a length below two rotates by nothing at all.
		FFTShift(got)
		FFTShift(got)
		selfInverse := slices.Equal(got, want)
		if want := n%2 == 0 || n < 2; selfInverse != want {
			t.Errorf("n=%d: FFTShift self-inverse = %v, want %v", n, selfInverse, want)
		}
		copy(got, want)
	}
}

// TestFFTShiftAgainstReference checks the in-place rotation against the obvious
// out-of-place one, which is the version that is clearly right and the version
// the three-reversal trick has to reproduce.
func TestFFTShiftAgainstReference(t *testing.T) {
	ref := func(x []complex128, k int) []complex128 {
		n := len(x)
		out := make([]complex128, n)
		for i := range x {
			out[(i+k)%n] = x[i]
		}
		return out
	}
	for n := 1; n <= 33; n++ {
		x := make([]complex128, n)
		for i := range x {
			x[i] = complex(float64(i), 0)
		}
		for _, tc := range []struct {
			name string
			fn   func([]complex128)
			k    int
		}{
			{"FFTShift", FFTShift[complex128], n / 2},
			{"IFFTShift", IFFTShift[complex128], n - n/2},
		} {
			got := slices.Clone(x)
			tc.fn(got)
			if want := ref(x, tc.k); !slices.Equal(got, want) {
				t.Errorf("n=%d %s: got %v, want %v", n, tc.name, got, want)
			}
		}
	}
}

// TestFFTShiftLocatesATone ties the rotation to a real transform: a complex
// exponential at a known negative frequency has to land in the bin the shifted
// layout says it should, which is what a two-sided spectrogram depends on.
func TestFFTShiftLocatesATone(t *testing.T) {
	const n = 256
	f, err := NewFFT[complex128](n)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []int{-n / 2, -37, -1, 0, 1, 37, n/2 - 1} {
		in := make([]complex128, n)
		for i := range in {
			// The phase argument is reduced in integers, so it stays exact
			// however long the frame is.
			num := (((k % n) + n) % n * i) % n
			th := 2 * math.Pi * float64(num) / float64(n)
			in[i] = complex(math.Cos(th), math.Sin(th))
		}
		out := make([]complex128, n)
		f.Forward(out, in)
		FFTShift(out)

		peak := 0
		for i, v := range out {
			if cmplx.Abs(v) > cmplx.Abs(out[peak]) {
				peak = i
			}
		}
		if want := k + n/2; peak != want {
			t.Errorf("tone at bin %d peaked at row %d, want %d", k, peak, want)
		}
	}
}
