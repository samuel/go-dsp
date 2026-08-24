package f64

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

// vscaleF64Plain, vscaleF64Block8 and vscaleF64Block32 exist only so
// BenchmarkVScaleF64 can measure the block size Scale chose against the
// alternatives. Keeping them here rather than in the doc comment means the
// numbers quoted there can be rechecked on any machine.
func vscaleF64Plain(dst, src []float64, scale float64) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for i, v := range src {
		dst[i] = v * scale
	}
}

func vscaleF64Block8(dst, src []float64, scale float64) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for len(src) >= 8 {
		in := (*[8]float64)(src[0:8])
		out := (*[8]float64)(dst[0:8])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		src, dst = src[8:], dst[8:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

func vscaleF64Block32(dst, src []float64, scale float64) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for len(src) >= 32 {
		in := (*[32]float64)(src[0:32])
		out := (*[32]float64)(dst[0:32])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		out[8], out[9], out[10], out[11] = in[8]*scale, in[9]*scale, in[10]*scale, in[11]*scale
		out[12], out[13], out[14], out[15] = in[12]*scale, in[13]*scale, in[14]*scale, in[15]*scale
		out[16], out[17], out[18], out[19] = in[16]*scale, in[17]*scale, in[18]*scale, in[19]*scale
		out[20], out[21], out[22], out[23] = in[20]*scale, in[21]*scale, in[22]*scale, in[23]*scale
		out[24], out[25], out[26], out[27] = in[24]*scale, in[25]*scale, in[26]*scale, in[27]*scale
		out[28], out[29], out[30], out[31] = in[28]*scale, in[29]*scale, in[30]*scale, in[31]*scale
		src, dst = src[32:], dst[32:]
	}
	if len(src) >= 16 {
		in := (*[16]float64)(src[0:16])
		out := (*[16]float64)(dst[0:16])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		out[8], out[9], out[10], out[11] = in[8]*scale, in[9]*scale, in[10]*scale, in[11]*scale
		out[12], out[13], out[14], out[15] = in[12]*scale, in[13]*scale, in[14]*scale, in[15]*scale
		src, dst = src[16:], dst[16:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

// TestVScaleF64 sweeps every length across a couple of blocks, which is what
// catches a mishandled tail: checking one convenient length hides exactly the
// bug the 16- and 8-element steps can introduce.
//
// The comparison is exact. A multiply is one operation, so there is no rounding
// to allow for and a tolerance would only hide a wrong answer.
func TestVScaleF64(t *testing.T) {
	const scale = 0.375 // a power-of-two sum, so every product is exact
	for n := range 80 {
		src := make([]float64, n)
		for i := range src {
			src[i] = float64(i) - 40.5
		}
		dst := make([]float64, n)
		Scale(dst, src, scale)
		for i := range src {
			if want := src[i] * scale; dst[i] != want {
				t.Fatalf("n=%d: dst[%d] = %v, want %v", n, i, dst[i], want)
			}
		}
	}
}

// TestVScaleF64AgainstPlain checks the unrolled blocks against the obvious loop
// over lengths that straddle every step boundary.
func TestVScaleF64AgainstPlain(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, n := range []int{0, 1, 7, 8, 9, 15, 16, 17, 23, 24, 31, 32, 33, 100, 1023, 1024} {
		src := make([]float64, n)
		for i := range src {
			src[i] = r.NormFloat64()
		}
		scale := r.NormFloat64()

		got := make([]float64, n)
		want := make([]float64, n)
		Scale(got, src, scale)
		vscaleF64Plain(want, src, scale)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("n=%d: element %d = %v, want %v", n, i, got[i], want[i])
			}
		}
	}
}

// TestVScaleF64MismatchedLengths is the package-wide contract: process the
// smaller of the two and leave the rest of the output alone.
func TestVScaleF64MismatchedLengths(t *testing.T) {
	const sentinel = 12345.0
	for _, tc := range []struct{ in, out int }{
		{0, 8}, {8, 0}, {3, 40}, {40, 3}, {17, 16}, {16, 17}, {33, 32},
	} {
		src := make([]float64, tc.in)
		for i := range src {
			src[i] = float64(i + 1)
		}
		dst := make([]float64, tc.out)
		for i := range dst {
			dst[i] = sentinel
		}
		Scale(dst, src, 2)

		n := min(tc.in, tc.out)
		for i := range n {
			if want := src[i] * 2; dst[i] != want {
				t.Errorf("in=%d out=%d: element %d = %v, want %v", tc.in, tc.out, i, dst[i], want)
			}
		}
		for i := n; i < tc.out; i++ {
			if dst[i] != sentinel {
				t.Errorf("in=%d out=%d: wrote past the src at %d", tc.in, tc.out, i)
			}
		}
	}
}

// TestVScaleF64InPlace covers the documented aliasing case, which is how a
// caller normalizes a buffer it already has.
func TestVScaleF64InPlace(t *testing.T) {
	for _, n := range []int{1, 8, 16, 17, 40, 1000} {
		x := make([]float64, n)
		want := make([]float64, n)
		for i := range x {
			x[i] = float64(i) - 3.5
			want[i] = x[i] * 0.25
		}
		Scale(x, x, 0.25)
		for i := range want {
			if x[i] != want[i] {
				t.Fatalf("n=%d: in place, element %d = %v, want %v", n, i, x[i], want[i])
			}
		}
	}
}

// TestVScaleF64Specials passes the values a signal actually contains at its
// edges. Zero times infinity is NaN and that is the right answer, not a case to
// special-case away.
func TestVScaleF64Specials(t *testing.T) {
	src := []float64{0, -0, 1, -1, math.Inf(1), math.Inf(-1), math.NaN(), math.MaxFloat64, math.SmallestNonzeroFloat64}
	for _, scale := range []float64{0, 1, -1, 2, 0.5, math.Inf(1), math.NaN()} {
		got := make([]float64, len(src))
		Scale(got, src, scale)
		for i, v := range src {
			want := v * scale
			if math.IsNaN(want) {
				if !math.IsNaN(got[i]) {
					t.Errorf("scale=%v: %v gave %v, want NaN", scale, v, got[i])
				}
				continue
			}
			if got[i] != want {
				t.Errorf("scale=%v: %v gave %v, want %v", scale, v, got[i], want)
			}
			if math.Signbit(got[i]) != math.Signbit(want) {
				t.Errorf("scale=%v: %v gave %v, want the sign of %v", scale, v, got[i], want)
			}
		}
	}
}

func BenchmarkVScaleF64(b *testing.B) {
	for _, n := range []int{64, 1024, 2048, 4096, 8192, 1 << 14, 1 << 16} {
		// Carve both slices out of one arena at differing offsets, so the
		// benchmark measures the loop rather than 4K aliasing between two
		// same-sized allocations.
		arena := make([]float64, 2*n+64)
		src := arena[:n]
		dst := arena[n+7 : 2*n+7]
		for i := range src {
			src[i] = float64(i)
		}

		for _, v := range []struct {
			name string
			fn   func([]float64, []float64, float64)
		}{
			{"VScaleF64", Scale},
			{"plain", vscaleF64Plain},
			{"block8", vscaleF64Block8},
			{"block32", vscaleF64Block32},
		} {
			b.Run(fmt.Sprintf("%s/%d", v.name, n), func(b *testing.B) {
				b.SetBytes(int64(n * 8))
				for b.Loop() {
					v.fn(dst, src, 1.0000001)
				}
			})
		}
	}
}
