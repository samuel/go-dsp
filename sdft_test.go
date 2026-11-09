package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

func sdftInput(n int) []complex128 {
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(math.Sin(float64(i)*0.7)+0.3, math.Cos(float64(i)*0.21))
	}
	return x
}

// TestSDFTMatchesDFT checks that after n samples the sliding DFT has converged
// on the same bin the direct transform gives.
func TestSDFTMatchesDFT(t *testing.T) {
	const n = 32
	x := sdftInput(n)
	for k := range n {
		sd, err := NewSDFT[complex128](k, n, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got complex128
		for _, v := range x {
			got = sd.FilterOne(v)
		}
		want := DFTBin(x, k)
		if cmplx.Abs(got-want) > 1e-9 {
			t.Fatalf("bin %d: got %v, want %v", k, got, want)
		}
	}
}

// TestSDFTWindowed checks the frequency-domain window: applying one is a
// combination of adjacent bins, so the result has to equal the same combination
// of direct DFT bins, including where the window straddles bin 0 or bin n-1 and
// the indices wrap.
func TestSDFTWindowed(t *testing.T) {
	const n = 32
	x := sdftInput(n)
	for _, w := range [][]float64{HannFreqCoeff(), HammingFreqCoeff(), BlackmanFreqCoeff()} {
		for _, k := range []int{0, 1, 5, n - 2, n - 1} {
			sd, err := NewSDFT[complex128](k, n, w)
			if err != nil {
				t.Fatal(err)
			}
			var got complex128
			for _, v := range x {
				got = sd.FilterOne(v)
			}
			var want complex128
			for i, c := range w {
				want += complex(c, 0) * DFTBin(x, sdftBin(k-len(w)/2+i, n))
			}
			if cmplx.Abs(got-want) > 1e-9 {
				t.Fatalf("window %v bin %d: got %v, want %v", w, k, got, want)
			}
		}
	}
}

// TestSDFTBinWraps checks that a bin index outside [0, n) names the same bin as
// its reduction, which the constructor used to get wrong for anything more than
// one period out.
func TestSDFTBinWraps(t *testing.T) {
	const n = 16
	x := sdftInput(n)
	for _, k := range []int{-33, -17, -1, 3, 19, 51} {
		sd, err := NewSDFT[complex128](k, n, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got complex128
		for _, v := range x {
			got = sd.FilterOne(v)
		}
		want := DFTBin(x, sdftBin(k, n))
		if cmplx.Abs(got-want) > 1e-9 {
			t.Fatalf("bin %d (reduced %d): got %v, want %v", k, sdftBin(k, n), got, want)
		}
	}
}

func TestSDFTWidthsAgree(t *testing.T) {
	const n = 32
	x := sdftInput(n)
	sd, err := NewSDFT[complex128](3, n, HannFreqCoeff())
	if err != nil {
		t.Fatal(err)
	}
	sd32, err := NewSDFT[complex64](3, n, HannFreqCoeff())
	if err != nil {
		t.Fatal(err)
	}
	var got complex128
	var got32 complex64
	for _, v := range x {
		got = sd.FilterOne(v)
		got32 = sd32.FilterOne(complex64(v))
	}
	if cmplx.Abs(complex128(got32)-got) > 1e-4 {
		t.Errorf("float32 %v, float64 %v", got32, got)
	}
}

// TestNewSDFTRejects covers the sizes that used to leave a filter that could
// not run: n of zero made the first FilterOne divide by len(sd.x), and a window
// wider than the transform combines bins that do not exist.
func TestNewSDFTRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		k, n int
		w    []float64
	}{
		{"zero n", 0, 0, nil},
		{"negative n", 1, -4, nil},
		{"window wider than n", 0, 2, HannFreqCoeff()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSDFT[complex128](tc.k, tc.n, tc.w); err == nil {
				t.Error("NewSDFT[complex128]: expected an error")
			}
			if _, err := NewSDFT[complex64](tc.k, tc.n, tc.w); err == nil {
				t.Error("NewSDFT[complex64]: expected an error")
			}
		})
	}
}

// TestSDFTReset checks that a reset transform reproduces its first run.
func TestSDFTReset(t *testing.T) {
	const n = 16
	x := sdftInput(n)
	sd, err := NewSDFT[complex128](3, n, nil)
	if err != nil {
		t.Fatal(err)
	}
	var first complex128
	for _, v := range x {
		first = sd.FilterOne(v)
	}
	sd.Reset()
	var again complex128
	for _, v := range x {
		again = sd.FilterOne(v)
	}
	if again != first {
		t.Errorf("after Reset got %v, want %v", again, first)
	}
}
