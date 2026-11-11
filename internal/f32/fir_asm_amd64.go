//go:build amd64

package f32

// The FIR and dot-product assembly dispatch, used by fir_nosimd_amd64.go and
// by fir_simd_amd64.go on a CPU archsimd cannot target. See math32_asm_amd64.go.
// The FIR leaves handle stride 1, where the samples of adjacent outputs are
// contiguous; a decimating stride runs the reference.

func firF32Asm(dst, work, taps []float32, stride int) {
	switch {
	case stride != 1:
		firF32Go(dst, work, taps, stride)
	case useAVX:
		firF32AVX(dst, work, taps)
	default:
		firF32SSE2(dst, work, taps)
	}
}

func cfirRealAsm(dst, work []complex64, taps []float32, stride int) {
	switch {
	case stride != 1:
		cfirRealGo(dst, work, taps, stride)
	case useAVX:
		cfirRealAVX(dst, work, taps)
	default:
		cfirRealSSE2(dst, work, taps)
	}
}

func cfirAsm(dst, work, taps []complex64, stride int) {
	switch {
	case stride != 1:
		cfirGo(dst, work, taps, stride)
	case useAVX:
		cfirAVX(dst, work, taps)
	default:
		cfirSSE2(dst, work, taps)
	}
}

func dotF32Asm(x, c []float32) float32 {
	if useAVX {
		return dotF32AVX(x, c)
	}
	return dotF32SSE2(x, c)
}

func cdotRealAsm(x []complex64, c []float32) complex64 {
	if useAVX {
		return cdotRealAVX(x, c)
	}
	return cdotRealSSE2(x, c)
}
