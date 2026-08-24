//go:build !arm && !amd64 && !arm64

package f32

// Scale calculates dst[i] = src[i] * scale.
func Scale(dst, src []float32, scale float32) {
	vscaleF32Scalar(dst, src, scale)
}

// Max returns the maximum value of src, or -Inf for an empty slice. NaN makes the result unspecified.
func Max(src []float32) float32 {
	return vmaxF32Scalar(src)
}

// Min returns the minimum value of src, or +Inf for an empty slice. NaN makes the result unspecified.
func Min(src []float32) float32 {
	return vminF32Scalar(src)
}

// CAbs writes dst[i] = |src[i]| over min(len(src), len(dst)) elements.
func CAbs(dst []float32, src []complex64) {
	vabsC64Scalar(dst, src)
}
