// Package dspviz measures and draws what a filter does to a signal.
//
// Everything here works through Processor, a black box that turns samples at
// one rate into samples at another, so a biquad, a set of FIR taps and a
// decimator are all measured the same way. A processor that also implements
// RealCoefficients is measured exactly from its transfer function instead, which is
// faster and resolves far more finely; MethodAuto picks between the two.
//
// Measurement is float64, whatever the device under test's width: a float32
// measurement path has a noise floor above the stopband of any filter worth
// looking at, so offering one would advertise an accuracy it could not deliver.
package dspviz

import (
	"errors"

	"samuelks.com/dsp"
)

// Processor is a black-box signal processor: something that turns a stream of
// samples at one rate into a stream at another.
//
// Process appends the output for src to dst and returns the extended slice,
// so a processor that emits fewer or more samples than it consumes needs no
// special case. State carries between calls, and Reset returns the processor to
// the state its constructor left it in. Like everything in dsp, no
// implementation is safe for concurrent use.
type Processor interface {
	Process(dst, src []float64) []float64
	Rates() (in, out float64)
	Reset()
}

// RealCoefficients is implemented by a Processor that is a linear
// time-invariant filter with a known transfer function B(z)/A(z), which lets
// the analysis evaluate the response exactly instead of measuring it. The
// slices are copies, and are the b and a dsp.Freqz takes. ok is false when the
// coefficients are complex, and the analysis then measures instead.
//
// The decimators do not implement it, and cannot: a rate changer is
// periodically time-varying, so it has no single transfer function.
type RealCoefficients interface {
	RealCoefficients() (b, a []float64, ok bool)
}

// realCoefficients returns p's transfer function, if it has a real one.
func realCoefficients(p Processor) (b, a []float64, ok bool) {
	c, ok := p.(RealCoefficients)
	if !ok {
		return nil, nil, false
	}
	return c.RealCoefficients()
}

// ratioRates returns the input and output rates of a rate changer whose input
// runs at rate.
func ratioRates(rate float64, r interface{ Ratio() (in, out int) }) (in, out float64) {
	i, o := r.Ratio()
	return rate, rate * float64(o) / float64(i)
}

func checkRate(rate float64) error {
	if !(rate > 0) {
		return errors.New("dspviz: sample rate must be positive")
	}
	return nil
}

// biquadProcessor adapts dsp.Biquad at any sample width.
type biquadProcessor[T dsp.Float] struct {
	f    *dsp.Biquad[T]
	rate float64
	buf  []T
}

