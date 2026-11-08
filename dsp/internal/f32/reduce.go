package f32

import "math"

// Sum returns the sum of src, or 0 for an empty slice.
//
// Eight independent accumulators folded pairwise on the way out — the form a
// vector kernel takes, and more accurate than a serial loop: the error of a
// naive sum grows with the dependent chain, and eight chains are each an
// eighth as long. Measured at 7.5x less error than a serial float32 sum over
// 64K values near 1.0, averaged over inputs, since rounding errors are a
// random walk and any single input can go either way.
//
// The order is therefore not left to right, and not part of the contract: a
// sum that must be bit-for-bit reproducible cannot be vectorized at all, and a
// different vector width folds differently. Inputs whose partial sums are all
// exact — small integers, say — come out exact whatever the order, which is
// what the tests pin.
func Sum(src []float32) float32 {
	var a [8]float32
	for len(src) >= 8 {
		s := (*[8]float32)(src[0:8])
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
// empty slice. On a tie the lowest index wins. NaN makes the result
// unspecified, as for Max.
//
// Serial, unlike Max: tracking the index needs a second vector of lane numbers
// and a blend on every step, so it is its own kernel rather than Max plus a
// register.
func MaxIdx(src []float32) (float32, int) {
	if len(src) == 0 {
		return float32(math.Inf(-1)), -1
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
//
// The explicit roundings match CAbs, of which this is the part before the
// square root, so the two agree on every input. Prefer it whenever magnitudes
// are only compared or summed, since the square root is monotonic.
func CMagSq(dst []float32, src []complex64) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		re, im := real(v), imag(v)
		dst[i] = float32(re*re) + float32(im*im)
	}
}
