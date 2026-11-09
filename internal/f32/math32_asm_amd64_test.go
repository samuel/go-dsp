//go:build amd64

package f32

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestVScaleF32Asm and TestVMaxF32Asm exercise the generated assembly leaves
// directly. simdTest only toggles the feature flags the dispatcher reads, so
// each width still needs to be called by name: the block sizes differ, and so
// do the tails.
func TestVScaleF32Asm(t *testing.T) {
	impls := []struct {
		name string
		ok   bool
		fn   func(dst, src []float32, scale float32)
	}{
		{"avx", useAVX, vscaleF32AVX},
		{"sse2", true, vscaleF32SSE2}, // SSE2 is part of the amd64 baseline
		{"scalar", true, vscaleF32Scalar},
	}
	const scale = 1.0 / 256.0
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			if !im.ok {
				t.Skip("not available on this CPU")
			}
			for n := range 200 {
				src := make([]float32, n)
				expected := make([]float32, n)
				for i := range src {
					src[i] = float32(i)
					expected[i] = float32(i) * scale
				}

				dst := make([]float32, n)
				im.fn(dst, src, scale)
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d: dst[%d] = %v, want %v", n, i, dst[i], v)
					}
				}

				// Unaligned input and output
				unaligned := make([]float32, n+1)[1:]
				copy(unaligned, src)
				dst = make([]float32, n+1)[1:]
				im.fn(dst, unaligned, scale)
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, dst[i], v)
					}
				}
			}

			// Mismatched lengths: only min(len(src), len(dst)) elements
			// are read and written.
			src := make([]float32, 64)
			for i := range src {
				src[i] = float32(i + 1)
			}
			for _, outLen := range []int{40, 64, 96} {
				dst := make([]float32, outLen)
				im.fn(dst, src, scale)
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
		})
	}
}

func TestVMaxF32Asm(t *testing.T) {
	impls := []struct {
		name string
		ok   bool
		fn   func(src []float32) float32
	}{
		{"avx", useAVX, vmaxF32AVX},
		{"sse2", true, vmaxF32SSE2},
		{"scalar", true, vmaxF32Scalar},
	}
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			if !im.ok {
				t.Skip("not available on this CPU")
			}
			if mx := im.fn(nil); mx != float32(math.Inf(-1)) {
				t.Fatalf("empty src: got %v, want -Inf", mx)
			}
			r := rand.New(rand.NewPCG(1, 2))
			for n := range 200 {
				src := make([]float32, n)
				expected := float32(math.Inf(-1))
				for i := range src {
					src[i] = r.Float32() - 0.5
					expected = max(expected, src[i])
				}
				if mx := im.fn(src); mx != expected {
					t.Fatalf("len %d: got %v, want %v", n, mx, expected)
				}

				unaligned := make([]float32, n+1)[1:]
				copy(unaligned, src)
				if mx := im.fn(unaligned); mx != expected {
					t.Fatalf("len %d unaligned: got %v, want %v", n, mx, expected)
				}
			}
			// The maximum in each individual lane, so a dropped accumulator or
			// a botched final reduction cannot hide.
			for i := range 512 {
				src := make([]float32, 512)
				src[i] = 1.0
				if mx := im.fn(src); mx != 1.0 {
					t.Fatalf("expected 1.0 got %v at position %d", mx, i)
				}
			}
		})
	}
}

// The min counterpart of TestVMaxF32Asm; see the note there.
func TestVMinF32Asm(t *testing.T) {
	impls := []struct {
		name string
		ok   bool
		fn   func(src []float32) float32
	}{
		{"avx", useAVX, vminF32AVX},
		{"sse2", true, vminF32SSE2},
		{"scalar", true, vminF32Scalar},
	}
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			if !im.ok {
				t.Skip("not available on this CPU")
			}
			if mn := im.fn(nil); mn != float32(math.Inf(1)) {
				t.Fatalf("empty src: got %v, want +Inf", mn)
			}
			r := rand.New(rand.NewPCG(1, 2))
			for n := range 200 {
				src := make([]float32, n)
				expected := float32(math.Inf(1))
				for i := range src {
					src[i] = r.Float32() - 0.5
					expected = min(expected, src[i])
				}
				if mn := im.fn(src); mn != expected {
					t.Fatalf("len %d: got %v, want %v", n, mn, expected)
				}

				unaligned := make([]float32, n+1)[1:]
				copy(unaligned, src)
				if mn := im.fn(unaligned); mn != expected {
					t.Fatalf("len %d unaligned: got %v, want %v", n, mn, expected)
				}
			}
			// The minimum in each individual lane, so a dropped accumulator or
			// a botched final reduction cannot hide.
			for i := range 512 {
				src := make([]float32, 512)
				src[i] = -1.0
				if mn := im.fn(src); mn != -1.0 {
					t.Fatalf("expected -1.0 got %v at position %d", mn, i)
				}
			}
		})
	}
}

