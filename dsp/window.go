package dsp

import (
	"math"
	"slices"
)

// The cosine-sum windows here -- Hamming, Hann, Blackman, Nuttall,
// Blackman-Harris and flat-top -- are the symmetric form, evaluated over
// len(dst)-1 points, so w[0] == w[N-1]. The periodic (DFT-even) form, which
// drops the last point so the window tiles without a seam, is not what these
// return.
//
// The frequency-domain forms apply the same window to a few adjacent bins, the
// form NewSDFT expects for its window argument.
var (
	blackmanFreqCoeff = [5]float64{0.16 / 4, -1.0 / 4, (1 - 0.16) / 2, -1.0 / 4, 0.16 / 4}
	hammingFreqCoeff  = [3]float64{(0.53836 - 1) / 2, 0.53836, (0.53836 - 1) / 2}
	hannFreqCoeff     = [3]float64{-0.25, 0.5, -0.25}
)

// BlackmanFreqCoeff returns the frequency-domain Blackman window.
func BlackmanFreqCoeff() []float64 { return slices.Clone(blackmanFreqCoeff[:]) }

// HammingFreqCoeff returns the frequency-domain Hamming window.
func HammingFreqCoeff() []float64 { return slices.Clone(hammingFreqCoeff[:]) }

// HannFreqCoeff returns the frequency-domain Hann window.
func HannFreqCoeff() []float64 { return slices.Clone(hannFreqCoeff[:]) }

// WindowGain returns the sum and the sum of squares of a window's points: the
// coherent gain, what a bin-center sinusoid comes out of a transform scaled by,
// and the incoherent gain, what noise picks up. Their ratio gives the
// equivalent noise bandwidth in bins, sumSq*len(w)/sum^2.
func WindowGain[T Float](w []T) (sum, sumSq float64) {
	for _, v := range w {
		f := float64(v)
		sum += f
		sumSq += f * f
	}
	return sum, sumSq
}

// TriangleWindow fills dst with a triangular window of len(dst) points.
//
// This is MATLAB's triang, not Bartlett: the denominator is (N+1)/2, so the
// end points are non-zero. Bartlett, which divides by (N-1)/2, touches zero at
// both ends.
func TriangleWindow[T Float](dst []T) {
	for n := range dst {
		dst[n] = T(1 - math.Abs((float64(n)-float64(len(dst)-1)/2.0)/(float64(len(dst)+1)/2.0)))
	}
}

// HammingWindow fills dst with a Hamming window of len(dst) points.
//
// The first coefficient is 0.53836, which cancels the nearest sidelobe exactly,
// rather than the 0.54 the window is usually quoted with; TestWindowSidelobes
// measures this one at -43.2 dB.
func HammingWindow[T Float](dst []T) {
	window(dst, []float64{0.53836, 1 - 0.53836})
}

// HannWindow fills dst with a Hann (raised cosine) window of
// len(dst) points.
func HannWindow[T Float](dst []T) {
	window(dst, []float64{0.5, 0.5})
}

// BlackmanWindow fills dst with a Blackman window of len(dst) points,
// using the common a=0.16 approximation rather than the exact coefficients.
func BlackmanWindow[T Float](dst []T) {
	a := 0.16
	window(dst, []float64{(1.0 - a) / 2.0, 1.0 / 2.0, a / 2.0})
}

// NuttallWindow fills dst with a Nuttall window of len(dst) points.
func NuttallWindow[T Float](dst []T) {
	window(dst, []float64{0.355768, 0.487396, 0.144232, 0.012604})
}

// window fills dst with the cosine-sum window whose coefficients are a: the
// terms alternate in sign, starting positive, so a of {a0, a1, a2, ...} gives
// a0 - a1*cos(2*pi*n/N) + a2*cos(4*pi*n/N) - ... The terms are summed in
// float64 whatever the dst width is, since the coefficients are constants and
// the cosines cost far more than the rounding.
//
// A window of one point is unity: the general form divides by len(dst)-1,
// which is zero there, so it would give NaN.
func window[T Float](dst []T, a []float64) {
	if len(a) < 1 {
		panic("dsp: window needs at least one coefficient")
	}
	if len(dst) == 0 {
		return
	}
	if len(dst) == 1 {
		dst[0] = 1
		return
	}
	nn := float64(len(dst) - 1)
	for n := range dst {
		fn := float64(n)
		var v float64
		for m, c := range a {
			t := c * math.Cos(2*math.Pi*float64(m)*fn/nn)
			if m&1 == 0 {
				v += t
			} else {
				v -= t
			}
		}
		dst[n] = T(v)
	}
}

