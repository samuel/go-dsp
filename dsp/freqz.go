package dsp

import "math"

// Freqz evaluates the frequency response of the filter with feedforward
// coefficients b and feedback coefficients a at each normalized angular
// frequency in w, in radians per sample, so pi is the Nyquist frequency:
//
//	H(w) = sum(b[n] * exp(-j*w*n)) / sum(a[n] * exp(-j*w*n))
//
// An empty or nil a means an FIR filter, the same as a of {1}. The result is a
// fresh slice the same length as w.
//
// Unlike a transform of the impulse response this is exact: there is no
// truncation, no transient, no window and no grid of the transform's choosing,
// so the frequencies can be spaced logarithmically, or packed into a passband
// finely enough to resolve a hundredth of a decibel. The one place it gives
// way is a zero on the unit circle, where the numerator is computed by
// cancellation and bottoms out around -300 dB rather than reaching the true
// null.
func Freqz(b, a []float64, w []float64) []complex128 {
	h := make([]complex128, len(w))
	for i, wi := range w {
		num := polyAtExp(b, wi)
		if len(a) == 0 {
			h[i] = num
			continue
		}
		h[i] = num / polyAtExp(a, wi)
	}
	return h
}

// GroupDelay returns the group delay of the filter, -d(phase)/d(w), in samples,
// at each normalized angular frequency in w. a is optional in the same way as
// it is for Freqz.
//
// It is evaluated in closed form rather than by differencing a phase curve, so
// it does not depend on how finely w is spaced and needs no unwrapping. A
// linear-phase FIR filter of n taps therefore returns exactly (n-1)/2 at every
// frequency. The delay is genuinely undefined at a zero on the unit circle, and
// there the result is whatever the cancellation leaves.
func GroupDelay(b, a []float64, w []float64) []float64 {
	d := make([]float64, len(w))
	for i, wi := range w {
		// d(arg P)/dw = Im(P'/P) and P'(w) = -j*sum(n*p[n]*exp(-j*w*n)), so
		// -d(arg P)/dw is Re(sum(n*p[n]*exp(-j*w*n)) / P). The delay of B/A is
		// that quantity for B less the same for A.
		t := real(rampAtExp(b, wi) / polyAtExp(b, wi))
		if len(a) != 0 {
			t -= real(rampAtExp(a, wi) / polyAtExp(a, wi))
		}
		d[i] = t
	}
	return d
}

// Unwrap removes the 2*pi jumps from a phase curve in place, so a curve that
// falls steadily reads as a straight line rather than as a sawtooth.
//
// It assumes the true phase changes by less than pi between neighbors, which is
// a statement about how finely the curve is sampled rather than about the
// filter: a filter of group delay d samples needs a step smaller than pi/d
// radians per sample. A coarser grid unwraps to a smooth curve that is quietly
// wrong, which is why GroupDelay does not go through here.
func Unwrap[T Float](dst []T) {
	if len(dst) < 2 {
		return
	}
	const twoPi = 2 * math.Pi
	var offset float64
	prev := float64(dst[0])
	for i := 1; i < len(dst); i++ {
		cur := float64(dst[i])
		d := cur - prev
		prev = cur
		offset -= twoPi * math.Round(d/twoPi)
		dst[i] = T(cur + offset)
	}
}

// polyAtExp returns sum(p[n] * exp(-j*w*n)).
func polyAtExp(p []float64, w float64) complex128 {
	var re, im float64
	for n, c := range p {
		s, cs := math.Sincos(w * float64(n))
		re += c * cs
		im -= c * s
	}
	return complex(re, im)
}

// rampAtExp returns sum(n * p[n] * exp(-j*w*n)), the numerator of the group
// delay of p.
func rampAtExp(p []float64, w float64) complex128 {
	var re, im float64
	for n, c := range p {
		s, cs := math.Sincos(w * float64(n))
		c *= float64(n)
		re += c * cs
		im -= c * s
	}
	return complex(re, im)
}
