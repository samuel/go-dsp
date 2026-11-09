//go:build arm64 && !goexperiment.simd

package f32

// GOEXPERIMENT=simd is off by default, so without this file a plain build on
// arm64 falls to the scalar code in math32_scalar.go — 12x slower for Max and
// 4x for Scale at 16K floats on an M2 Pro. NEON is part of the arm64 baseline,
// so these go straight to the assembly in math32_nosimd_arm64.s. Tuned like
// math32_simd_arm64.go and within a few percent of it from 1K floats up.

// Scale calculates dst[i] = src[i] * scale.
func Scale(dst, src []float32, scale float32)

// Max returns the maximum value of src, or -Inf for an empty slice. NaN makes the result unspecified.
func Max(src []float32) float32

// Min returns the minimum value of src, or +Inf for an empty slice. NaN makes the result unspecified.
func Min(src []float32) float32

// CAbs writes dst[i] = |src[i]| over min(len(src), len(dst)) elements.
func CAbs(dst []float32, src []complex64)