// BlackmanHarrisWindow fills dst with the four term minimum Blackman-Harris
// window, whose sidelobes fall about 92 dB below the main lobe. That is far
// enough down for most spectrum work and considerably cheaper than a Kaiser,
// which is the only window here that goes deeper.
func BlackmanHarrisWindow[T Float](dst []T) {
	window(dst, []float64{0.35875, 0.48829, 0.14128, 0.01168})
}

// FlatTopWindow fills dst with a five term flat-top window. Its main lobe is
// flat to within about a thousandth of a decibel, so a tone that does not sit
// on a bin center still reads its true amplitude. That costs a main lobe
// several bins wide.
func FlatTopWindow[T Float](dst []T) {
	window(dst, []float64{0.21557895, 0.41663158, 0.277263158, 0.083578947, 0.006947368})
}

// KaiserWindow fills dst with a Kaiser window of len(dst) points and
// shape parameter beta, which must not be negative.
//
// beta trades main lobe width against sidelobe level, and it is the only window
// here whose sidelobes can be put wherever they are needed. The peak sidelobe
// runs about -13 dB at beta 0, which is the rectangular window, -46 at 6.3,
// -69 at 9.4, -95 at 12.6, -119 at 15.6, -151 at 19.4 and -201 at 25 -- very
// nearly 8 dB per unit of beta once beta is above about 5. The main lobe widens
// in step, to roughly 2*(beta/pi + 1) bins.
//
// A window whose sidelobes are not below the feature being looked for measures
// the window rather than the signal, so analyzing a stopband deeper than about
// 100 dB needs this one. Note that the levels above are the window's own, which
// is not what KaiserBeta computes -- see the warning there.
func KaiserWindow[T Float](dst []T, beta float64) {
	if beta < 0 {
		panic("dsp: Kaiser window beta must not be negative")
	}
	if len(dst) == 0 {
		return
	}
	if len(dst) == 1 {
		dst[0] = 1
		return
	}
	nn := float64(len(dst) - 1)
	norm := 1 / besselI0(beta)
	for n := range dst {
		r := 2*float64(n)/nn - 1
		dst[n] = T(besselI0(beta*math.Sqrt(1-r*r)) * norm)
	}
}

// KaiserBeta returns the shape parameter of the Kaiser window that designs an
// FIR filter whose stopband attenuation is at least attenuation dB.
//
// This is the filter design formula, not a window sidelobe figure: the window
// it selects has sidelobes well above the attenuation asked for, because a
// windowed sinc buys the rest of the way down from the sum over the sidelobes
// rather than from any one of them. KaiserBeta(150) returns about 15.6, and a
// Kaiser window at that beta has sidelobes near -119 dB -- 31 dB short of the
// argument. When the window itself has to be quiet, pick beta from the levels
// quoted on KaiserWindow instead: -150 dB needs about 19.4.
func KaiserBeta(attenuation float64) float64 {
	switch {
	case attenuation > 50:
		return 0.1102 * (attenuation - 8.7)
	case attenuation >= 21:
		a := attenuation - 21
		return 0.5842*math.Pow(a, 0.4) + 0.07886*a
	}
	return 0
}

// besselI0 returns the modified Bessel function of the first kind of order zero,
// by its power series sum((x/2)^k / k!)^2. The terms fall superlinearly once k
// passes x/2, so it converges in about fifty terms at the largest useful beta.
func besselI0(x float64) float64 {
	half := x / 2
	sum := 1.0
	term := 1.0
	for k := 1; k < 1000; k++ {
		term *= half / float64(k)
		t := term * term
		sum += t
		if t <= sum*1e-17 {
			break
		}
	}
	return sum
}
