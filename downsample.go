package dsp

import (
	"errors"

	"samuelks.com/dsp/internal/f32"
)

// BoxcarDecimator decimates by a whole-number factor, summing each group of
// that many consecutive samples into one output. That boxcar average is a
// cheap low-pass, so it also serves as the anti-alias filter. The sum is not
// divided by the factor, so the filter has a gain of the factor.
//
// complex64 runs a kernel with VFP assembly on 32-bit arm; the other widths
// run the same loop in Go.
type BoxcarDecimator[T Sample] struct {
	factor int

	now       T
	prevIndex int
}

// NewBoxcarDecimator returns a decimator by the given factor, which must be at
// least 1. The sample type cannot be inferred and has to be given, as in
// NewBoxcarDecimator[complex64](4).
func NewBoxcarDecimator[T Sample](factor int) (*BoxcarDecimator[T], error) {
	if factor < 1 {
		return nil, errors.New("dsp: boxcar decimator needs a factor of at least 1")
	}
	return &BoxcarDecimator[T]{factor: factor}, nil
}

// OutputLen returns how many samples Process will write for that many inputs,
// which is what dst has to have room for. It depends on how far into the
// current group the filter is, so ask it rather than dividing by the factor.
func (f *BoxcarDecimator[T]) OutputLen(inputs int) int {
	return (f.prevIndex + inputs) / f.factor
}

// Process writes the decimated output and returns how many samples it wrote,
// which is OutputLen(len(src)). It panics if dst is shorter than that. dst may
// be the same slice as src: the output index never runs ahead of the input
// index, so filtering in place overwrites only samples already read. Samples
// left over from a group that is not yet complete are carried into the next
// call.
func (f *BoxcarDecimator[T]) Process(dst, src []T) int {
	checkDst("BoxcarDecimator", len(dst), f.OutputLen(len(src)))
	if c, ok := any(f).(*BoxcarDecimator[complex64]); ok {
		var n int
		n, c.now, c.prevIndex = f32.BoxcarDecimate(any(dst).([]complex64), any(src).([]complex64), c.factor, c.now, c.prevIndex)
		return n
	}
	return boxcarDecimateGeneric(f, dst, src)
}

// Ratio returns the input rate to the output rate, (factor, 1).
func (f *BoxcarDecimator[T]) Ratio() (in, out int) { return f.factor, 1 }

// Reset drops the partial group and the accumulator, so the filter starts a new
// stream from silence and in phase.
func (f *BoxcarDecimator[T]) Reset() {
	f.now = 0
	f.prevIndex = 0
}

func boxcarDecimateGeneric[T Sample](f *BoxcarDecimator[T], dst, src []T) int {
	n := 0
	for _, v := range src {
		f.now += v
		f.prevIndex++
		if f.prevIndex < f.factor {
			continue
		}
		dst[n] = f.now
		n++
		f.prevIndex = 0
		f.now = 0
	}
	return n
}

// RationalBoxcarDecimator decimates by the ratio fast/slow, where fast is the
// input rate and slow the output rate. Like BoxcarDecimator it sums the
// samples that fall in each output period, but it divides each sum by the
// number that went into it, so the filter has unity gain.
//
// float32 runs a kernel with VFP assembly on 32-bit arm; the other widths run
// the same loop in Go.
type RationalBoxcarDecimator[T Sample] struct {
	fast, slow int

	sum       T
	count     int
	prevIndex int
}

// NewRationalBoxcarDecimator returns a decimator from the input rate fast to
// the output rate slow. Both must be positive, and slow may not exceed fast:
// this filter only decimates. The sample type cannot be inferred and has to be
// given, as in NewRationalBoxcarDecimator[float32](48000, 11025).
func NewRationalBoxcarDecimator[T Sample](fast, slow int) (*RationalBoxcarDecimator[T], error) {
	if fast <= 0 || slow <= 0 {
		return nil, errors.New("dsp: rational boxcar decimator rates must be positive")
	}
	if slow > fast {
		return nil, errors.New("dsp: rational boxcar decimator output rate cannot exceed the input rate")
	}
	return &RationalBoxcarDecimator[T]{fast: fast, slow: slow}, nil
}

// OutputLen returns how many samples Process will write for that many inputs,
// which is what dst has to have room for. The phase advances by slow per input
// and emits every time it reaches fast, so this is not len(src)*slow/fast.
func (f *RationalBoxcarDecimator[T]) OutputLen(inputs int) int {
	// The product is formed in int64 because it overflows a 32-bit int at reasonable values.
	return int((int64(f.prevIndex) + int64(inputs)*int64(f.slow)) / int64(f.fast))
}

// Process writes the decimated output and returns how many samples it wrote,
// which is OutputLen(len(src)). It panics if dst is shorter than that. dst may
// be the same slice as src, for the reason BoxcarDecimator.Process gives. The
// accumulator and the phase carry across calls.
func (f *RationalBoxcarDecimator[T]) Process(dst, src []T) int {
	checkDst("RationalBoxcarDecimator", len(dst), f.OutputLen(len(src)))
	if r, ok := any(f).(*RationalBoxcarDecimator[float32]); ok {
		var n int
		n, r.sum, r.count, r.prevIndex = f32.RationalBoxcarDecimate(any(dst).([]float32), any(src).([]float32),
			r.fast, r.slow, r.sum, r.count, r.prevIndex)
		return n
	}
	return rationalBoxcarDecimateGeneric(f, dst, src)
}

// Ratio returns the input rate to the output rate, (fast, slow) as the
// constructor was given them and not reduced.
func (f *RationalBoxcarDecimator[T]) Ratio() (in, out int) {
	return f.fast, f.slow
}

// Reset drops the partial output and the phase, so the filter starts a new
// stream from silence and in phase.
func (f *RationalBoxcarDecimator[T]) Reset() {
	f.sum = 0
	f.count = 0
	f.prevIndex = 0
}

func rationalBoxcarDecimateGeneric[T Sample](f *RationalBoxcarDecimator[T], dst, src []T) int {
	n := 0
	for _, v := range src {
		f.sum += v
		f.count++
		f.prevIndex += f.slow
		if f.prevIndex < f.fast {
			continue
		}
		// Divide by the count this output actually accumulated, not by the
		// nominal fast/slow. For a non-integer ratio the count alternates
		// between the floor and the ceiling of it, and a fixed divisor turns
		// that alternation into a spurious tone at the beat frequency.
		dst[n] = f.sum / realSample[T](float64(f.count))
		n++
		f.prevIndex -= f.fast
		f.sum = 0
		f.count = 0
	}
	return n
}
