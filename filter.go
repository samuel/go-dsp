package dsp

import (
	"errors"
	"slices"
)

// IIRFilter is a direct form 1 IIR filter of arbitrary order, over real or
// complex samples at either width. It keeps the delay lines between calls so a
// stream can be filtered in blocks.
type IIRFilter[T Sample] struct {
	bCoef, aCoef []T
	pIn, pOut    []T
}

// NewIIRFilter returns an IIR filter with the given feedforward (b) and
// feedback (a) coefficients, both normalized by aCoef[0]. It returns an error
// unless the two are the same length and hold at least two coefficients each:
// with one, there is no delay line and y[n] = (b[0]/a[0])*x[n], which is a
// gain rather than a filter.
func NewIIRFilter[T Sample](bCoef, aCoef []T) (*IIRFilter[T], error) {
	if len(bCoef) != len(aCoef) {
		return nil, errors.New("dsp: IIR filter needs len(b) == len(a)")
	}
	if len(bCoef) < 2 {
		return nil, errors.New("dsp: IIR filter needs at least two coefficients; one pair is a gain, not a filter")
	}
	if aCoef[0] == 0 {
		return nil, errors.New("dsp: IIR filter needs a non-zero a[0]; every coefficient is normalized by it")
	}
	b := slices.Clone(bCoef)
	a := slices.Clone(aCoef)
	a0 := a[0]
	for i := range b {
		b[i] /= a0
	}
	for i := range a[1:] {
		a[i+1] /= a0
	}
	// Exactly 1, so the recursion below can leave it out. Dividing a[0] by
	// itself is not guaranteed to give that for a complex coefficient.
	a[0] = 1
	return &IIRFilter[T]{
		bCoef: b,
		aCoef: a,
		pIn:   make([]T, len(b)-1),
		pOut:  make([]T, len(b)-1),
	}, nil
}

// NewRealCoefIIRFilter returns an IIR filter over complex samples built from
// real coefficients, which is the usual case: a filter with a real impulse
// response has real coefficients whether or not the signal it filters does.
// The complex sample type has to be given explicitly, as in
// NewRealCoefIIRFilter[complex64](b, a).
func NewRealCoefIIRFilter[C Complex, F Float](bCoef, aCoef []F) (*IIRFilter[C], error) {
	// Widening is exact and changes no length, so NewIIRFilter's checks say
	// everything there is to say about these arguments too.
	return NewIIRFilter(rtoc[C](bCoef), rtoc[C](aCoef))
}

// Filter writes one output sample per input sample, over
// min(len(dst), len(src)) of them. The slices may be the same. Samples past
// that count are not consumed, so the delay lines match what was written.
func (f *IIRFilter[T]) Filter(dst, src []T) {
	for i, s := range src[:min(len(dst), len(src))] {
		dst[i] = f.step(s)
	}
}

// FilterOne filters a single sample.
func (f *IIRFilter[T]) FilterOne(s T) T { return f.step(s) }

// step advances the recursion by one sample and returns the output. Both entry
// points go through it, so the two cannot drift apart: they are the same
// arithmetic in the same order, which is what makes filtering a block and
// filtering its samples one at a time give bit-identical streams.
func (f *IIRFilter[T]) step(s T) T {
	sum := f.bCoef[0] * s
	for j, p := range f.pIn {
		sum += f.bCoef[j+1]*p - f.aCoef[j+1]*f.pOut[j]
	}
	for i := len(f.pIn) - 1; i > 0; i-- {
		f.pIn[i] = f.pIn[i-1]
		f.pOut[i] = f.pOut[i-1]
	}
	f.pIn[0] = s
	f.pOut[0] = sum
	return sum
}

// Coefficients returns copies of the feedforward and feedback coefficients as
// the constructor normalized them, so a[0] is exactly 1. They are the b and a a
// transfer function is written in. Freqz takes float64 slices, so a filter at
// another width has to be converted first.
func (f *IIRFilter[T]) Coefficients() (b, a []T) {
	return slices.Clone(f.bCoef), slices.Clone(f.aCoef)
}

// Reset clears the delay lines, so the filter starts a new stream from silence.
func (f *IIRFilter[T]) Reset() {
	clear(f.pIn)
	clear(f.pOut)
}

// DCFilter is a one-pole DC blocker: a high-pass with its zero at DC and its
// pole at a, which removes a constant offset. a is just below 1, and the
// closer it is the narrower the notch. State is kept between calls.
//
// The pole and the state are float64 whatever the sample type is, as
// BiQuadFilter's are and for the same reason: this is a leaky integrator whose
// pole sits just inside the unit circle, exactly where a narrower accumulator
// shows up as drift. The sample type therefore has to be given, as in
// NewDCFilter[float32](0.95).
type DCFilter[T Float] struct {
	a float64
	w float64
}

// NewDCFilter returns a DC blocker with pole a, which must be in [0, 1).
func NewDCFilter[T Float](a float64) (*DCFilter[T], error) {
	if !(a >= 0 && a < 1) {
		return nil, errors.New("dsp: DC blocker pole must be in [0, 1)")
	}
	return &DCFilter[T]{a: a}, nil
}

// Filter writes one output sample per input sample, over
// min(len(dst), len(src)) of them. The slices may be the same.
func (f *DCFilter[T]) Filter(dst, src []T) {
	lw := f.w
	for i, x := range src[:min(len(dst), len(src))] {
		w := float64(x) + f.a*lw
		dst[i] = T(w - lw)
		lw = w
	}
	f.w = lw
}

// FilterOne filters a single sample.
func (f *DCFilter[T]) FilterOne(x T) T {
	w := float64(x) + f.a*f.w
	y := w - f.w
	f.w = w
	return T(y)
}

// Coefficients returns the blocker written as an ordinary IIR filter: b is
// {1, -1} for the zero at DC and a is {1, -pole}, so Freqz and the analysis
// tools can treat it like any other filter. They are float64 whatever the
// sample type is, as BiQuadFilter's are, which is what makes every real filter
// here satisfy one Coefficients interface.
func (f *DCFilter[T]) Coefficients() (b, a []float64) {
	return []float64{1, -1}, []float64{1, -f.a}
}

// Reset clears the filter state, so it starts a new stream from silence.
func (f *DCFilter[T]) Reset() {
	f.w = 0
}
