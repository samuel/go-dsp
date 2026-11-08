//go:build amd64

package f32

//go:generate go run math32_avo_amd64.go -out math32_asm_amd64.s -stubs math32_asm_stub_amd64.go

// The assembly fallbacks, used whenever the archsimd implementations in
// math32_simd_amd64.go are unavailable: builds without GOEXPERIMENT=simd (the
// default) and CPUs without AVX, which archsimd cannot target because it is
// VEX-encoded even at 128 bits. See simdWidth.
//
// The dispatch lives here, not in the assembly, so tests can drive each path
// directly; math32_avo_amd64.go generates the leaf functions.

func vscaleF32Asm(dst, src []float32, scale float32) {
	if useAVX {
		vscaleF32AVX(dst, src, scale)
		return
	}
	vscaleF32SSE2(dst, src, scale)
}

func vmaxF32Asm(src []float32) float32 {
	if useAVX {
		return vmaxF32AVX(src)
	}
	return vmaxF32SSE2(src)
}

func vminF32Asm(src []float32) float32 {
	if useAVX {
		return vminF32AVX(src)
	}
	return vminF32SSE2(src)
}

func vabsC64Asm(dst []float32, src []complex64) {
	if useAVX {
		vabsC64AVX(dst, src)
		return
	}
	vabsC64SSE2(dst, src)
}
