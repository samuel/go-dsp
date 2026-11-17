package dsp

import (
	"errors"
	"math"
)

// RootRaisedCosineFIR designs a root-raised-cosine pulse of numTaps taps at sps samples per
// symbol with the given rolloff (excess bandwidth, in [0, 1]), the matched filter of a
// root-raised-cosine transmitter. The taps are normalized to unit energy (their squares sum to
// 1), so the pulse through its own matched filter peaks at 1; NormalizeTaps sets a DC gain
// instead. The design is done in float64 and rounded to T once at the end.
//
// Tap k sits at t = (k - (numTaps-1)/2)/sps symbols, so an odd numTaps puts a tap on the
// pulse's center. Truncation to a span of a few symbols leaves some intersymbol interference;
// 8 to 16 symbols is typical.
func RootRaisedCosineFIR[T Float](numTaps int, sps, rolloff float64) ([]T, error) {
	if err := checkPulse(numTaps, sps, rolloff); err != nil {
		return nil, err
	}
	design := pulse(numTaps, sps, func(t float64) float64 { return rootRaisedCosine(t, rolloff) })
	var energy float64
	for _, v := range design {
		energy += v * v
	}
	return roundTaps[T](design, 1/math.Sqrt(energy)), nil
}

// RaisedCosineFIR designs a raised-cosine pulse of numTaps taps at sps samples per symbol with
// the given rolloff, in [0, 1]. It is normalized to a peak of 1 at t = 0, where it has a tap
// when numTaps is odd, and is zero at every other whole number of symbols from the center: the
// Nyquist pulse with no intersymbol interference. The design is done in float64 and rounded to
// T once at the end. Tap k sits at t = (k - (numTaps-1)/2)/sps symbols.
func RaisedCosineFIR[T Float](numTaps int, sps, rolloff float64) ([]T, error) {
	if err := checkPulse(numTaps, sps, rolloff); err != nil {
		return nil, err
	}
	design := pulse(numTaps, sps, func(t float64) float64 { return raisedCosine(t, rolloff) })
	return roundTaps[T](design, 1), nil
}

func checkPulse(numTaps int, sps, rolloff float64) error {
	switch {
	case numTaps < 1:
		return errors.New("dsp: pulse shaping filter needs at least one tap")
	case !(sps >= 1) || math.IsInf(sps, 1):
		return errors.New("dsp: pulse shaping filter needs at least 1 sample per symbol")
	case !(rolloff >= 0 && rolloff <= 1):
		return errors.New("dsp: pulse shaping rolloff must be in [0, 1]")
	}
	return nil
}

// pulse samples h at numTaps points spaced 1/sps symbols apart, centered on t = 0.
func pulse(numTaps int, sps float64, h func(t float64) float64) []float64 {
	center := float64(numTaps-1) / 2
	design := make([]float64, numTaps)
	for k := range design {
		design[k] = h((float64(k) - center) / sps)
	}
	return design
}

func roundTaps[T Float](design []float64, scale float64) []T {
	taps := make([]T, len(design))
	for k, v := range design {
		taps[k] = T(v * scale)
	}
	return taps
}

// raisedCosine is the raised-cosine pulse at t symbols, 1 at t = 0.
func raisedCosine(t, beta float64) float64 {
	x := 2 * beta * t
	if math.Abs(math.Abs(x)-1) < 1e-9 {
		// The limit at the zeros of the denominator, t = ±1/(2β).
		return math.Pi / 4 * sinc(1/(2*beta))
	}
	return sinc(t) * math.Cos(math.Pi*beta*t) / (1 - x*x)
}

// rootRaisedCosine is the root-raised-cosine pulse at t symbols, unnormalized.
func rootRaisedCosine(t, beta float64) float64 {
	switch {
	case t == 0:
		return 1 - beta + 4*beta/math.Pi
	case beta > 0 && math.Abs(math.Abs(4*beta*t)-1) < 1e-9:
		// The limit at the zeros of the denominator, t = ±1/(4β).
		a := math.Pi / (4 * beta)
		return beta / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(a) + (1-2/math.Pi)*math.Cos(a))
	}
	x := 4 * beta * t
	return (math.Sin(math.Pi*t*(1-beta)) + x*math.Cos(math.Pi*t*(1+beta))) / (math.Pi * t * (1 - x*x))
}
