//go:build goexperiment.simd

package pcm

import (
	"simd/archsimd"
	"testing"
)

// TestSIMDWidthsMatchReference drives each archsimd width directly rather
// than only the one this CPU dispatches to.
func TestSIMDWidthsMatchReference(t *testing.T) {
	avx, avx2, avx512 := archsimd.X86.AVX(), archsimd.X86.AVX2(), archsimd.X86.AVX512()
	for _, w := range []struct {
		name     string
		ok       bool
		bytes    func([]float32, []byte, uint8)
		i16      func([]float32, []byte)
		f32ToI16 func([]byte, []float32)
	}{
		{"128", avx, bytesToF32x4, i16ToF32x4, f32ToI16x4},
		{"256", avx2, bytesToF32x8, i16ToF32x8, f32ToI16x8},
		{"512", avx512, bytesToF32x16, i16ToF32x16, nil},
	} {
		t.Run(w.name, func(t *testing.T) {
			if !w.ok {
				t.Skipf("%s-bit kernels not available", w.name)
			}
			// The width kernels take equal lengths; the wrappers below bound
			// them as the family functions do.
			u8 := func(dst []float32, src []byte) {
				n := min(len(dst), len(src))
				w.bytes(dst[:n], src[:n], 128)
				archsimd.ClearAVXUpperBits()
			}
			i8 := func(dst []float32, src []byte) {
				n := min(len(dst), len(src))
				w.bytes(dst[:n], src[:n], 0)
				archsimd.ClearAVXUpperBits()
			}
			i16 := func(dst []float32, src []byte) {
				n := min(len(dst), len(src)/2)
				w.i16(dst[:n], src[:2*n])
				archsimd.ClearAVXUpperBits()
			}
			checkDecodeKernel(t, decodeKernel[float32]{"u8ToF32", 1, u8, u8ToFloatRef[float32]})
			checkDecodeKernel(t, decodeKernel[float32]{"i8ToF32", 1, i8, i8ToFloatRef[float32]})
			checkDecodeKernel(t, decodeKernel[float32]{"i16LEToF32", 2, i16, i16LEToFloatRef[float32]})
			if w.f32ToI16 != nil {
				enc := func(dst []byte, src []float32) {
					n := min(len(dst)/2, len(src))
					w.f32ToI16(dst[:2*n], src[:n])
					archsimd.ClearAVXUpperBits()
				}
				checkEncodeKernel(t, encodeKernel[float32]{"f32ToI16LE", 2, enc, floatToI16LERef[float32]})
			}
		})
	}
}
