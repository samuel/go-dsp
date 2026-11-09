package dsp

import (
	"errors"
	"math"
)

// http://www.musicdsp.org/files/Audio-EQ-Cookbook.txt

// BiQuadFilter is a biquad: a two-pole, two-zero IIR filter in direct form 1.
// The coefficients are not normalized, so A0 is divided out on every sample;
// the constructors below fill them in for the usual responses. The filter
// keeps its delay lines between calls, so a stream can be filtered in blocks.
//
// The coefficients and the delay lines are float64 whatever the sample type is,
// since a biquad's recursion is where a narrower state shows up as drift. The
// sample type therefore cannot be inferred from a constructor's arguments and
// has to be given, as in NewLowPassBiQuad[float32](48000, 1000, 0.7071).
type BiQuadFilter[T Float] struct {
	B0, B1, B2      float64
	A0, A1, A2      float64
	prevIn, prevOut [2]float64
}

// NewBiQuad returns a biquad with the given coefficients, for a response none
// of the constructors below covers. It returns an error unless a0 is non-zero
// and every coefficient divided by it is finite, which is what the recursion
// needs; see norm.
func NewBiQuad[T Float](b0, b1, b2, a0, a1, a2 float64) (*BiQuadFilter[T], error) {
	f := &BiQuadFilter[T]{B0: b0, B1: b1, B2: b2, A0: a0, A1: a1, A2: a2}
	if err := f.check(); err != nil {
		return nil, err
	}
	return f, nil
}

// biquadCoef is the five coefficients the recursion uses, each already divided
// by A0. a0 is not among them: it is 1 by construction.
type biquadCoef struct {
	b0, b1, b2 float64
	a1, a2     float64
}

// norm divides the coefficients by A0 and reports why the recursion cannot use
// the result, or "". The fields are exported, so a caller can reach both
// failures without going through a constructor: A0 == 0 makes every coefficient
// 0/0, and a non-finite one turns the stream into NaN from the first sample,
// since the delay lines feed back.
//
// A single sum is checked because a sum is finite exactly when every term is: a
// NaN propagates, and two infinities of opposite sign give a NaN rather than
// canceling.
func (f *BiQuadFilter[T]) norm() (biquadCoef, string) {
	if f.A0 == 0 {
		return biquadCoef{}, "A0 is zero"
	}
	c := biquadCoef{
		b0: f.B0 / f.A0,
		b1: f.B1 / f.A0,
		b2: f.B2 / f.A0,
		a1: f.A1 / f.A0,
		a2: f.A2 / f.A0,
	}
	if s := c.b0 + c.b1 + c.b2 + c.a1 + c.a2; math.IsNaN(s) || math.IsInf(s, 0) {
		return biquadCoef{}, "a coefficient is not finite once divided by A0"
	}
	return c, ""
}

// mustNorm is norm for the filtering path, where the coefficients are already
// in the struct and there is no error to return. Reaching the panic means the
// filter was assembled by hand, since every constructor rejects both cases.
func (f *BiQuadFilter[T]) mustNorm() biquadCoef {
	c, bad := f.norm()
	if bad != "" {
		panic("dsp: BiQuadFilter cannot filter: " + bad +
			"; build it with NewBiQuad or one of the New*BiQuad constructors")
	}
	return c
}

// check is norm's verdict as an error, for the constructors.
func (f *BiQuadFilter[T]) check() error {
	if _, bad := f.norm(); bad != "" {
		return errors.New("dsp: biquad coefficients are unusable: " + bad)
	}
	return nil
}

// step advances the recursion by one sample and returns the output. Filter and
// FilterOne share it, and share norm, so the two produce bit-identical results
// for the same stream -- FilterOne pays the five divisions per sample, which is
// the price of that rather than an oversight.
func (f *BiQuadFilter[T]) step(c biquadCoef, x float64) float64 {
	y := c.b0*x + c.b1*f.prevIn[0] + c.b2*f.prevIn[1] - c.a1*f.prevOut[0] - c.a2*f.prevOut[1]
	f.prevOut[1] = f.prevOut[0]
	f.prevOut[0] = y
	f.prevIn[1] = f.prevIn[0]
	f.prevIn[0] = x
	return y
}

