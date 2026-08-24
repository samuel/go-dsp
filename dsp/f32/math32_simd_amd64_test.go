//go:build goexperiment.simd

package f32

import (
	"math"
	"math/rand/v2"
	"os"
	"simd/archsimd"
	"testing"

	"github.com/samuel/go-dsp/dsp/internal/cpu"
	"github.com/samuel/go-dsp/dsp/internal/view"
)

// TestVScaleF32Widths exercises each vector width directly rather than only the
// one this CPU dispatches to. Every length across a couple of unrolled blocks is
// checked against a scalar reference so that mishandled tails cannot hide, and
// each width has a different block size and so a different tail.
func TestVScaleF32Widths(t *testing.T) {
	widths := []struct {
		name string
		ok   bool
		fn   func(dst, src []float32, scale float32)
	}{
		{"128", archsimd.X86.AVX(), vscaleF32x4},
		{"256", archsimd.X86.AVX(), vscaleF32x8},
		{"512", archsimd.X86.AVX512(), vscaleF32x16},
	}
	const scale = 1.0 / 256.0
	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit vectors not available", w.name)
			}
			for n := range 200 {
				src := make([]float32, n)
				expected := make([]float32, n)
				for i := range src {
					src[i] = float32(i)
					expected[i] = float32(i) * scale
				}

				dst := make([]float32, n)
				w.fn(dst, src, scale)
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d: dst doesn't match expected:\n%+v\n%+v", n, dst, expected)
					}
				}

				// Unaligned input and output
				unaligned := make([]float32, n+1)[1:]
				copy(unaligned, src)
				dst = make([]float32, n+1)[1:]
				w.fn(dst, unaligned, scale)
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d unaligned: dst doesn't match expected:\n%+v\n%+v", n, dst, expected)
					}
				}
			}
		})
	}
}

// TestVMaxF32Widths exercises each vector width directly, for the reasons given
// on TestVScaleF32Widths.
func TestVMaxF32Widths(t *testing.T) {
	widths := []struct {
		name string
		ok   bool
		fn   func(src []float32) float32
	}{
		{"128", archsimd.X86.AVX(), vmaxF32x4},
		{"256", archsimd.X86.AVX(), vmaxF32x8},
		{"512", archsimd.X86.AVX512(), vmaxF32x16},
	}
	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit vectors not available", w.name)
			}
			r := rand.New(rand.NewPCG(1, 2))
			// Past three full blocks of the largest variant, which is 128
			// floats, so that the main loop, the intermediate step and every
			// remainder length are all covered.
			for n := range 400 {
				src := make([]float32, n)
				expected := float32(math.Inf(-1))
				for i := range src {
					src[i] = r.Float32() - 0.5
					expected = max(expected, src[i])
				}

				if mx := w.fn(src); mx != expected {
					t.Fatalf("len %d: expected %v got %v", n, expected, mx)
				}

				// Unaligned input
				unaligned := make([]float32, n+1)[1:]
				copy(unaligned, src)
				if mx := w.fn(unaligned); mx != expected {
					t.Fatalf("len %d unaligned: expected %v got %v", n, expected, mx)
				}

				// The remainder is folded in by re-reading the last full
				// vector, so put the maximum where only that read can find it.
				if n > 0 {
					tail := make([]float32, n)
					tail[n-1] = 1.0
					if mx := w.fn(tail); mx != 1.0 {
						t.Fatalf("len %d last element: expected 1 got %v", n, mx)
					}
				}
			}

			// The maximum in each individual lane, so that a dropped
			// accumulator or a botched final reduction cannot hide.
			for i := range 1024 {
				src := make([]float32, 1024)
				src[i] = 1.0
				if mx := w.fn(src); mx != 1.0 {
					t.Fatalf("expected 1.0 got %f at position %d", mx, i)
				}
			}
		})
	}
}

// TestVMinF32Widths exercises each vector width directly, for the reasons given
// on TestVScaleF32Widths.
func TestVMinF32Widths(t *testing.T) {
	widths := []struct {
		name string
		ok   bool
		fn   func(src []float32) float32
	}{
		{"128", archsimd.X86.AVX(), vminF32x4},
		{"256", archsimd.X86.AVX(), vminF32x8},
		{"512", archsimd.X86.AVX512(), vminF32x16},
	}
	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit vectors not available", w.name)
			}
			r := rand.New(rand.NewPCG(1, 2))
			// Past three full blocks of the largest variant, which is 128
			// floats, so that the main loop, the intermediate step and every
			// remainder length are all covered.
			for n := range 400 {
				src := make([]float32, n)
				expected := float32(math.Inf(1))
				for i := range src {
					src[i] = r.Float32() - 0.5
					expected = min(expected, src[i])
				}

				if mn := w.fn(src); mn != expected {
					t.Fatalf("len %d: expected %v got %v", n, expected, mn)
				}

				// Unaligned input
				unaligned := make([]float32, n+1)[1:]
				copy(unaligned, src)
				if mn := w.fn(unaligned); mn != expected {
					t.Fatalf("len %d unaligned: expected %v got %v", n, expected, mn)
				}

				// The remainder is folded in by re-reading the last full
				// vector, so put the minimum where only that read can find it.
				if n > 0 {
					tail := make([]float32, n)
					tail[n-1] = -1.0
					if mn := w.fn(tail); mn != -1.0 {
						t.Fatalf("len %d last element: expected -1 got %v", n, mn)
					}
				}
			}

			// The minimum in each individual lane, so that a dropped
			// accumulator or a botched final reduction cannot hide.
			for i := range 1024 {
				src := make([]float32, 1024)
				src[i] = -1.0
				if mn := w.fn(src); mn != -1.0 {
					t.Fatalf("expected -1.0 got %f at position %d", mn, i)
				}
			}
		})
	}
}

