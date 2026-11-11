//go:build goexperiment.simd

package f32

import (
	"math/rand"
	"simd/archsimd"
	"strconv"
	"testing"
)

// firSIMDWidths are the dispatch widths the FIR and dot-product kernels
// select between; 0 is the pre-AVX assembly dispatch.
var firSIMDWidths = []struct {
	width int
	ok    bool
}{
	{512, archsimd.X86.AVX512()},
	{256, archsimd.X86.AVX()},
	{128, archsimd.X86.AVX()},
	{0, true},
}

// forEachSIMDWidth runs fn with simdWidth forced to each width the CPU can
// execute, skipping the others, and restores it afterwards.
func forEachSIMDWidth(t *testing.T, fn func(t *testing.T)) {
	for _, w := range firSIMDWidths {
		t.Run(strconv.Itoa(w.width), func(t *testing.T) {
			if !w.ok {
				t.Skipf("%d-bit vectors not available", w.width)
			}
			saved := simdWidth
			simdWidth = w.width
			defer func() { simdWidth = saved }()
			fn(t)
		})
	}
}

// firSIMDCounts covers every output count through the widest kernel's
// unrolled block, its four-vector step and every narrower remainder, plus a
// few counts past two blocks.
func firSIMDCounts() []int {
	counts := make([]int, 0, 150)
	for n := range 140 {
		counts = append(counts, n)
	}
	return append(counts, 191, 255, 256, 257, 300)
}

// firSentinel marks dst elements the kernel must overwrite, and guard
// elements past dst it must not touch.
const firSentinel = 12345

// TestFIRSIMDWidths compares FIR, CFIRReal and CFIR with their references bit
// for bit at every dispatch width. Strided calls run the reference, so they
// get fewer counts.
func TestFIRSIMDWidths(t *testing.T) {
	const guard = 5
	forEachSIMDWidth(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(21))
		for _, taps := range firTapCounts {
			h := randF32(r, taps)
			hc := randC64(r, taps)
			for _, stride := range []int{1, 2, 3} {
				counts := firSIMDCounts()
				if stride > 1 {
					counts = []int{0, 1, 5, 17, 70}
				}
				for _, n := range counts {
					off := (n + taps) & 3
					wl := firWorkLen(n, taps, stride) + off
					w := randF32(r, wl)[off:]
					wc := randC64(r, wl)[off:]
					// dst unaligned too, independently of work.
					doff := n & 1

					got := make([]float32, doff+n+guard)
					for i := range got {
						got[i] = firSentinel
					}
					want := make([]float32, n)
					FIR(got[doff:doff+n], w, h, stride)
					firF32Go(want, w, h, stride)
					checkF32(t, "FIR", taps, stride, n, got[doff:], want, guard)

					gotC := make([]complex64, doff+n+guard)
					wantC := make([]complex64, n)
					for i := range gotC {
						gotC[i] = firSentinel
					}
					CFIRReal(gotC[doff:doff+n], wc, h, stride)
					cfirRealGo(wantC, wc, h, stride)
					checkC64(t, "CFIRReal", taps, stride, n, gotC[doff:], wantC, guard)

					for i := range gotC {
						gotC[i] = firSentinel
					}
					CFIR(gotC[doff:doff+n], wc, hc, stride)
					cfirGo(wantC, wc, hc, stride)
					checkC64(t, "CFIR", taps, stride, n, gotC[doff:], wantC, guard)
				}
			}
		}
	})
}

func checkF32(t *testing.T, name string, taps, stride, n int, got, want []float32, guard int) {
	t.Helper()
	for k := range want {
		if !sameBits(got[k], want[k]) {
			t.Fatalf("%s taps=%d stride=%d n=%d: [%d] = %v, reference %v", name, taps, stride, n, k, got[k], want[k])
		}
	}
	for k := n; k < n+guard; k++ {
		if got[k] != firSentinel {
			t.Fatalf("%s taps=%d stride=%d n=%d: wrote past dst at %d", name, taps, stride, n, k)
		}
	}
}

func checkC64(t *testing.T, name string, taps, stride, n int, got, want []complex64, guard int) {
	t.Helper()
	for k := range want {
		if !sameC64(got[k], want[k]) {
			t.Fatalf("%s taps=%d stride=%d n=%d: [%d] = %v, reference %v", name, taps, stride, n, k, got[k], want[k])
		}
	}
	for k := n; k < n+guard; k++ {
		if got[k] != firSentinel {
			t.Fatalf("%s taps=%d stride=%d n=%d: wrote past dst at %d", name, taps, stride, n, k)
		}
	}
}

// TestDotSIMDWidths compares Dot and CDotReal with their references bit for
// bit at every dispatch width, at every length through several unrolled
// blocks and off every alignment.
func TestDotSIMDWidths(t *testing.T) {
	forEachSIMDWidth(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(22))
		for n := range 300 {
			for off := range 4 {
				x := randF32(r, n+off+3)[off:]
				c := randF32(r, n)
				if got, want := Dot(x, c), dotF32Go(x[:n], c); !sameBits(got, want) {
					t.Fatalf("Dot n=%d off=%d = %v, reference %v", n, off, got, want)
				}
				xc := randC64(r, n+off+3)[off:]
				if got, want := CDotReal(xc, c), cdotRealGo(xc[:n], c); !sameC64(got, want) {
					t.Fatalf("CDotReal n=%d off=%d = %v, reference %v", n, off, got, want)
				}
			}
		}
	})
}

// TestDotSIMDLanes puts a single non-zero product in each position of a few
// blocks, so an accumulator combined out of combine8's order, or a part read
// from the wrong lane, changes the result.
func TestDotSIMDLanes(t *testing.T) {
	forEachSIMDWidth(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(23))
		const n = 37
		base := randF32(r, n)
		c := randF32(r, n)
		xc := randC64(r, n)
		for i := range n {
			x := append([]float32(nil), base...)
			x[i] *= 1 << 20
			if got, want := Dot(x, c), dotF32Go(x, c); !sameBits(got, want) {
				t.Fatalf("Dot lane %d = %v, reference %v", i, got, want)
			}
			y := append([]complex64(nil), xc...)
			y[i] = complex(real(y[i])*(1<<20), imag(y[i])*(1<<10))
			if got, want := CDotReal(y, c), cdotRealGo(y, c); !sameC64(got, want) {
				t.Fatalf("CDotReal lane %d = %v, reference %v", i, got, want)
			}
		}
	})
}