// Filter writes one output sample per input sample, over
// min(len(dst), len(src)) of them. The slices may be the same.
func (f *BiQuadFilter[T]) Filter(dst, src []T) {
	c := f.mustNorm()
	for i, s := range src[:min(len(dst), len(src))] {
		dst[i] = T(f.step(c, float64(s)))
	}
}

// FilterOne filters a single sample. Filter hoists the division by A0 out of
// its loop, so prefer it when there is a block to filter; the two agree bit for
// bit either way.
func (f *BiQuadFilter[T]) FilterOne(s T) T {
	return T(f.step(f.mustNorm(), float64(s)))
}

// Coefficients returns the numerator and denominator as the b and a slices a
// transfer function is written in, so a biquad can be handed to Freqz like any
// other filter. They are copies, and they are not normalized: A0 is returned as
// it stands, since Freqz divides one polynomial by the other anyway. The fields
// are exported and can be read or written directly instead.
func (f *BiQuadFilter[T]) Coefficients() (b, a []float64) {
	return []float64{f.B0, f.B1, f.B2}, []float64{f.A0, f.A1, f.A2}
}

// Reset clears the delay lines, so the filter starts a new stream from silence.
func (f *BiQuadFilter[T]) Reset() {
	f.prevIn = [2]float64{}
	f.prevOut = [2]float64{}
}

