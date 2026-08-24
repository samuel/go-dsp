// Package dspviz measures and draws what a filter does to a signal.
//
// Everything here works through Processor, a black box that turns samples at
// one rate into samples at another, so a biquad, a set of FIR taps and a
// decimator are all measured the same way. A processor that also implements
// Coefficients is measured exactly from its transfer function instead, which is
// faster and resolves far more finely; MethodAuto picks between the two.
//
// Measurement is float64, whatever the device under test's width: a float32
// measurement path has a noise floor above the stopband of any filter worth
// looking at, so offering one would advertise an accuracy it could not deliver.
package dspviz

import (
	"errors"

	"github.com/samuel/go-dsp/dsp"
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

// Coefficients is implemented by a Processor that is a linear time-invariant
// filter with a known transfer function B(z)/A(z), which lets the analysis
// evaluate the response exactly instead of measuring it. The slices are copies,
// and are the b and a dsp.Freqz takes.
//
// The decimators do not implement it, and cannot: a rate changer is
// periodically time-varying, so it has no single transfer function.
type Coefficients interface {
	Coefficients() (b, a []float64)
}

func checkRate(rate float64) error {
	if !(rate > 0) {
		return errors.New("dspviz: sample rate must be positive")
	}
	return nil
}

// biquadProcessor adapts dsp.BiQuadFilter at any sample width.
type biquadProcessor[T dsp.Float] struct {
	f    *dsp.BiQuadFilter[T]
	rate float64
	buf  []T
}

// NewBiQuadProcessor measures a biquad running at sampleRate. A biquad keeps
// float64 state at any sample width, so the width shows only in the rounding of
// each output sample.
func NewBiQuadProcessor[T dsp.Float](f *dsp.BiQuadFilter[T], sampleRate float64) (Processor, error) {
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
	p.f.Filter(p.buf, p.buf)
	return widenInto(dst, p.buf)
}

func (p *biquadProcessor[T]) Coefficients() (b, a []float64) { return p.f.Coefficients() }

// iirProcessor adapts dsp.IIRFilter.
type iirProcessor struct {
	f    *dsp.IIRFilter[float64]
	rate float64
	buf  []float64
}

// NewIIRProcessor measures an IIR filter running at sampleRate.
//
// It takes float64 samples specifically, rather than being generic over
// dsp.Sample: an IIR filter with complex coefficients has a response that is
// not symmetric about DC, and a single magnitude and phase curve cannot express
// it. Such a filter needs a two-sided analysis that is not here.
func NewIIRProcessor(f *dsp.IIRFilter[float64], sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil IIR filter")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &iirProcessor{f: f, rate: sampleRate}, nil
}

func (p *iirProcessor) Rates() (in, out float64) { return p.rate, p.rate }
func (p *iirProcessor) Reset()                   { p.f.Reset() }

func (p *iirProcessor) Process(dst, src []float64) []float64 {
	p.buf = resize(p.buf, len(src))
	p.f.Filter(p.buf, src)
	return append(dst, p.buf...)
}

func (p *iirProcessor) Coefficients() (b, a []float64) { return p.f.Coefficients() }

// firProcessor adapts dsp.FIRFilter, so the catalog's firpm entry measures the
// filter a caller would actually run rather than a second convolution written
// here. It keeps its state between calls like every other processor, so a block
// boundary is not a discontinuity.
type firProcessor struct {
	f    *dsp.FIRFilter[float64]
	rate float64
	buf  []float64
}

// NewFIRProcessor measures a FIR filter with the given taps, running at
// sampleRate. The taps are copied.
func NewFIRProcessor(taps []float64, sampleRate float64) (Processor, error) {
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	f, err := dsp.NewFIRFilter(taps)
	if err != nil {
		return nil, err
	}
	return &firProcessor{f: f, rate: sampleRate}, nil
}

func (p *firProcessor) Rates() (in, out float64) { return p.rate, p.rate }

func (p *firProcessor) Reset() { p.f.Reset() }

func (p *firProcessor) Process(dst, src []float64) []float64 {
	if cap(p.buf) < len(src) {
		p.buf = make([]float64, len(src))
	}
	out := p.buf[:len(src)]
	p.f.Filter(out, src)
	return append(dst, out...)
}

func (p *firProcessor) Coefficients() (b, a []float64) { return p.f.Coefficients() }

// firDecimProcessor adapts dsp.FIRDecimator: an anti-alias filter and the rate
// change in one pass, which is what a real decimation stage looks like.
//
// It deliberately does not implement Coefficients. A rate changer is
// periodically time-varying rather than time-invariant, so it has no single
// transfer function and has to be measured by injecting tones -- which is the
// point of measuring it here rather than reasoning about the taps.
type firDecimProcessor struct {
	f    *dsp.FIRDecimator[float64]
	rate float64
	buf  []float64
}

// NewFIRDecimatorProcessor measures a decimating FIR filter with the given taps
// and factor, running at sampleRate on its input. The taps are copied.
func NewFIRDecimatorProcessor(taps []float64, factor int, sampleRate float64) (Processor, error) {
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	f, err := dsp.NewFIRDecimator(taps, factor)
	if err != nil {
		return nil, err
	}
	return &firDecimProcessor{f: f, rate: sampleRate}, nil
}

func (p *firDecimProcessor) Rates() (in, out float64) {
	return p.rate, p.rate / float64(p.f.Factor())
}

func (p *firDecimProcessor) Reset() { p.f.Reset() }

func (p *firDecimProcessor) Process(dst, src []float64) []float64 {
	p.buf = resize(p.buf, p.f.OutputLen(len(src)))
	n := p.f.Filter(p.buf, src)
	return append(dst, p.buf[:n]...)
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
	p.f.Filter(p.buf, p.buf)
	return widenInto(dst, p.buf)
}

func (p *dcProcessor[T]) Coefficients() (b, a []float64) { return p.f.Coefficients() }

// complexDownsampleProcessor adapts dsp.BoxcarDecimator by
// putting the real signal on the real axis and reading it back off.
type complexDownsampleProcessor struct {
	f    *dsp.BoxcarDecimator
	rate float64
	buf  []complex64
}

// NewComplexDownsampleProcessor measures the boxcar complex decimator, fed a
// real signal on the real axis, running at sampleRate on its input.
//
// The filter works in complex64, so this path cannot resolve much past -130 dB.
// That is not a limitation in practice: a boxcar's own first sidelobe is only
// 13 dB down, three orders of magnitude above the floor.
func NewComplexDownsampleProcessor(f *dsp.BoxcarDecimator, sampleRate float64) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil complex decimator")
	}
	if err := checkRate(sampleRate); err != nil {
		return nil, err
	}
	return &complexDownsampleProcessor{f: f, rate: sampleRate}, nil
}

