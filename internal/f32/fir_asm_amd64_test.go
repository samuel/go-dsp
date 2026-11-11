//go:build amd64

package f32

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// The assembly dispatchers are called directly, so these run under
// GOEXPERIMENT=simd too, and simdTest drives the AVX and SSE2 leaves in turn.

// sameBitsOrNaN is sameBits with any two NaNs equal: which NaN an operation
// propagates depends on operand order, which the reference does not define.
func sameBitsOrNaN(a, b float32) bool {
	return sameBits(a, b) || (math.IsNaN(float64(a)) && math.IsNaN(float64(b)))
}

func sameC64OrNaN(a, b complex64) bool {
	return sameBitsOrNaN(real(a), real(b)) && sameBitsOrNaN(imag(a), imag(b))
}

func TestFIRAsm(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(11))
		const guard = 4
		counts := make([]int, 0, 72)
		for n := range 68 {
			counts = append(counts, n)
		}
		counts = append(counts, 100, 127, 200)
		for _, taps := range firTapCounts {
			for stride := 1; stride <= 3; stride++ {
				h := randF32(r, taps)
				hc := randC64(r, taps)
				for _, n := range counts {
					off := n % 3
					wl := firWorkLen(n, taps, stride) + off
					w := randF32(r, wl)[off:]
					wc := randC64(r, wl)[off:]
					name := strconv.Itoa(taps) + "x" + strconv.Itoa(stride) + " n=" + strconv.Itoa(n)

					// dst starts off any alignment, with guards either side.
					buf := make([]float32, n+2*guard)
					got := buf[guard+off : guard+off+n]
					want := make([]float32, n)
					firF32Asm(got, w, h, stride)
					firF32Go(want, w, h, stride)
					for k := range want {
						if !sameBits(got[k], want[k]) {
							t.Fatalf("FIR %s: [%d] = %v, reference %v", name, k, got[k], want[k])
						}
					}
					checkGuards(t, "FIR "+name, buf, guard+off, n)

					bufC := make([]complex64, n+2*guard)
					gotC := bufC[guard+off : guard+off+n]
					wantC := make([]complex64, n)
					cfirRealAsm(gotC, wc, h, stride)
					cfirRealGo(wantC, wc, h, stride)
					for k := range wantC {
						if !sameC64(gotC[k], wantC[k]) {
							t.Fatalf("CFIRReal %s: [%d] = %v, reference %v", name, k, gotC[k], wantC[k])
						}
					}
					checkGuardsC(t, "CFIRReal "+name, bufC, guard+off, n)

					clear(bufC)
					cfirAsm(gotC, wc, hc, stride)
					cfirGo(wantC, wc, hc, stride)
					for k := range wantC {
						if !sameC64(gotC[k], wantC[k]) {
							t.Fatalf("CFIR %s: [%d] = %v, reference %v", name, k, gotC[k], wantC[k])
						}
					}
					checkGuardsC(t, "CFIR "+name, bufC, guard+off, n)
				}
			}
		}
	})
}

func checkGuards(t *testing.T, name string, buf []float32, start, n int) {
	t.Helper()
	for k, v := range buf {
		if (k < start || k >= start+n) && v != 0 {
			t.Fatalf("%s: wrote outside dst at %d", name, k-start)
		}
	}
}

func checkGuardsC(t *testing.T, name string, buf []complex64, start, n int) {
	t.Helper()
	for k, v := range buf {
		if (k < start || k >= start+n) && v != 0 {
			t.Fatalf("%s: wrote outside dst at %d", name, k-start)
		}
	}
}

// firEdgeValues are values random inputs never produce: signed zeros,
// infinities, subnormals, and magnitudes whose products overflow or underflow.
var firEdgeValues = []float32{
	0, float32(math.Copysign(0, -1)), 1, -1,
	math.MaxFloat32, -math.MaxFloat32,
	math.SmallestNonzeroFloat32, -math.SmallestNonzeroFloat32,
	1e20, -1e20, 1e-20, -1e-20,
	float32(math.Inf(1)), float32(math.Inf(-1)),
}

func edgeF32(r *rand.Rand, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = firEdgeValues[r.Intn(len(firEdgeValues))]
	}
	return out
}

