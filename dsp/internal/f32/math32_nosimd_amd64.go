//go:build amd64 && !goexperiment.simd

package f32

// Without GOEXPERIMENT=simd there is no archsimd, so amd64 runs the generated
// assembly: AVX1 dispatch with a fallback to SSE2, part of the amd64 baseline,
// so no scalar path is reachable. math32_avo_amd64.go must not emit AVX2; see useAVX.

// Scale calculates dst[i] = src[i] * scale.
func Scale(dst, src []float32, scale float32) {
	vscaleF32Asm(dst, src, scale)
}

// Max returns the maximum value of src, or -Inf for an empty slice. NaN makes the result unspecified.
func Max(src []float32) float32 {
	return vmaxF32Asm(src)
}

// Min returns the minimum value of src, or +Inf for an empty slice. NaN makes the result unspecified.
func Min(src []float32) float32 {
	return vminF32Asm(src)
}

// CAbs writes dst[i] = |src[i]| over min(len(src), len(dst)) elements.
func CAbs(dst []float32, src []complex64) {
	vabsC64Asm(dst, src)
}