func (p *complexDownsampleProcessor) Rates() (in, out float64) {
	return p.rate, p.rate / float64(p.f.Factor())
}

func (p *complexDownsampleProcessor) Reset() { p.f.Reset() }

func (p *complexDownsampleProcessor) Process(dst, src []float64) []float64 {
	p.buf = resize(p.buf, len(src))
	for i, x := range src {
		p.buf[i] = complex(float32(x), 0)
	}
	for _, v := range p.f.FilterInPlace(p.buf) {
		dst = append(dst, float64(real(v)))
	}
	return dst
}

// rationalDownsampleProcessor adapts dsp.RationalBoxcarDecimator.
type rationalDownsampleProcessor struct {
	f   *dsp.RationalBoxcarDecimator
	buf []float32
}

// NewRationalDownsampleProcessor measures the rational decimator. Its rates
// come from the filter, so they cannot disagree with what it was built for.
//
// It divides each output by the number of inputs that went into it, which
// alternates for a ratio that is not a whole number, so it is time-varying in a
// way a fixed divisor would not be.
func NewRationalDownsampleProcessor(f *dsp.RationalBoxcarDecimator) (Processor, error) {
	if f == nil {
		return nil, errors.New("dspviz: nil rational decimator")
	}
	return &rationalDownsampleProcessor{f: f}, nil
}

func (p *rationalDownsampleProcessor) Rates() (in, out float64) {
	fast, slow := p.f.Rates()
	return float64(fast), float64(slow)
}

func (p *rationalDownsampleProcessor) Reset() { p.f.Reset() }

func (p *rationalDownsampleProcessor) Process(dst, src []float64) []float64 {
	p.buf = resize(p.buf, len(src))
	for i, x := range src {
		p.buf[i] = float32(x)
	}
	for _, v := range p.f.FilterInPlace(p.buf) {
		dst = append(dst, float64(v))
	}
	return dst
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

func narrowInto[T dsp.Float](dst []T, src []float64) []T {
	dst = resize(dst, len(src))
	for i, v := range src {
		dst[i] = T(v)
	}
	return dst
}

func widenInto[T dsp.Float](dst []float64, src []T) []float64 {
	for _, v := range src {
		dst = append(dst, float64(v))
	}
	return dst
}
