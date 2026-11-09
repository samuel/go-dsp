//go:build !arm && !arm64

package f32

import "math"

// Scalar implementations of the vectorized float32 helpers, the fallback
// wherever neither archsimd nor assembly is available (386, future ports), so
// written for speed rather than brevity. Neither arm builds this file; amd64
// does, but reaches it only through tests.
//
// Tuned as portable Go: written so the compiler gets the clearest intent, on
// the assumption a backend able to do better will. Measurements below are from
// an M2 Pro. vmaxF32Scalar is where that assumption fails.

// vscaleF32Scalar calculates dst[i] = src[i] * scale.
//
// A 32-element block with 16- and 8-element steps. Unrolling hoists the bounds
// check and keeps scale in a register, and the block must be wide enough to
// amortize the two guarded slice advances: 32 beats 8 by 32-49% and 16 by ~5%.
// The steps are worth 20-23% when the length is not a multiple of the block.
//
// Write the block out longhand: a nested `for j := range` is not unrolled and
// gives back the entire win.
func vscaleF32Scalar(dst, src []float32, scale float32) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for len(src) >= 32 {
		in := (*[32]float32)(src[0:32])
		out := (*[32]float32)(dst[0:32])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		out[8], out[9], out[10], out[11] = in[8]*scale, in[9]*scale, in[10]*scale, in[11]*scale
		out[12], out[13], out[14], out[15] = in[12]*scale, in[13]*scale, in[14]*scale, in[15]*scale
		out[16], out[17], out[18], out[19] = in[16]*scale, in[17]*scale, in[18]*scale, in[19]*scale
		out[20], out[21], out[22], out[23] = in[20]*scale, in[21]*scale, in[22]*scale, in[23]*scale
		out[24], out[25], out[26], out[27] = in[24]*scale, in[25]*scale, in[26]*scale, in[27]*scale
		out[28], out[29], out[30], out[31] = in[28]*scale, in[29]*scale, in[30]*scale, in[31]*scale
		src, dst = src[32:], dst[32:]
	}
	if len(src) >= 16 {
		in := (*[16]float32)(src[0:16])
		out := (*[16]float32)(dst[0:16])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		out[8], out[9], out[10], out[11] = in[8]*scale, in[9]*scale, in[10]*scale, in[11]*scale
		out[12], out[13], out[14], out[15] = in[12]*scale, in[13]*scale, in[14]*scale, in[15]*scale
		src, dst = src[16:], dst[16:]
	}
	if len(src) >= 8 {
		in := (*[8]float32)(src[0:8])
		out := (*[8]float32)(dst[0:8])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		src, dst = src[8:], dst[8:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

// vmaxF32Scalar returns the maximum value of src, or -Inf for an empty slice.
//
// The max builtin instead of an `if v > m` compare, which is a data-dependent
// branch that mispredicts on real data. Its cost varies because it must honor
// NaN and signed zero: one instruction on arm64 (2x the compare, 3.4x with the
// accumulators below), nine instructions inline on amd64, and a CALL to
// runtime.fmax32 per element on 386 — a real regression, and 386 is the one
// architecture that runs this in production. Accepted rather than write the
// portable fallback backwards for a legacy target.
//
// Eight accumulators over sixteen elements per iteration so the maxes do not
// form one dependent chain: two are 2.4x slower at 1K floats, four 1.6x,
// sixteen spill. The 8- and 4-element steps keep a non-multiple length off that
// chain, worth 25-37% between 15 and 63.
func vmaxF32Scalar(src []float32) float32 {
	n := float32(math.Inf(-1))
	m0, m1, m2, m3, m4, m5, m6, m7 := n, n, n, n, n, n, n, n
	for len(src) >= 16 {
		in := (*[16]float32)(src[0:16])
		m0, m1, m2, m3 = max(m0, in[0]), max(m1, in[1]), max(m2, in[2]), max(m3, in[3])
		m4, m5, m6, m7 = max(m4, in[4]), max(m5, in[5]), max(m6, in[6]), max(m7, in[7])
		m0, m1, m2, m3 = max(m0, in[8]), max(m1, in[9]), max(m2, in[10]), max(m3, in[11])
		m4, m5, m6, m7 = max(m4, in[12]), max(m5, in[13]), max(m6, in[14]), max(m7, in[15])
		src = src[16:]
	}
	if len(src) >= 8 {
		in := (*[8]float32)(src[0:8])
		m0, m1, m2, m3 = max(m0, in[0]), max(m1, in[1]), max(m2, in[2]), max(m3, in[3])
		m4, m5, m6, m7 = max(m4, in[4]), max(m5, in[5]), max(m6, in[6]), max(m7, in[7])
		src = src[8:]
	}
	if len(src) >= 4 {
		in := (*[4]float32)(src[0:4])
		m0, m1, m2, m3 = max(m0, in[0]), max(m1, in[1]), max(m2, in[2]), max(m3, in[3])
		src = src[4:]
	}
	m0, m1, m2, m3 = max(m0, m4), max(m1, m5), max(m2, m6), max(m3, m7)
	m0, m2 = max(m0, m1), max(m2, m3)
	m0 = max(m0, m2)
	for _, v := range src {
		m0 = max(m0, v)
	}
	return m0
}

// vminF32Scalar returns the minimum value of src, or +Inf for an empty slice.
//
// The same loop structure as vmaxF32Scalar above, and for the same reasons — including
// what the builtin costs on 386.
func vminF32Scalar(src []float32) float32 {
	n := float32(math.Inf(1))
	m0, m1, m2, m3, m4, m5, m6, m7 := n, n, n, n, n, n, n, n
	for len(src) >= 16 {
		in := (*[16]float32)(src[0:16])
		m0, m1, m2, m3 = min(m0, in[0]), min(m1, in[1]), min(m2, in[2]), min(m3, in[3])
		m4, m5, m6, m7 = min(m4, in[4]), min(m5, in[5]), min(m6, in[6]), min(m7, in[7])
		m0, m1, m2, m3 = min(m0, in[8]), min(m1, in[9]), min(m2, in[10]), min(m3, in[11])
		m4, m5, m6, m7 = min(m4, in[12]), min(m5, in[13]), min(m6, in[14]), min(m7, in[15])
		src = src[16:]
	}
	if len(src) >= 8 {
		in := (*[8]float32)(src[0:8])
		m0, m1, m2, m3 = min(m0, in[0]), min(m1, in[1]), min(m2, in[2]), min(m3, in[3])
		m4, m5, m6, m7 = min(m4, in[4]), min(m5, in[5]), min(m6, in[6]), min(m7, in[7])
		src = src[8:]
	}
	if len(src) >= 4 {
		in := (*[4]float32)(src[0:4])
		m0, m1, m2, m3 = min(m0, in[0]), min(m1, in[1]), min(m2, in[2]), min(m3, in[3])
		src = src[4:]
	}
	m0, m1, m2, m3 = min(m0, m4), min(m1, m5), min(m2, m6), min(m3, m7)
	m0, m2 = min(m0, m1), min(m2, m3)
	m0 = min(m0, m2)
	for _, v := range src {
		m0 = min(m0, v)
	}
	return m0
}

// vabsC64Scalar calculates dst[i] = |src[i]|.
//
// Written as re*re + im*im, Go may contract the expression into one FMA (arm64
// does), while the vector units round the two squares separately. The explicit
// float32 conversions pin those roundings so all four architectures agree.
//
// Widening to float64 for the square root costs no accuracy — float64 carries
// more than the 2*24+2 bits needed, so double rounding is innocuous for sqrt —
// and arm64 folds the idiom into one FSQRTS.
//
// No unrolled block: this loop is square-root bound on every target.
func vabsC64Scalar(dst []float32, src []complex64) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for i, v := range src {
		re, im := real(v), imag(v)
		dst[i] = float32(math.Sqrt(float64(float32(re*re) + float32(im*im))))
	}
}
