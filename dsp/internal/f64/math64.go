// Package f64 holds the float64 and complex128 kernels behind the generic
// vector functions in dsp.
//
// One implementation on every target where f32 has six files apiece: no
// assembly, no archsimd, no per-architecture file. Adding a vector rung here
// changes no caller and no name; that is the point of the split.
//
// Call these directly only to skip dsp's generic dispatch; see f32's doc.
package f64

import "math"

// The kernels.
//
// Tuned the way math32_scalar.go is, and for the same reasons.

// Scale calculates dst[i] = src[i] * scale. It processes
// min(len(src), len(dst)) elements, and the slices may be the same.
//
// A 16-element block with an 8-element step, faster than a plain loop by
// 22-57% at every length measured on an M2 Pro (32 is no better). The block
// size trade is cache-driven: against 8, 16 is 17-31% faster up to 8192 and
// 9-10% slower at 16K/64K, two slices of 8192 float64 being exactly L1 here.
// 16 wins over the wider range, and past L1 the loop is not where time goes.
// BenchmarkVScaleF64 keeps both variants so a different L1 can be rechecked.
func Scale(dst, src []float64, scale float64) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	for len(src) >= 16 {
		in := (*[16]float64)(src[0:16])
		out := (*[16]float64)(dst[0:16])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		out[8], out[9], out[10], out[11] = in[8]*scale, in[9]*scale, in[10]*scale, in[11]*scale
		out[12], out[13], out[14], out[15] = in[12]*scale, in[13]*scale, in[14]*scale, in[15]*scale
		src, dst = src[16:], dst[16:]
	}
	if len(src) >= 8 {
		in := (*[8]float64)(src[0:8])
		out := (*[8]float64)(dst[0:8])
		out[0], out[1], out[2], out[3] = in[0]*scale, in[1]*scale, in[2]*scale, in[3]*scale
		out[4], out[5], out[6], out[7] = in[4]*scale, in[5]*scale, in[6]*scale, in[7]*scale
		src, dst = src[8:], dst[8:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

// Max returns the largest value in src, or -Inf for an empty slice.
//
// Scalar; no vector rung yet. The comparison is a branch rather than the max
// builtin because the builtin must honor NaN and signed zero, costing a longer
// sequence; see math32_scalar.go.
func Max(src []float64) float64 {
	m := math.Inf(-1)
	for _, v := range src {
		if v > m {
			m = v
		}
	}
	return m
}

// Min is Max mirrored; see the note there.
func Min(src []float64) float64 {
	m := math.Inf(1)
	for _, v := range src {
		if v < m {
			m = v
		}
	}
	return m
}

// Sum returns the sum of src, or 0 for an empty slice. Eight accumulators
// folded pairwise; see f32.Sum for why the order is not part of the contract.
func Sum(src []float64) float64 {
	var a [8]float64
	for len(src) >= 8 {
		s := (*[8]float64)(src[0:8])
		a[0] += s[0]
		a[1] += s[1]
		a[2] += s[2]
		a[3] += s[3]
		a[4] += s[4]
		a[5] += s[5]
		a[6] += s[6]
		a[7] += s[7]
		src = src[8:]
	}
	for i, v := range src {
		a[i] += v
	}
	return ((a[0] + a[1]) + (a[2] + a[3])) + ((a[4] + a[5]) + (a[6] + a[7]))
}

// MaxIdx returns the largest value in src and its index, or (-Inf, -1) for an
// empty slice. On a tie the lowest index wins; see f32.MaxIdx.
func MaxIdx(src []float64) (float64, int) {
	if len(src) == 0 {
		return math.Inf(-1), -1
	}
	m, idx := src[0], 0
	for i, v := range src[1:] {
		if v > m {
			m, idx = v, i+1
		}
	}
	return m, idx
}

// CMagSq writes dst[i] = |src[i]|^2 over min(len(dst), len(src)) elements.
// See f32.CMagSq.
func CMagSq(dst []float64, src []complex128) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = float64(re*re) + float64(im*im)
	}
}

// CAbs writes dst[i] = |src[i]| over min(len(dst), len(src)) elements.
//
// The naive expression rather than math.Hypot, as f32.CAbs uses: Hypot has no
// vector form and would make the two widths disagree about what they compute.
// There is nothing wider than float64 to accumulate the squares in here, so a
// magnitude above about 1.3e154 overflows where Hypot would not.
func CAbs(dst []float64, src []complex128) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = math.Sqrt(re*re + im*im)
	}
}

// CMulReal writes dst[i] = complex(real(src[i])*mul[i], imag(src[i])*mul[i]),
// over min(len(dst), len(src), len(mul)) elements. See f32.CMulReal.
func CMulReal(dst, src []complex128, mul []float64) {
	n := min(len(mul), len(src), len(dst))
	for i, v := range src[:n] {
		w := mul[i]
		dst[i] = complex(real(v)*w, imag(v)*w)
	}
}
