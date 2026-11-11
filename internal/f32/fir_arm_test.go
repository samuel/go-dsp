package f32

import (
	"math"
	"testing"
)

// TestFIRDispatch runs the reference comparisons with NEON on and off, so a
// host with NEON covers the fallback too.
func TestFIRDispatch(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		t.Run("FIR", TestFIRMatchesReference)
		t.Run("CFIRRealPerPart", TestCFIRRealIsFIRPerPart)
		t.Run("Dot", TestDotMatchesReference)
		t.Run("CDotRealPerPart", TestCDotRealIsDotPerPart)
	})
}

// BenchmarkFIRNoNEON runs the FIR and dot-product benchmarks on the
// references, for comparison with the NEON leaves.
func BenchmarkFIRNoNEON(b *testing.B) {
	saved := haveNEON
	haveNEON = false
	defer func() { haveNEON = saved }()
	b.Run("FIR", BenchmarkFIR)
	b.Run("Dot", BenchmarkDot)
}

// TestFIRSignedZero checks that the kernels start their sums from +0 as the
// references do: products that are all -0 must sum to +0, not -0.
func TestFIRSignedZero(t *testing.T) {
	negZero := float32(math.Copysign(0, -1))
	simdTest(t, func(t *testing.T) {
		for _, n := range []int{1, 2, 3, 4, 5, 8, 16, 17, 21} {
			const taps = 3
			h := []float32{1, 2, 3}
			hc := []complex64{1, 2, 3}
			w := make([]float32, n+taps-1)
			wc := make([]complex64, n+taps-1)
			for i := range w {
				w[i] = negZero
				wc[i] = complex(negZero, negZero)
			}
			got := make([]float32, n)
			gotC := make([]complex64, n)
			want := make([]float32, n)
			wantC := make([]complex64, n)

			FIR(got, w, h, 1)
			firF32Go(want, w, h, 1)
			for k := range want {
				if !sameBits(got[k], want[k]) {
					t.Fatalf("FIR n=%d [%d] = %v, reference %v", n, k, got[k], want[k])
				}
			}
			CFIRReal(gotC, wc, h, 1)
			cfirRealGo(wantC, wc, h, 1)
			for k := range wantC {
				if !sameC64(gotC[k], wantC[k]) {
					t.Fatalf("CFIRReal n=%d [%d] = %v, reference %v", n, k, gotC[k], wantC[k])
				}
			}
			CFIR(gotC, wc, hc, 1)
			cfirGo(wantC, wc, hc, 1)
			for k := range wantC {
				if !sameC64(gotC[k], wantC[k]) {
					t.Fatalf("CFIR n=%d [%d] = %v, reference %v", n, k, gotC[k], wantC[k])
				}
			}

			c := make([]float32, n)
			for i := range c {
				c[i] = 1
			}
			if got, want := Dot(w, c), dotF32Go(w[:n], c); !sameBits(got, want) {
				t.Fatalf("Dot n=%d = %v, reference %v", n, got, want)
			}
			if got, want := CDotReal(wc, c), cdotRealGo(wc[:n], c); !sameC64(got, want) {
				t.Fatalf("CDotReal n=%d = %v, reference %v", n, got, want)
			}
		}
	})
}
