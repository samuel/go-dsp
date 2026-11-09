//go:build goexperiment.simd

package encoding

import (
	"simd/archsimd"
	"testing"
)

// TestU8ToF32Widths exercises each vector width directly rather than only the
// one this CPU dispatches to. The widths differ in how many bytes one
// VPMOVSXBD consumes and therefore in how the rest of a sixteen-byte load is
// rotated down, so a wrong rotate constant would show up here and nowhere else.
func TestU8ToF32Widths(t *testing.T) {
	widths := []struct {
		name string
		ok   bool
		fn   func(dst []float32, src []byte)
	}{
		{"128", archsimd.X86.AVX(), u8ToF32x4},
		{"256", archsimd.X86.AVX(), u8ToF32x8},
		{"512", archsimd.X86.AVX512(), u8ToF32x16},
	}
	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit vectors not available", w.name)
			}
			// Past three full blocks of the largest variant, which is 128
			// samples, so that the main loop, the sixteen-wide step and every
			// remainder length are covered — including the overlapping final
			// block that replaces the scalar tail.
			for n := range 400 {
				src := make([]byte, n)
				expected := make([]float32, n)
				for i := range src {
					src[i] = byte(i * 7)
					expected[i] = u8ToF32Ref(src[i])
				}

				dst := make([]float32, n)
				w.fn(dst, src)
				for i, v := range expected {
					if dst[i] != v {
						t.Fatalf("len %d: dst[%d] = %v, want %v", n, i, dst[i], v)
					}
				}

				// Unaligned input and output
				ui := make([]byte, n+1)[1:]
				copy(ui, src)
				uo := make([]float32, n+1)[1:]
				w.fn(uo, ui)
				for i, v := range expected {
					if uo[i] != v {
						t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, uo[i], v)
					}
				}
			}

			// Every byte value, so a sign extension that mishandles 0x80..0xff
			// cannot hide.
			all := make([]byte, 256)
			for i := range all {
				all[i] = byte(i)
			}
			out := make([]float32, 256)
			w.fn(out, all)
			for i, v := range all {
				if want := u8ToF32Ref(v); out[i] != want {
					t.Fatalf("src %d: got %v want %v", v, out[i], want)
				}
			}
		})
	}
}