// checkBiQuadArgs validates the arguments every response constructor shares and
// returns w0, the cutoff or center as an angle per sample. shape is Q for the
// resonant responses and shelfSlope for the two shelves; shapeName names it in
// the error, since a shelf reporting a bad "Q" would send a caller looking at
// the wrong argument.
//
// The comparisons are written so that a NaN fails them: !(x > 0) rather than
// x <= 0. A frequency at or above Nyquist has nothing to sample, and one at
// zero makes sinW0 zero and alpha with it, which leaves A0 == 1 and a filter
// that is not the response asked for.
func checkBiQuadArgs(sampleRate, freq, shape float64, shapeName string) (w0 float64, err error) {
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

// NewLowPassBiQuad returns a low-pass biquad with the given cutoff frequency
// and Q, both in the same units as sampleRate.
//
//	H(s) = 1 / (s^2 + s/Q + 1)
func NewLowPassBiQuad[T Float](sampleRate, cutoffFreq, q float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, cutoffFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiQuad[T](
		(1-cosW0)/2, 1-cosW0, (1-cosW0)/2,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewHighPassBiQuad returns a high-pass biquad with the given cutoff frequency
// and Q.
//
//	H(s) = s^2 / (s^2 + s/Q + 1)
func NewHighPassBiQuad[T Float](sampleRate, cutoffFreq, q float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, cutoffFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiQuad[T](
		(1+cosW0)/2, -(1 + cosW0), (1+cosW0)/2,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBandPassConstantSkirtGainBiQuad returns a band-pass biquad whose skirt
// gain is constant, which makes the peak gain equal to q.
//
//	H(s) = s / (s^2 + s/Q + 1)
func NewBandPassConstantSkirtGainBiQuad[T Float](sampleRate, centerFreq, q float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiQuad[T](
		sinW0/2, 0, -sinW0/2, // +/- Q*alpha
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewBandPassConstantPeakGainBiQuad returns a band-pass biquad whose peak gain
// is 0 dB whatever q is.
//
//	H(s) = (s/Q) / (s^2 + s/Q + 1)
func NewBandPassConstantPeakGainBiQuad[T Float](sampleRate, centerFreq, q float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiQuad[T](
		alpha, 0, -alpha,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewNotchBiQuad returns a notch biquad, which rejects a band around
// centerFreq and passes everything else. q sets how narrow the notch is.
//
//	H(s) = (s^2 + 1) / (s^2 + s/Q + 1)
func NewNotchBiQuad[T Float](sampleRate, centerFreq, q float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiQuad[T](
		1, -2*cosW0, 1,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewAllPassBiQuad returns an all-pass biquad, which leaves the magnitude
// response flat and only shifts phase around centerFreq.
//
//	H(s) = (s^2 - s/Q + 1) / (s^2 + s/Q + 1)
func NewAllPassBiQuad[T Float](sampleRate, centerFreq, q float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	return NewBiQuad[T](
		1-alpha, -2*cosW0, 1+alpha,
		1+alpha, -2*cosW0, 1-alpha,
	)
}

// NewPeakingEQBiQuad returns a peaking EQ biquad, which boosts or cuts a band
// around centerFreq by dbGain decibels and leaves the rest alone.
//
//	H(s) = (s^2 + s*(A/Q) + 1) / (s^2 + s/(A*Q) + 1)
func NewPeakingEQBiQuad[T Float](sampleRate, centerFreq, q, dbGain float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, centerFreq, q, "Q")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	alpha := sinW0 / (2 * q)
	a := math.Pow(10, dbGain/40)
	return NewBiQuad[T](
		1+alpha*a, -2*cosW0, 1-alpha*a,
		1+alpha/a, -2*cosW0, 1-alpha/a,
	)
}

// NewLowShelfBiQuad returns a low shelf biquad, which applies dbGain decibels
// of gain below cutoffFreq and unity gain above it.
//
//	H(s) = A * (s^2 + (sqrt(A)/Q)*s + A)/(A*s^2 + (sqrt(A)/Q)*s + 1)
//
// shelfSlope controls how abrupt the transition is. At 1 the shelf is as steep
// as it can be while the gain stays monotonic in frequency; for other values
// the slope in dB/octave is proportional to it, for a fixed cutoffFreq /
// sampleRate and dbGain. Past 1 the square root below can go negative, which
// NewBiQuad rejects rather than returning a filter of NaNs.
func NewLowShelfBiQuad[T Float](sampleRate, cutoffFreq, shelfSlope, dbGain float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, cutoffFreq, shelfSlope, "shelf slope")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	a := math.Pow(10, dbGain/40.0)
	alpha := sinW0 / 2 * math.Sqrt((a+1/a)*(1/shelfSlope-1)+2)
	temp := 2 * math.Sqrt(a) * alpha
	return NewBiQuad[T](
		a*((a+1)-(a-1)*cosW0+temp),
		2*a*((a-1)-(a+1)*cosW0),
		a*((a+1)-(a-1)*cosW0-temp),
		(a+1)+(a-1)*cosW0+temp,
		-2*((a-1)+(a+1)*cosW0),
		(a+1)+(a-1)*cosW0-temp,
	)
}

// NewHighShelfBiQuad returns a high shelf biquad, which applies dbGain decibels
// of gain above cutoffFreq and unity gain below it. shelfSlope is as described
// for NewLowShelfBiQuad.
//
//	H(s) = A * (A*s^2 + (sqrt(A)/Q)*s + 1)/(s^2 + (sqrt(A)/Q)*s + A)
func NewHighShelfBiQuad[T Float](sampleRate, cutoffFreq, shelfSlope, dbGain float64) (*BiQuadFilter[T], error) {
	w0, err := checkBiQuadArgs(sampleRate, cutoffFreq, shelfSlope, "shelf slope")
	if err != nil {
		return nil, err
	}
	sinW0, cosW0 := math.Sincos(w0)
	a := math.Pow(10, dbGain/40)
	alpha := sinW0 / 2 * math.Sqrt((a+1/a)*(1/shelfSlope-1)+2)
	temp := 2 * math.Sqrt(a) * alpha
	return NewBiQuad[T](
		a*((a+1)+(a-1)*cosW0+temp),
		-2*a*((a-1)+(a+1)*cosW0),
		a*((a+1)+(a-1)*cosW0-temp),
		(a+1)-(a-1)*cosW0+temp,
		2*((a-1)-(a+1)*cosW0),
		(a+1)-(a-1)*cosW0-temp,
	)
}
