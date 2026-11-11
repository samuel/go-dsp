package f32

// Scale calculates dst[i] = src[i] * scale.
//
//go:noescape
func Scale(dst, src []float32, scale float32)

// Max returns the maximum value of src, or -Inf for an empty slice. NaN makes the result unspecified.
//
//go:noescape
func Max(src []float32) float32

// Min returns the minimum value of src, or +Inf for an empty slice. NaN makes the result unspecified.
//
//go:noescape
func Min(src []float32) float32

// CAbs writes dst[i] = |src[i]| over min(len(src), len(dst)) elements.
//
//go:noescape
func CAbs(dst []float32, src []complex64)