// The CAbs counterpart of the two tests above.
func TestVAbsC64Asm(t *testing.T) {
	impls := []struct {
		name string
		ok   bool
		fn   func(dst []float32, src []complex64)
	}{
		{"avx", useAVX, vabsC64AVX},
		{"sse2", true, vabsC64SSE2},
		{"scalar", true, vabsC64Scalar},
	}
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			if !im.ok {
				t.Skip("not available on this CPU")
			}
			im.fn(nil, nil)
			r := rand.New(rand.NewPCG(3, 4))
			for n := range 200 {
				src := make([]complex64, n)
				expected := make([]float32, n)
				for i := range src {
					src[i] = complex(r.Float32()*4-2, r.Float32()*4-2)
					expected[i] = absC64Ref(src[i])
				}

				dst := make([]float32, n)
				im.fn(dst, src)
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d: dst[%d] = %v, want %v", n, i, dst[i], v)
					}
				}

				ui := make([]complex64, n+1)[1:]
				copy(ui, src)
				uo := make([]float32, n+1)[1:]
				im.fn(uo, ui)
				for i, v := range expected {
					if uo[i] != v {
						t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, uo[i], v)
					}
				}
			}

			// Mismatched lengths: only min(len(src), len(dst)) elements
			// are read and written.
			src := make([]complex64, 64)
			for i := range src {
				src[i] = complex(float32(i+1), float32(i+2))
			}
			for _, outLen := range []int{40, 64, 96} {
				dst := make([]float32, outLen)
				im.fn(dst, src)
				for i, v := range dst {
					var want float32
					if i < len(src) {
						want = absC64Ref(src[i])
					}
					if v != want {
						t.Fatalf("src 64 dst %d: dst[%d] = %v, want %v", outLen, i, v, want)
					}
				}
			}
		})
	}
}

// absEdgeInput returns complex values that exercise overflow, non-finites,
// signed zero and denormals, padded past a couple of blocks so the main loop,
// the intermediate step and the remainder all see them.
func absEdgeInput() []complex64 {
	vals := []float32{
		0, float32(math.Copysign(0, -1)),
		1, -1,
		math.MaxFloat32, -math.MaxFloat32,
		math.SmallestNonzeroFloat32, -math.SmallestNonzeroFloat32,
		1e20, -1e20, // squares overflow float32
		1e-20, -1e-20, // squares underflow to zero
		float32(math.Inf(1)), float32(math.Inf(-1)),
		float32(math.NaN()),
		3, 4, // exactly 5
	}
	var src []complex64
	for _, re := range vals {
		for _, im := range vals {
			src = append(src, complex(re, im))
		}
	}
	for len(src)%128 != 0 {
		src = append(src, complex(1, 1))
	}
	return src
}

// absEdgeCheck compares got against the scalar reference bit for bit, treating
// any two NaNs as equal.
func absEdgeCheck(t *testing.T, name string, src []complex64, got []float32) {
	t.Helper()
	want := make([]float32, len(src))
	vabsC64Scalar(want, src)
	for i := range want {
		w, g := want[i], got[i]
		if math.Float32bits(w) == math.Float32bits(g) {
			continue
		}
		if math.IsNaN(float64(w)) && math.IsNaN(float64(g)) {
			continue
		}
		t.Fatalf("%s: src %v: got %v (%#08x) want %v (%#08x)",
			name, src[i], g, math.Float32bits(g), w, math.Float32bits(w))
	}
}

// TestVAbsC64AsmEdge checks the assembly leaves against the scalar reference on
// values the random inputs never produce.
func TestVAbsC64AsmEdge(t *testing.T) {
	src := absEdgeInput()
	impls := []struct {
		name string
		ok   bool
		fn   func([]float32, []complex64)
	}{
		{"avx", useAVX, vabsC64AVX},
		{"sse2", true, vabsC64SSE2},
		{"scalar", true, vabsC64Scalar},
	}
	for _, im := range impls {
		if !im.ok {
			t.Run(im.name, func(t *testing.T) { t.Skip("not available on this CPU") })
			continue
		}
		t.Run(im.name, func(t *testing.T) {
			got := make([]float32, len(src))
			im.fn(got, src)
			absEdgeCheck(t, im.name, src, got)
		})
	}
}
