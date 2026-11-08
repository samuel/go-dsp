package f32

import "testing"

// TestCMulReal sweeps every length across a couple of blocks, so that when
// CMulReal gains its shuffle-based vector version, a mishandled tail cannot
// hide: a test at one convenient length would not see it.
//
// The comparison is exact: each component is one multiply, so a tolerance would
// only hide a wrong answer.
func TestCMulReal(t *testing.T) {
	for n := range 200 {
		src := make([]complex64, n)
		mul := make([]float32, n)
		for i := range src {
			src[i] = complex(float32(i)*0.5-3, float32(i)*-0.25+1)
			mul[i] = float32(i%17) - 8
		}
		dst := make([]complex64, n)
		CMulReal(dst, src, mul)
		for i, v := range src {
			want := complex(real(v)*mul[i], imag(v)*mul[i])
			if dst[i] != want {
				t.Fatalf("n=%d: dst[%d] = %v, want %v", n, i, dst[i], want)
			}
		}

		// Unaligned on all three slices at once.
		usrc := make([]complex64, n+1)[1:]
		umul := make([]float32, n+1)[1:]
		udst := make([]complex64, n+1)[1:]
		copy(usrc, src)
		copy(umul, mul)
		CMulReal(udst, usrc, umul)
		for i := range src {
			if udst[i] != dst[i] {
				t.Fatalf("n=%d unaligned: dst[%d] = %v, want %v", n, i, udst[i], dst[i])
			}
		}
	}
}

// TestCMulRealMismatchedLengths pins the contract of the one kernel here with
// three slices: the shortest bounds the work, nothing past it is written.
func TestCMulRealMismatchedLengths(t *testing.T) {
	const sentinel = complex64(12345)
	for _, tc := range []struct{ dst, src, mul int }{
		{0, 8, 8}, {8, 0, 8}, {8, 8, 0},
		{3, 40, 40}, {40, 3, 40}, {40, 40, 3},
		{17, 16, 33}, {33, 17, 16}, {16, 33, 17},
	} {
		src := make([]complex64, tc.src)
		for i := range src {
			src[i] = complex(float32(i+1), float32(-i))
		}
		mul := make([]float32, tc.mul)
		for i := range mul {
			mul[i] = 2
		}
		dst := make([]complex64, tc.dst)
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
