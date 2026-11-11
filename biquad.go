package dsp

import (
	"errors"
	"math"
)

// http://www.musicdsp.org/files/Audio-EQ-Cookbook.txt

// Biquad is a biquad: a two-pole, two-zero IIR filter in direct form 1. The
// constructors below fill in the coefficients for the usual responses, and
// NewBiquad takes them directly. The filter keeps its delay lines between
// calls, so a stream can be filtered in blocks. The zero value outputs silence.
//
// The coefficients and the delay lines are float64 whatever the sample type is,
// since a biquad's recursion is where a narrower state shows up as drift. The
// sample type therefore cannot be inferred from a constructor's arguments and
// has to be given, as in NewBiquadLowPass[float32](48000, 1000, 0.7071).
type Biquad[T Float] struct {
	// The coefficients divided by a0, which leaves a0 as exactly 1.
	b0, b1, b2      float64
	a1, a2          float64
	prevIn, prevOut [2]float64
}

// NewBiquad returns a biquad with the given coefficients, for a response none
// of the constructors below covers. It divides them by a0 and returns an error
// unless a0 is non-zero and every result is finite: a non-finite coefficient
// turns the stream into NaN from the first sample, since the delay lines feed
// back.
func NewBiquad[T Float](b0, b1, b2, a0, a1, a2 float64) (*Biquad[T], error) {
	if a0 == 0 {
		return nil, errors.New("dsp: biquad coefficients are unusable: a0 is zero")
	}
	f := &Biquad[T]{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: a1 / a0, a2: a2 / a0}
	// A sum is finite exactly when every term is: a NaN propagates, and two
	// infinities of opposite sign give a NaN rather than canceling.
	if s := f.b0 + f.b1 + f.b2 + f.a1 + f.a2; math.IsNaN(s) || math.IsInf(s, 0) {
		return nil, errors.New("dsp: biquad coefficients are unusable: a coefficient is not finite once divided by a0")
	}
	return f, nil
}

// step advances the recursion by one sample and returns the output. Process and
// ProcessOne share it, so the two produce bit-identical results for the same
// stream.
func (f *Biquad[T]) step(x float64) float64 {
	y := f.b0*x + f.b1*f.prevIn[0] + f.b2*f.prevIn[1] - f.a1*f.prevOut[0] - f.a2*f.prevOut[1]
	f.prevOut[1] = f.prevOut[0]
	f.prevOut[0] = y
	f.prevIn[1] = f.prevIn[0]
	f.prevIn[0] = x
	return y
}

// OutputLen returns inputs: the filter writes one sample per input.
func (f *Biquad[T]) OutputLen(inputs int) int { return inputs }

// Process writes one output sample per input sample and returns len(src). It
// panics if dst is shorter than src. The slices may be the same.
func (f *Biquad[T]) Process(dst, src []T) int {
	checkDst("Biquad", len(dst), len(src))
	for i, s := range src {
		dst[i] = T(f.step(float64(s)))
	}
	return len(src)
}

// ProcessOne filters a single sample.
func (f *Biquad[T]) ProcessOne(s T) T {
	return T(f.step(float64(s)))
}

// RealCoefficients returns the numerator and denominator as the b and a slices
// a transfer function is written in, so a biquad can be handed to Freqz like
// any other filter. They are copies, normalized as the constructor left them,
// so a[0] is exactly 1. ok is always true.
func (f *Biquad[T]) RealCoefficients() (b, a []float64, ok bool) {
	return []float64{f.b0, f.b1, f.b2}, []float64{1, f.a1, f.a2}, true
}

// Reset clears the delay lines, so the filter starts a new stream from silence.
func (f *Biquad[T]) Reset() {
	f.prevIn = [2]float64{}
	f.prevOut = [2]float64{}
}

// checkBiquadArgs validates the arguments every response constructor shares and
// returns w0, the cutoff or center as an angle per sample. shape is Q for the
// resonant responses and shelfSlope for the two shelves; shapeName names it in
// the error, since a shelf reporting a bad "Q" would send a caller looking at
// the wrong argument.
//
// The comparisons are written so that a NaN fails them: !(x > 0) rather than
// x <= 0. A frequency at or above Nyquist has nothing to sample, and one at
// zero makes sinW0 zero and alpha with it, which leaves a0 == 1 and a filter
// that is not the response asked for.
func checkBiquadArgs(sampleRate, freq, shape float64, shapeName string) (w0 float64, err error) {
	if !(sampleRate > 0) {
		return 0, errors.New("dsp: biquad needs a positive sample rate")
	}
	if !(freq > 0 && freq < sampleRate/2) {
		return 0, errors.New("dsp: biquad frequency must be above 0 and below the Nyquist rate")
	}
	if !(shape > 0) {
		return 0, errors.New("dsp: biquad needs a positive " + shapeName)
	}
	return 2 * math.Pi * freq / sampleRate, nil
}