func edgeC64(r *rand.Rand, n int) []complex64 {
	out := make([]complex64, n)
	for i := range out {
		out[i] = complex(firEdgeValues[r.Intn(len(firEdgeValues))], firEdgeValues[r.Intn(len(firEdgeValues))])
	}
	return out
}

// TestFIRAsmEdge checks the leaves against the references on edge values. The
// sums of signed zeros and of products that round to zero are where a kernel
// that starts its accumulator differently, or negates instead of subtracting,
// would differ.
func TestFIRAsmEdge(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(12))
		for _, taps := range []int{1, 2, 3, 9} {
			for _, n := range []int{1, 3, 7, 8, 9, 33, 70} {
				for range 20 {
					h := edgeF32(r, taps)
					hc := edgeC64(r, taps)
					w := edgeF32(r, firWorkLen(n, taps, 1))
					wc := edgeC64(r, firWorkLen(n, taps, 1))

					got := make([]float32, n)
					want := make([]float32, n)
					firF32Asm(got, w, h, 1)
					firF32Go(want, w, h, 1)
					for k := range want {
						if !sameBitsOrNaN(got[k], want[k]) {
							t.Fatalf("FIR taps=%d n=%d: [%d] = %v, reference %v", taps, n, k, got[k], want[k])
						}
					}

					gotC := make([]complex64, n)
					wantC := make([]complex64, n)
					cfirRealAsm(gotC, wc, h, 1)
					cfirRealGo(wantC, wc, h, 1)
					for k := range wantC {
						if !sameC64OrNaN(gotC[k], wantC[k]) {
							t.Fatalf("CFIRReal taps=%d n=%d: [%d] = %v, reference %v", taps, n, k, gotC[k], wantC[k])
						}
					}

					cfirAsm(gotC, wc, hc, 1)
					cfirGo(wantC, wc, hc, 1)
					for k := range wantC {
						if !sameC64OrNaN(gotC[k], wantC[k]) {
							t.Fatalf("CFIR taps=%d n=%d: [%d] = %v, reference %v", taps, n, k, gotC[k], wantC[k])
						}
					}
				}
			}
		}
	})
}

func TestDotAsm(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(13))
		for n := range 201 {
			for off := range 4 {
				x := randF32(r, n+off)[off:]
				c := randF32(r, n+off)[off:]
				if got, want := dotF32Asm(x, c), dotF32Go(x, c); !sameBits(got, want) {
					t.Fatalf("Dot n=%d off=%d = %v, reference %v", n, off, got, want)
				}
				xc := randC64(r, n+off)[off:]
				if got, want := cdotRealAsm(xc, c), cdotRealGo(xc, c); !sameC64(got, want) {
					t.Fatalf("CDotReal n=%d off=%d = %v, reference %v", n, off, got, want)
				}
			}
		}
		// Edge values, and a lone nonzero product in each position, so a lane
		// dropped or misplaced in the combine cannot hide.
		for _, n := range []int{8, 15, 16, 17, 64, 71} {
			for range 50 {
				x, c, xc := edgeF32(r, n), edgeF32(r, n), edgeC64(r, n)
				if got, want := dotF32Asm(x, c), dotF32Go(x, c); !sameBitsOrNaN(got, want) {
					t.Fatalf("Dot edge n=%d = %v, reference %v", n, got, want)
				}
				if got, want := cdotRealAsm(xc, c), cdotRealGo(xc, c); !sameC64OrNaN(got, want) {
					t.Fatalf("CDotReal edge n=%d = %v, reference %v", n, got, want)
				}
			}
			for i := range n {
				x := make([]float32, n)
				c := make([]float32, n)
				xc := make([]complex64, n)
				x[i], c[i], xc[i] = 3, float32(i+1), complex(5, float32(-i-2))
				if got, want := dotF32Asm(x, c), dotF32Go(x, c); !sameBits(got, want) || got == 0 {
					t.Fatalf("Dot one-hot n=%d at %d = %v, reference %v", n, i, got, want)
				}
				if got, want := cdotRealAsm(xc, c), cdotRealGo(xc, c); !sameC64(got, want) || got == 0 {
					t.Fatalf("CDotReal one-hot n=%d at %d = %v, reference %v", n, i, got, want)
				}
			}
		}
	})
}
