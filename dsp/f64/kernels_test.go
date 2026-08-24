package f64

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestMaxMin sweeps every length, placing the extreme at every position so a
// dropped accumulator or mishandled tail in a future vector rung cannot hide.
func TestMaxMin(t *testing.T) {
	if got := Max(nil); !math.IsInf(got, -1) {
		t.Errorf("Max(nil) = %v, want -Inf", got)
	}
	if got := Min(nil); !math.IsInf(got, 1) {
		t.Errorf("Min(nil) = %v, want +Inf", got)
	}

	base := func(n int) []float64 {
		src := make([]float64, n)
		for i := range src {
			src[i] = float64(i%7) - 3
		}
		return src
	}
	for n := 1; n < 80; n++ {
		for p := range n {
			src := base(n)
			src[p] = 100
			if got := Max(src); got != 100 {
				t.Fatalf("n=%d peak at %d: Max = %v, want 100", n, p, got)
			}
			src = base(n)
			src[p] = -100
			if got := Min(src); got != -100 {
				t.Fatalf("n=%d trough at %d: Min = %v, want -100", n, p, got)
			}
		}
	}

	// Against a scalar reference over data with nothing convenient about it.
	r := rand.New(rand.NewPCG(7, 11))
	for _, n := range []int{1, 7, 8, 9, 15, 16, 17, 31, 32, 33, 1023, 1024} {
		src := make([]float64, n)
		for i := range src {
			src[i] = r.NormFloat64() * 1e6
		}
		wantMax, wantMin := math.Inf(-1), math.Inf(1)
		for _, v := range src {
			if v > wantMax {
				wantMax = v
			}
			if v < wantMin {
				wantMin = v
			}
		}
		if got := Max(src); got != wantMax {
			t.Fatalf("n=%d: Max = %v, want %v", n, got, wantMax)
		}
		if got := Min(src); got != wantMin {
			t.Fatalf("n=%d: Min = %v, want %v", n, got, wantMin)
		}
	}

	// A slice that is all one value, and the infinities, which are the two
	// empty-slice sentinels appearing as real data. NaN is deliberately not
	// covered: the doc comment leaves the result unspecified.
	flat := make([]float64, 40)
	for i := range flat {
		flat[i] = -2.5
	}
	if got := Max(flat); got != -2.5 {
		t.Errorf("Max(flat) = %v, want -2.5", got)
	}
	if got := Min(flat); got != -2.5 {
		t.Errorf("Min(flat) = %v, want -2.5", got)
	}
	if got := Max([]float64{math.Inf(-1), 3, math.Inf(1)}); !math.IsInf(got, 1) {
		t.Errorf("Max with infinities = %v, want +Inf", got)
	}
	if got := Min([]float64{math.Inf(-1), 3, math.Inf(1)}); !math.IsInf(got, -1) {
		t.Errorf("Min with infinities = %v, want -Inf", got)
	}
}

// TestCAbs sweeps every length across a couple of blocks. The data is integral,
// so re*re + im*im is exact and the comparison can be too: Go may contract that
// product and sum into one FMA and arm64 does, which is enough to make a
// reference written here disagree with the kernel on general data.
func TestCAbs(t *testing.T) {
	for n := range 80 {
		src := make([]complex128, n)
		for i := range src {
			src[i] = complex(float64(i%13-6), float64(i%7-3))
		}
		dst := make([]float64, n)
		CAbs(dst, src)
		for i, v := range src {
			re, im := real(v), imag(v)
			if want := math.Sqrt(re*re + im*im); dst[i] != want {
				t.Fatalf("n=%d: dst[%d] = %v, want %v", n, i, dst[i], want)
			}
		}
	}

	// General data against math.Hypot, which computes the same magnitude by a
	// scaled route. Only to a tolerance: the doc comment says the naive
	// expression is the contract, and the two differ in the last bits (and in
	// whether they overflow) by design.
	r := rand.New(rand.NewPCG(13, 17))
	src := make([]complex128, 200)
	for i := range src {
		src[i] = complex(r.NormFloat64()*1e3, r.NormFloat64()*1e-3)
	}
	dst := make([]float64, len(src))
	CAbs(dst, src)
	for i, v := range src {
		want := math.Hypot(real(v), imag(v))
		if math.Abs(dst[i]-want) > 1e-12*want {
			t.Fatalf("[%d] = %v, want about %v", i, dst[i], want)
		}
	}
}

// TestCAbsMismatchedLengths is the package-wide contract: the shorter slice
// bounds the work, and nothing past it is written.
func TestCAbsMismatchedLengths(t *testing.T) {
	const sentinel = 12345.0
	for _, tc := range []struct{ dst, src int }{
		{0, 8}, {8, 0}, {3, 40}, {40, 3}, {17, 16}, {16, 17},
	} {
		src := make([]complex128, tc.src)
		for i := range src {
			src[i] = complex(float64(3*(i+1)), float64(4*(i+1)))
		}
		dst := make([]float64, tc.dst)
		for i := range dst {
			dst[i] = sentinel
		}
		CAbs(dst, src)
		for i := range dst {
			want := sentinel
			if i < min(tc.dst, tc.src) {
				want = float64(5 * (i + 1)) // a 3-4-5 triangle, so exact
			}
			if dst[i] != want {
				t.Fatalf("dst %d src %d: dst[%d] = %v, want %v", tc.dst, tc.src, i, dst[i], want)
			}
		}
	}
}

// TestCMulReal sweeps every length across a couple of blocks, exactly: each
// component is one multiply, so there is no rounding to allow for.
func TestCMulReal(t *testing.T) {
	for n := range 80 {
		src := make([]complex128, n)
		mul := make([]float64, n)
		for i := range src {
			src[i] = complex(float64(i)*0.5-3, float64(i)*-0.25+1)
			mul[i] = float64(i%17) - 8
		}
		dst := make([]complex128, n)
		CMulReal(dst, src, mul)
		for i, v := range src {
			want := complex(real(v)*mul[i], imag(v)*mul[i])
			if dst[i] != want {
				t.Fatalf("n=%d: dst[%d] = %v, want %v", n, i, dst[i], want)
			}
		}
	}
}

// TestCMulRealMismatchedLengths covers the kernel with three slices to
// disagree about.
func TestCMulRealMismatchedLengths(t *testing.T) {
	const sentinel = complex128(12345)
	for _, tc := range []struct{ dst, src, mul int }{
		{0, 8, 8}, {8, 0, 8}, {8, 8, 0},
		{3, 40, 40}, {40, 3, 40}, {40, 40, 3},
		{17, 16, 33}, {33, 17, 16}, {16, 33, 17},
	} {
		src := make([]complex128, tc.src)
		for i := range src {
			src[i] = complex(float64(i+1), float64(-i))
		}
		mul := make([]float64, tc.mul)
		for i := range mul {
			mul[i] = 2
		}
		dst := make([]complex128, tc.dst)
		for i := range dst {
			dst[i] = sentinel
		}
		CMulReal(dst, src, mul)

		n := min(tc.dst, tc.src, tc.mul)
		for i := range dst {
			want := sentinel
			if i < n {
				want = complex(real(src[i])*mul[i], imag(src[i])*mul[i])
			}
			if dst[i] != want {
				t.Fatalf("dst %d src %d mul %d: dst[%d] = %v, want %v",
					tc.dst, tc.src, tc.mul, i, dst[i], want)
			}
		}
	}
}
