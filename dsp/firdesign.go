package dsp

import (
	"errors"
	"math"
)

// NormalizeTaps scales taps in place so they sum to gain, which is the
// filter's response at DC. The sum and the scaling are done in float64. It
// returns an error, leaving taps unchanged, when gain is not finite or the
// taps sum to zero or to a non-finite value, since such a filter has no DC
// gain to scale.
func NormalizeTaps[T Float](taps []T, gain float64) error {
	if math.IsInf(gain, 0) || math.IsNaN(gain) {
		return errors.New("dsp: normalized tap gain must be finite")
	}
	var sum float64
	for _, c := range taps {
		sum += float64(c)
	}
	if sum == 0 || math.IsInf(sum, 0) || math.IsNaN(sum) {
		return errors.New("dsp: taps have no finite, non-zero DC gain to normalize")
	}
	for i, c := range taps {
		taps[i] = T(float64(c) * gain / sum)
	}
	return nil
}

// LowPassFIR designs a linear-phase low-pass FIR filter of numTaps taps by the
// window method: a sinc with its cutoff at cutoff cycles per sample (0.5 is
// the Nyquist frequency), multiplied by KaiserWindow of numTaps points and
// parameter beta, and normalized to a DC gain of 1. The design is done in
// float64 and rounded to T once at the end. KaiserBeta gives beta for a
// stopband attenuation.
//
// Tap k sits at t = k - (numTaps-1)/2, so the window's edges fall on the end
// taps.
func LowPassFIR[T Float](numTaps int, cutoff, beta float64) ([]T, error) {
	switch {
	case numTaps < 1:
		return nil, errors.New("dsp: low-pass FIR needs at least one tap")
	case !(cutoff > 0 && cutoff <= 0.5):
		return nil, errors.New("dsp: low-pass FIR cutoff must be in (0, 0.5] cycles per sample")
	case !(beta >= 0) || math.IsInf(beta, 1):
		return nil, errors.New("dsp: low-pass FIR Kaiser beta must be finite and not negative")
	}
	center := float64(numTaps-1) / 2
	design := make([]float64, numTaps)
	KaiserWindow(design, beta)
	for k := range design {
		design[k] *= sinc(2 * cutoff * (float64(k) - center))
	}
	// A beta large enough to overflow besselI0 makes the window NaN, which
	// NormalizeTaps rejects.
	if err := NormalizeTaps(design, 1); err != nil {
		return nil, err
	}
	taps := make([]T, numTaps)
	for k, v := range design {
		taps[k] = T(v)
	}
	return taps, nil
}

// sinc returns the normalized sinc function sin(pi*x)/(pi*x), which is 1 at 0.
func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}
