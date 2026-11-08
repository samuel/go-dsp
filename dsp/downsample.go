package dsp

import "errors"

// BoxcarDecimator decimates by a whole-number factor, summing each group of
// that many consecutive samples into one output. That boxcar average is a
// cheap low-pass, so it also serves as the anti-alias filter. The sum is not
// divided by the factor, so the filter has a gain of the factor.
//
// complex64 runs the assembly; the other widths run the same loop in Go.
type BoxcarDecimator[T Sample] struct {
	// The assembly reads BoxcarDecimator[complex64] at fixed offsets; see
	// downsample_arm.go before changing these fields.
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

// FilterInPlace decimates samples in place and returns the prefix of the same
// slice holding the output, so the caller must not go on using what it passed
// in. Samples left over from a group that is not yet complete are carried into
// the next call, so a stream can be filtered in blocks.
func (f *BoxcarDecimator[T]) FilterInPlace(dst []T) []T {
	return dst[:f.Filter(dst, dst)]
}

// OutputLen returns how many samples Filter will write for that many inputs,
// which is what dst has to have room for. It depends on how far into the
// current group the filter is, so ask it rather than dividing by the factor.
func (f *BoxcarDecimator[T]) OutputLen(inputs int) int {
	return (f.prevIndex + inputs) / f.factor
}

// Filter writes the decimated output to a separate slice and returns how many
// samples it wrote, which is OutputLen(len(src)). dst must have room for that
// many, and may be the same slice as src: the output index never runs ahead of
// the input index, so filtering in place overwrites only samples already read.
func (f *BoxcarDecimator[T]) Filter(dst, src []T) int {
	if c, ok := any(f).(*BoxcarDecimator[complex64]); ok {
		return boxcarDecimateAsm(c, any(dst).([]complex64), any(src).([]complex64))
	}
	return boxcarDecimateGeneric(f, dst, src)
}

// Factor returns the decimation factor, so a caller measuring the filter can
// work out the output rate.
func (f *BoxcarDecimator[T]) Factor() int { return f.factor }

// Reset drops the partial group and the accumulator, so the filter starts a new
// stream from silence and in phase.
func (f *BoxcarDecimator[T]) Reset() {
	f.now = 0
	f.prevIndex = 0
}

// boxcarDecimate is the Go reference behind the complex64 assembly, and the
// fallback the assembly stubs jump to.
func boxcarDecimate(f *BoxcarDecimator[complex64], dst, src []complex64) int {
	return boxcarDecimateGeneric(f, dst, src)
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
// float32 runs the assembly; the other widths run the same loop in Go.
type RationalBoxcarDecimator[T Sample] struct {
	// The assembly reads RationalBoxcarDecimator[float32] at fixed offsets;
	// see downsample_arm.go before changing these fields.
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

// FilterInPlace decimates samples in place and returns the prefix of the same
// slice holding the output, so the caller must not go on using what it passed
// in. The accumulator and the phase carry across calls, so a stream can be
// filtered in blocks.
func (f *RationalBoxcarDecimator[T]) FilterInPlace(dst []T) []T {
	return dst[:f.Filter(dst, dst)]
}

// OutputLen returns how many samples Filter will write for that many inputs,
// which is what dst has to have room for. The phase advances by slow per input
// and emits every time it reaches fast, so this is not len(src)*slow/fast.
func (f *RationalBoxcarDecimator[T]) OutputLen(inputs int) int {
	// The product is formed in int64 because it overflows a 32-bit int at reasonable values.
	return int((int64(f.prevIndex) + int64(inputs)*int64(f.slow)) / int64(f.fast))
}

// Filter writes the decimated output to a separate slice and returns how many
// samples it wrote, which is OutputLen(len(src)). dst must have room for that
// many, and may be the same slice as src, for the reason BoxcarDecimator.Filter
// gives.
func (f *RationalBoxcarDecimator[T]) Filter(dst, src []T) int {
	if r, ok := any(f).(*RationalBoxcarDecimator[float32]); ok {
		return rationalBoxcarDecimateAsm(r, any(dst).([]float32), any(src).([]float32))
	}
	return rationalBoxcarDecimateGeneric(f, dst, src)
}

// Rates returns the input and output rates the filter was built with. They are
// whatever units the constructor was given; only their ratio matters.
func (f *RationalBoxcarDecimator[T]) Rates() (fast, slow int) {
	return f.fast, f.slow
}

// Reset drops the partial output and the phase, so the filter starts a new
// stream from silence and in phase.
func (f *RationalBoxcarDecimator[T]) Reset() {
	f.sum = 0
	f.count = 0
	f.prevIndex = 0
}

// rationalBoxcarDecimate is the Go reference behind the float32 assembly, and
// the fallback the assembly stubs jump to. See boxcarDecimate.
func rationalBoxcarDecimate(f *RationalBoxcarDecimator[float32], dst, src []float32) int {
	return rationalBoxcarDecimateGeneric(f, dst, src)
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