// TestVAbsC64Widths exercises each vector width directly, for the reasons given
// on TestVScaleF32Widths. The deinterleave differs per width here — an index
// vector at 512 bits, VPERM2F128 plus VSHUFPS at 256, VSHUFPS alone at 128 — so
// this is the one place a lane picked out of the wrong half would show up.
func TestVAbsC64Widths(t *testing.T) {
	widths := []struct {
		name string
		ok   bool
		fn   func(out, in []float32)
	}{
		{"128", archsimd.X86.AVX(), vabsC64x4},
		{"256", archsimd.X86.AVX(), vabsC64x8},
		{"512", archsimd.X86.AVX512(), vabsC64x16},
	}
	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit vectors not available", w.name)
			}
			r := rand.New(rand.NewPCG(3, 4))
			// Past three full blocks of the widest variant, which is 64
			// magnitudes.
			for n := range 200 {
				src := make([]complex64, n)
				expected := make([]float32, n)
				for i := range src {
					src[i] = complex(r.Float32()*4-2, r.Float32()*4-2)
					expected[i] = absC64Ref(src[i])
				}

				dst := make([]float32, n)
				w.fn(dst, view.C64ToF32(src))
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d: dst[%d] = %v, want %v", n, i, dst[i], v)
					}
				}

				// Unaligned input and output
				ui := make([]complex64, n+1)[1:]
				copy(ui, src)
				uo := make([]float32, n+1)[1:]
				w.fn(uo, view.C64ToF32(ui))
				for i, v := range expected {
					if uo[i] != v {
						t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, uo[i], v)
					}
				}
			}

			// A single non-zero part in each lane of three full blocks, so a
			// deinterleave that crosses lanes cannot hide behind symmetric
			// data.
			for i := range 192 {
				src := make([]complex64, 192)
				if i%2 == 0 {
					src[i/2] = complex(float32(i+1), 0)
				} else {
					src[i/2] = complex(0, float32(i+1))
				}
				dst := make([]float32, 192)
				w.fn(dst, view.C64ToF32(src))
				for j := range dst {
					if want := absC64Ref(src[j]); dst[j] != want {
						t.Fatalf("lane %d: dst[%d] = %v, want %v", i, j, dst[j], want)
					}
				}
			}
		})
	}
}

// TestVAbsC64Idx checks the index tables the 512-bit deinterleave is built on
// without needing a CPU that can execute it.
//
// The tables are hand-written literals, which is exactly the kind of thing a
// typo hides in, so model what ConcatPermute is documented to do — index k
// picks element k of the concatenation of the two operands, x first — and
// check that the lanes named really are the real and imaginary parts of
// consecutive complex values.
func TestVAbsC64Idx(t *testing.T) {
	// The concatenation the indices address: two 512-bit vectors of complex64,
	// so 32 float32, holding complex values 0..15 as {re, im} = {2i, 2i + 1}.
	var xy [32]float32
	for i := range xy {
		xy[i] = float32(i)
	}
	for lane := range 16 {
		re := absEvenIdx[lane]
		im := absOddIdx[lane]
		if re >= 32 || im >= 32 {
			t.Fatalf("lane %d: index out of range (%d, %d)", lane, re, im)
		}
		if got, want := xy[re], float32(lane*2); got != want {
			t.Errorf("lane %d: real part comes from element %v, want %v", lane, got, want)
		}
		if got, want := xy[im], float32(lane*2+1); got != want {
			t.Errorf("lane %d: imaginary part comes from element %v, want %v", lane, got, want)
		}
	}
}