// NewBiquadLowPass returns a low-pass biquad with the given cutoff frequency
// and Q, both in the same units as sampleRate.
//
//	H(s) = 1 / (s^2 + s/Q + 1)
func NewBiquadLowPass[T Float](sampleRate, cutoffFreq, q float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, cutoffFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiquad[T](
		(1-cosW0)/2, 1-cosW0, (1-cosW0)/2,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBiquadHighPass returns a high-pass biquad with the given cutoff frequency
// and Q.
//
//	H(s) = s^2 / (s^2 + s/Q + 1)
func NewBiquadHighPass[T Float](sampleRate, cutoffFreq, q float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, cutoffFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiquad[T](
		(1+cosW0)/2, -(1 + cosW0), (1+cosW0)/2,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBiquadBandPassConstantSkirtGain returns a band-pass biquad whose skirt
// gain is constant, which makes the peak gain equal to q.
//
//	H(s) = s / (s^2 + s/Q + 1)
func NewBiquadBandPassConstantSkirtGain[T Float](sampleRate, centerFreq, q float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiquad[T](
		sinW0/2, 0, -sinW0/2, // +/- Q*alpha
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBiquadBandPassConstantPeakGain returns a band-pass biquad whose peak gain
// is 0 dB whatever q is.
//
//	H(s) = (s/Q) / (s^2 + s/Q + 1)
func NewBiquadBandPassConstantPeakGain[T Float](sampleRate, centerFreq, q float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiquad[T](
		alpha, 0, -alpha,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBiquadNotch returns a notch biquad, which rejects a band around
// centerFreq and passes everything else. q sets how narrow the notch is.
//
//	H(s) = (s^2 + 1) / (s^2 + s/Q + 1)
func NewBiquadNotch[T Float](sampleRate, centerFreq, q float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiquad[T](
		1, -2*cosW0, 1,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBiquadAllPass returns an all-pass biquad, which leaves the magnitude
// response flat and only shifts phase around centerFreq.
//
//	H(s) = (s^2 - s/Q + 1) / (s^2 + s/Q + 1)
func NewBiquadAllPass[T Float](sampleRate, centerFreq, q float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiquad[T](
		1-alpha, -2*cosW0, 1+alpha,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBiquadPeakingEQ returns a peaking EQ biquad, which boosts or cuts a band
// around centerFreq by dbGain decibels and leaves the rest alone.
//
//	H(s) = (s^2 + s*(A/Q) + 1) / (s^2 + s/(A*Q) + 1)
func NewBiquadPeakingEQ[T Float](sampleRate, centerFreq, q, dbGain float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	a := math.Pow(10, dbGain/40)
	return NewBiquad[T](
		1+alpha*a, -2*cosW0, 1-alpha*a,
		1+alpha/a, -2*cosW0, 1-alpha/a,
	)
}

// NewBiquadLowShelf returns a low shelf biquad, which applies dbGain decibels
// of gain below cutoffFreq and unity gain above it.
//
//	H(s) = A * (s^2 + (sqrt(A)/Q)*s + A)/(A*s^2 + (sqrt(A)/Q)*s + 1)
//
// shelfSlope controls how abrupt the transition is. At 1 the shelf is as steep
// as it can be while the gain stays monotonic in frequency; for other values
// the slope in dB/octave is proportional to it, for a fixed cutoffFreq /
// sampleRate and dbGain. Past 1 the square root below can go negative, which
// NewBiquad rejects rather than returning a filter of NaNs.
func NewBiquadLowShelf[T Float](sampleRate, cutoffFreq, shelfSlope, dbGain float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, cutoffFreq, shelfSlope, "shelf slope")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	a := math.Pow(10, dbGain/40.0)
	alpha := sinW0 / 2 * math.Sqrt((a+1/a)*(1/shelfSlope-1)+2)
	temp := 2 * math.Sqrt(a) * alpha
	return NewBiquad[T](
		a*((a+1)-(a-1)*cosW0+temp),
		2*a*((a-1)-(a+1)*cosW0),
		a*((a+1)-(a-1)*cosW0-temp),
		(a+1)+(a-1)*cosW0+temp,
		-2*((a-1)+(a+1)*cosW0),
		(a+1)+(a-1)*cosW0-temp,
	)
}

// NewBiquadHighShelf returns a high shelf biquad, which applies dbGain decibels
// of gain above cutoffFreq and unity gain below it. shelfSlope is as described
// for NewBiquadLowShelf.
//
//	H(s) = A * (A*s^2 + (sqrt(A)/Q)*s + 1)/(s^2 + (sqrt(A)/Q)*s + A)
func NewBiquadHighShelf[T Float](sampleRate, cutoffFreq, shelfSlope, dbGain float64) (*Biquad[T], error) {
	w0, err := checkBiquadArgs(sampleRate, cutoffFreq, shelfSlope, "shelf slope")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	a := math.Pow(10, dbGain/40)
	alpha := sinW0 / 2 * math.Sqrt((a+1/a)*(1/shelfSlope-1)+2)
	temp := 2 * math.Sqrt(a) * alpha
	return NewBiquad[T](
		a*((a+1)+(a-1)*cosW0+temp),
		-2*a*((a-1)+(a+1)*cosW0),
		a*((a+1)+(a-1)*cosW0-temp),
		(a+1)-(a-1)*cosW0+temp,
		2*((a-1)-(a+1)*cosW0),
		(a+1)-(a-1)*cosW0-temp,
	)
}
