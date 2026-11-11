// Package analysis measures what a filter does to a signal, and what is in a
// recording.
//
// A filter is measured as a black box: a dsp.Processor[float64, float64] and
// the sample rate at its input, so a biquad, a set of FIR taps and a decimator
// are all measured the same way. A processor that also has real
// RealCoefficients is measured exactly from its transfer function instead,
// which is faster and resolves far more finely; MethodAuto picks between the
// two. Adapt wraps a processor at another sample width.
//
// Measurement is float64, whatever the device under test's width: a float32
// measurement path has a noise floor above the stopband of any filter worth
// looking at, so offering one would advertise an accuracy it could not deliver.
//
// Recordings are analyzed by single-pass builders that implement Sink, so a
// file larger than memory is read once and feeds every view. The plot package
// draws the results.
package analysis

import (
	"errors"

	"samuelks.com/dsp"
)

// RealCoefficients is implemented by a processor that is a linear
// time-invariant filter with a known transfer function B(z)/A(z), which lets
// the analysis evaluate the response exactly instead of measuring it. The
// slices are copies, and are the b and a dsp.Freqz takes. ok is false when the
// coefficients are complex, and the analysis then measures instead.
//
// The rate changers do not implement it, and cannot: a rate changer is
// periodically time-varying, so it has no single transfer function.
type RealCoefficients interface {
	RealCoefficients() (b, a []float64, ok bool)
}

// Ratio is implemented by a processor that changes the sample rate, as every
// rate changer in dsp does: in input samples become out output samples. A
// processor without it runs at one rate.
type Ratio interface {
	Ratio() (in, out int)
}

// Adapt returns p as a float64 processor, so a filter at any sample width can
// be measured. Samples are narrowed to T on the way in, on the real axis for a
// complex T, and the real part is widened on the way out. The result forwards
// RealCoefficients and Ratio from p; without that the exact transfer-function
// path would silently fall back to black-box measurement, and a rate changer
// would be measured at the wrong output rate.
//
// A filter with complex coefficients has a response that is not symmetric
// about DC, which a stimulus on the real axis cannot show.
func Adapt[T dsp.Sample](p dsp.Processor[T, T]) dsp.Processor[float64, float64] {
	if q, ok := any(p).(dsp.Processor[float64, float64]); ok {
		return q
	}
	return &adapter[T]{p: p}
}

type adapter[T dsp.Sample] struct {
	p        dsp.Processor[T, T]
	in, outs []T
}

func (a *adapter[T]) OutputLen(inputs int) int { return a.p.OutputLen(inputs) }
func (a *adapter[T]) Reset()                   { a.p.Reset() }

func (a *adapter[T]) Process(dst, src []float64) int {
	a.in = narrowInto(a.in, src)
	a.outs = resize(a.outs, a.p.OutputLen(len(src)))
	n := a.p.Process(a.outs, a.in)
	if len(dst) < n {
		panic("analysis: adapter dst is shorter than OutputLen(len(src))")
	}
	widenInto(dst[:0], a.outs[:n])
	return n
}

func (a *adapter[T]) RealCoefficients() ([]float64, []float64, bool) {
	c, ok := a.p.(RealCoefficients)
	if !ok {
		return nil, nil, false
	}
	return c.RealCoefficients()
}

func (a *adapter[T]) Ratio() (in, out int) {
	if r, ok := a.p.(Ratio); ok {
		return r.Ratio()
	}
	return 1, 1
}

// realCoefficients returns p's transfer function, if it has a real one.
func realCoefficients(p dsp.Processor[float64, float64]) (b, a []float64, ok bool) {
	c, ok := p.(RealCoefficients)
	if !ok {
		return nil, nil, false
	}
	return c.RealCoefficients()
}

// rates returns the input and output rates of p running at rate on its input.
func rates(p dsp.Processor[float64, float64], rate float64) (in, out float64) {
	r, ok := p.(Ratio)
	if !ok {
		return rate, rate
	}
	i, o := r.Ratio()
	return rate, rate * float64(o) / float64(i)
}

// run resets p and returns its whole output for in, reusing dst.
func run(p dsp.Processor[float64, float64], dst, in []float64) []float64 {
	p.Reset()
	return process(p, dst, in)
}

// process returns p's output for in, reusing dst, without resetting it.
func process(p dsp.Processor[float64, float64], dst, in []float64) []float64 {
	dst = resize(dst, p.OutputLen(len(in)))
	return dst[:p.Process(dst, in)]
}

func checkDevice(p dsp.Processor[float64, float64], rate float64) error {
	if p == nil {
		return errors.New("analysis: nil processor")
	}
	return checkRate(rate)
}

func checkRate(rate float64) error {
	if !(rate > 0) {
		return errors.New("analysis: sample rate must be positive")
	}
	return nil
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