// TestSIMDWidth checks that the width selection landed somewhere sane for the
// CPU running the tests.
func TestSIMDWidth(t *testing.T) {
	switch simdWidth {
	case 0, 128, 256, 512:
	default:
		t.Fatalf("simdWidth = %d, want 0, 128, 256 or 512", simdWidth)
	}
	if simdWidth == 512 && !archsimd.X86.AVX512() {
		t.Fatal("selected 512-bit vectors without AVX-512")
	}
	// Every archsimd path, and ClearAVXUpperBits with it, is VEX-encoded, so
	// any non-zero width requires AVX. Selecting one without it would fault
	// with SIGILL rather than run slowly.
	if simdWidth > 0 && !archsimd.X86.AVX() {
		t.Fatalf("selected %d-bit vectors without AVX", simdWidth)
	}
	if simdWidth == 0 && archsimd.X86.AVX() && os.Getenv("X86VECTOR") == "" {
		t.Fatal("fell back to scalar despite AVX being available")
	}
	if haveSIMD() != (simdWidth > 0) {
		t.Fatalf("haveSIMD() = %v with simdWidth = %d", haveSIMD(), simdWidth)
	}
	t.Logf("simdWidth = %d (AVX=%v AVX512=%v, %+v)",
		simdWidth, archsimd.X86.AVX(), archsimd.X86.AVX512(), cpu.X86Signature())
}

// TestSIMDPreAVXFallback checks the pre-AVX path end to end by forcing the
// dispatch off archsimd, since no AVX-less CPU is available to run the tests
// on. It lands on the generated assembly, which dispatches AVX2/SSE2 itself;
// TestVScaleF32Asm, TestVMaxF32Asm, TestVMinF32Asm and TestVAbsC64Asm cover
// those leaves individually.
func TestSIMDPreAVXFallback(t *testing.T) {
	saved := simdWidth
	simdWidth = 0
	defer func() { simdWidth = saved }()

	if haveSIMD() {
		t.Fatal("haveSIMD() true at simdWidth 0")
	}
	const scale = 1.0 / 256.0
	r := rand.New(rand.NewPCG(11, 12))
	for n := range 200 {
		src := make([]float32, n)
		wantScaled := make([]float32, n)
		wantMax := float32(math.Inf(-1))
		wantMin := float32(math.Inf(1))
		for i := range src {
			src[i] = r.Float32() - 0.5
			wantScaled[i] = src[i] * scale
			wantMax = max(wantMax, src[i])
			wantMin = min(wantMin, src[i])
		}

		dst := make([]float32, n)
		Scale(dst, src, scale)
		for i, v := range wantScaled {
			if dst[i] != v {
				t.Fatalf("Scale len %d: dst[%d] = %v, want %v", n, i, dst[i], v)
			}
		}
		if mx := Max(src); mx != wantMax {
			t.Fatalf("Max len %d: got %v want %v", n, mx, wantMax)
		}
		if mn := Min(src); mn != wantMin {
			t.Fatalf("Min len %d: got %v want %v", n, mn, wantMin)
		}

		cin := make([]complex64, n)
		for i := range cin {
			cin[i] = complex(src[i], src[i]*0.5)
		}
		cout := make([]float32, n)
		CAbs(cout, cin)
		for i, v := range cin {
			if want := absC64Ref(v); cout[i] != want {
				t.Fatalf("CAbs len %d: dst[%d] = %v, want %v", n, i, cout[i], want)
			}
		}
	}

	// Mismatched lengths still only touch min(len(src), len(dst)).
	src := make([]float32, 64)
	for i := range src {
		src[i] = float32(i + 1)
	}
	for _, outLen := range []int{40, 64, 96} {
		dst := make([]float32, outLen)
		Scale(dst, src, scale)
		for i, v := range dst {
			var want float32
			if i < len(src) {
				want = src[i] * scale
			}
			if v != want {
				t.Fatalf("src 64 dst %d: dst[%d] = %v, want %v", outLen, i, v, want)
			}
		}
	}
}

// TestVAbsC64WidthsEdge checks each archsimd width against the scalar reference
// on values the random inputs never produce. The remainder is folded in by
// recomputing the last full vector, so the padding in absEdgeInput deliberately
// leaves every width an exact number of blocks and the lengths below cover the
// overlap.
func TestVAbsC64WidthsEdge(t *testing.T) {
	widths := []struct {
		name string
		ok   bool
		fn   func(out, in []float32)
	}{
		{"128", archsimd.X86.AVX(), vabsC64x4},
		{"256", archsimd.X86.AVX(), vabsC64x8},
		{"512", archsimd.X86.AVX512(), vabsC64x16},
	}
	full := absEdgeInput()
	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit vectors not available", w.name)
			}
			// Trim to lengths that leave a remainder as well as none, so the
			// overlapping final vector is exercised on these values too.
			for _, trim := range []int{0, 1, 3, 7, 15, 31} {
				src := full[:len(full)-trim]
				got := make([]float32, len(src))
				w.fn(got, view.C64ToF32(src))
				archsimd.ClearAVXUpperBits()
				absEdgeCheck(t, w.name, src, got)
			}
		})
	}
}