// NewBiquadProcessor measures a biquad running at sampleRate. A biquad keeps
// float64 state at any sample width, so the width shows only in the rounding of
// each output sample.
func NewBiquadProcessor[T dsp.Float](f *dsp.Biquad[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil biquad")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &biquadProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *biquadProcessor[T]) Rates() (in, out float64) { return p.rate, p.rate }
func (p *biquadProcessor[T]) Reset()                   { p.f.Reset() }

func (p *biquadProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	p.f.Process(p.buf, p.buf)
	return widenInto(dst, p.buf)
}

func (p *biquadProcessor[T]) RealCoefficients() (b, a []float64, ok bool) {
	return p.f.RealCoefficients()
}

// iirProcessor adapts dsp.IIRFilter.
type iirProcessor[T dsp.Float] struct {
	f    *dsp.IIRFilter[T]
	rate float64
	buf  []T
}

// NewIIRProcessor measures an IIR filter running at sampleRate.
//
// It takes real samples rather than any dsp.Sample: an IIR filter with complex
// coefficients has a response that is not symmetric about DC, and a single
// magnitude and phase curve cannot express it. Such a filter needs a two-sided
// analysis that is not here.
func NewIIRProcessor[T dsp.Float](f *dsp.IIRFilter[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil IIR filter")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &iirProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *iirProcessor[T]) Rates() (in, out float64) { return p.rate, p.rate }
func (p *iirProcessor[T]) Reset()                   { p.f.Reset() }

func (p *iirProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	p.f.Process(p.buf, p.buf)
	return widenInto(dst, p.buf)
}

func (p *iirProcessor[T]) RealCoefficients() (b, a []float64, ok bool) {
	return p.f.RealCoefficients()
}

// firProcessor adapts dsp.FIRFilter.
type firProcessor[T dsp.Float] struct {
	f    *dsp.FIRFilter[T]
	rate float64
	buf  []T
}

// NewFIRProcessor measures an FIR filter running at sampleRate.
func NewFIRProcessor[T dsp.Float](f *dsp.FIRFilter[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil FIR filter")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &firProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *firProcessor[T]) Rates() (in, out float64) { return p.rate, p.rate }
func (p *firProcessor[T]) Reset()                   { p.f.Reset() }

func (p *firProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	p.f.Process(p.buf, p.buf)
	return widenInto(dst, p.buf)
}

func (p *firProcessor[T]) RealCoefficients() (b, a []float64, ok bool) {
	return p.f.RealCoefficients()
}

// firDecimatorProcessor adapts dsp.FIRDecimator: an anti-alias filter and the
// rate change in one pass, which is what a real decimation stage looks like.
//
// It deliberately does not implement RealCoefficients, and neither do the other
// rate changers. A rate changer is periodically time-varying rather than
// time-invariant, so it has no single transfer function and has to be
// measured by injecting tones -- which is the point of measuring it here
// rather than reasoning about the taps.
type firDecimatorProcessor[T dsp.Float] struct {
	f    *dsp.FIRDecimator[T]
	rate float64
	buf  []T
}

// NewFIRDecimatorProcessor measures a decimating FIR filter running at
// sampleRate on its input.
func NewFIRDecimatorProcessor[T dsp.Float](f *dsp.FIRDecimator[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil FIR decimator")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &firDecimatorProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *firDecimatorProcessor[T]) Rates() (in, out float64) {
	return ratioRates(p.rate, p.f)
}

func (p *firDecimatorProcessor[T]) Reset() { p.f.Reset() }

func (p *firDecimatorProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	return widenInto(dst, p.buf[:p.f.Process(p.buf, p.buf)])
}

// firInterpolatorProcessor adapts dsp.FIRInterpolator.
type firInterpolatorProcessor[T dsp.Float] struct {
	f        *dsp.FIRInterpolator[T]
	rate     float64
	in, outs []T
}

// NewFIRInterpolatorProcessor measures an interpolating FIR filter running at
// sampleRate on its input.
func NewFIRInterpolatorProcessor[T dsp.Float](f *dsp.FIRInterpolator[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil FIR interpolator")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &firInterpolatorProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *firInterpolatorProcessor[T]) Rates() (in, out float64) {
	return ratioRates(p.rate, p.f)
}

func (p *firInterpolatorProcessor[T]) Reset() { p.f.Reset() }

func (p *firInterpolatorProcessor[T]) Process(dst, src []float64) []float64 {
	p.in = narrowInto(p.in, src)
	p.outs = resize(p.outs, p.f.OutputLen(len(src)))
	n := p.f.Process(p.outs, p.in)
	return widenInto(dst, p.outs[:n])
}

// dcProcessor adapts dsp.DCFilter.
type dcProcessor[T dsp.Float] struct {
	f    *dsp.DCFilter[T]
	rate float64
	buf  []T
}

// NewDCProcessor measures a DC blocker running at sampleRate.
func NewDCProcessor[T dsp.Float](f *dsp.DCFilter[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil DC filter")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &dcProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *dcProcessor[T]) Rates() (in, out float64) { return p.rate, p.rate }
func (p *dcProcessor[T]) Reset()                   { p.f.Reset() }

func (p *dcProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	p.f.Process(p.buf, p.buf)
	return widenInto(dst, p.buf)
}

func (p *dcProcessor[T]) RealCoefficients() (b, a []float64, ok bool) {
	return p.f.RealCoefficients()
}

// boxcarDecimatorProcessor adapts dsp.BoxcarDecimator. A complex decimator is
// fed the real signal on the real axis and read back off it.
type boxcarDecimatorProcessor[T dsp.Sample] struct {
	f    *dsp.BoxcarDecimator[T]
	rate float64
	buf  []T
}

// NewBoxcarDecimatorProcessor measures a boxcar decimator running at
// sampleRate on its input.
//
// At complex64 or float32 this path cannot resolve much past -130 dB. That is
// not a limitation in practice: a boxcar's own first sidelobe is only 13 dB
// down, three orders of magnitude above the floor.
func NewBoxcarDecimatorProcessor[T dsp.Sample](f *dsp.BoxcarDecimator[T], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil boxcar decimator")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &boxcarDecimatorProcessor[T]{f: f, rate: sampleRate}, nil
}

func (p *boxcarDecimatorProcessor[T]) Rates() (in, out float64) {
	return ratioRates(p.rate, p.f)
}

func (p *boxcarDecimatorProcessor[T]) Reset() { p.f.Reset() }

func (p *boxcarDecimatorProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	return widenInto(dst, p.buf[:p.f.Process(p.buf, p.buf)])
}

// rationalBoxcarDecimatorProcessor adapts dsp.RationalBoxcarDecimator.
type rationalBoxcarDecimatorProcessor[T dsp.Sample] struct {
	f   *dsp.RationalBoxcarDecimator[T]
	buf []T
}

// NewRationalBoxcarDecimatorProcessor measures a rational boxcar decimator.
// Its rates come from the filter, so they cannot disagree with what it was
// built for.
//
// It divides each output by the number of inputs that went into it, which
// alternates for a ratio that is not a whole number, so it is time-varying in a
// way a fixed divisor would not be.
func NewRationalBoxcarDecimatorProcessor[T dsp.Sample](f *dsp.RationalBoxcarDecimator[T]) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil rational boxcar decimator")
	}
	return &rationalBoxcarDecimatorProcessor[T]{f: f}, nil
}

func (p *rationalBoxcarDecimatorProcessor[T]) Rates() (in, out float64) {
	fast, slow := p.f.Ratio()
	return float64(fast), float64(slow)
}

func (p *rationalBoxcarDecimatorProcessor[T]) Reset() { p.f.Reset() }

func (p *rationalBoxcarDecimatorProcessor[T]) Process(dst, src []float64) []float64 {
	p.buf = narrowInto(p.buf, src)
	return widenInto(dst, p.buf[:p.f.Process(p.buf, p.buf)])
}

// funcProcessor wraps a plain function.
type funcProcessor struct {
	fn      ProcessFunc
	reset   func()
	in, out float64
}

// ProcessFunc is the signature of Processor.Process, for NewFuncProcessor.
type ProcessFunc func(dst, src []float64) []float64

// NewFuncProcessor adapts a plain function, for anything the constructors above
// do not cover. reset may be nil for a stateless function.
func NewFuncProcessor(fn ProcessFunc, reset func(), inRate, outRate float64) (Processor, error) {
	if fn == nil {
		return nil, errors.New("dspviz: nil process function")
	}
	if err := checkRate(inRate); err != nil {
		return nil, err
	}
	if err := checkRate(outRate); err != nil {
		return nil, err
	}
	return &funcProcessor{fn: fn, reset: reset, in: inRate, out: outRate}, nil
}

func (p *funcProcessor) Rates() (in, out float64) { return p.in, p.out }

func (p *funcProcessor) Reset() {
	if p.reset != nil {
		p.reset()
	}
}

func (p *funcProcessor) Process(dst, src []float64) []float64 { return p.fn(dst, src) }

// Identity returns a processor that passes its src through unchanged at
// sampleRate. It is the reference the measurement itself is checked against:
// anything it reports other than a flat unity response is the measurement's own
// error rather than a filter's.
func Identity(sampleRate float64) (Processor, error) {
	return NewFuncProcessor(func(dst, src []float64) []float64 {
		return append(dst, src...)
	}, nil, sampleRate, sampleRate)
}

func resize[T any](s []T, n int) []T {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]T, n)
}

// narrowInto puts src into dst at T's width, on the real axis for a complex T.
func narrowInto[T dsp.Sample](dst []T, src []float64) []T {
	dst = resize(dst, len(src))
	switch d := any(dst).(type) {
	case []float32:
		for i, v := range src {
			d[i] = float32(v)
		}
	case []float64:
		copy(d, src)
	case []complex64:
		for i, v := range src {
			d[i] = complex(float32(v), 0)
		}
	case []complex128:
		for i, v := range src {
			d[i] = complex(v, 0)
		}
	}
	return dst
}

// widenInto appends src to dst as float64, taking the real part of a complex
// T.
func widenInto[T dsp.Sample](dst []float64, src []T) []float64 {
	switch s := any(src).(type) {
	case []float32:
		for _, v := range s {
			dst = append(dst, float64(v))
		}
	case []float64:
		dst = append(dst, s...)
	case []complex64:
		for _, v := range s {
			dst = append(dst, float64(real(v)))
		}
	case []complex128:
		for _, v := range s {
			dst = append(dst, real(v))
		}
	}
	return dst
}
