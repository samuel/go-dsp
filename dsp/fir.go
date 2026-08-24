package dsp

import (
	"errors"
	"slices"
)

// FIRFilter convolves a signal with a fixed set of taps: the finite impulse
// response filter that firpm designs and that a windowed sinc produces. It
// keeps the samples that straddle a block boundary between calls, so a stream
// can be filtered in blocks.
//
// The taps are the b coefficients of a transfer function whose a is 1, which is
// what Coefficients returns. Unlike IIRFilter there is no
// feedback, which is why the response is finite and the phase can be exactly
// linear: a symmetric tap set delays every frequency by the same (len(taps)-1)/2
// samples.
type FIRFilter[T Sample] struct {
	taps []T
	// hist holds the len(taps)-1 samples before the next block, so that its
	// first outputs see the same history a continuous stream would.
	hist []T
	// work is hist followed by the block being filtered, so the inner loop
	// walks one contiguous run with no wraparound test per tap. A circular
	// delay line saves this copy and costs an index fixup on every multiply,
	// which is the wrong trade once the loop is vectorized.
	work []T
}

// NewFIRFilter returns an FIR filter with the given taps, which are copied. It
// returns an error if there are none: a filter with no taps outputs silence,
// which is never what a caller meant.
func NewFIRFilter[T Sample](taps []T) (*FIRFilter[T], error) {
	if len(taps) == 0 {
		return nil, errors.New("dsp: FIR filter needs at least one tap")
	}
	return &FIRFilter[T]{
		taps: slices.Clone(taps),
		hist: make([]T, len(taps)-1),
	}, nil
}

// NewRealCoefFIRFilter returns an FIR filter over complex samples built from
// real taps, which is the usual case for radio: a filter with a real impulse
// response has real taps whether or not the signal it filters does. The complex
// sample type has to be given explicitly, as in
// NewRealCoefFIRFilter[complex64](taps).
func NewRealCoefFIRFilter[C Complex, F Float](taps []F) (*FIRFilter[C], error) {
	// Widening is exact and changes no length, so NewFIRFilter's checks say
	// everything there is to say about these taps too.
	return NewFIRFilter(rtoc[C](taps))
}

// Filter writes one output sample per input sample, over
// min(len(dst), len(src)) of them. The slices may be the same. Samples past
// that count are not consumed, so the history matches what was written.
func (f *FIRFilter[T]) Filter(dst, src []T) {
	src = src[:min(len(dst), len(src))]
	if len(src) == 0 {
		return
	}
	w := f.stage(src)
	h := len(f.hist)
	for i := range src {
		dst[i] = f.dot(w, h+i)
	}
	f.save(w, len(src))
}

// FilterOne filters a single sample. Filter stages a whole block at once, so
// prefer it when there is a block to filter.
func (f *FIRFilter[T]) FilterOne(x T) T {
	var buf [1]T
	buf[0] = x
	f.Filter(buf[:], buf[:])
	return buf[0]
}

// Coefficients returns the taps as the b of a transfer function, and a nil a:
// an FIR filter has no feedback, so its denominator is 1. They are a copy.
// Freqz takes float64 slices, so a filter at another width has to be converted
// first, and it reads a nil a as the same 1.
func (f *FIRFilter[T]) Coefficients() (b, a []T) {
	return slices.Clone(f.taps), nil
}

// Len returns the number of taps.
func (f *FIRFilter[T]) Len() int { return len(f.taps) }

// Reset clears the history, so the filter starts a new stream from silence.
func (f *FIRFilter[T]) Reset() { clear(f.hist) }

// stage copies the history and src into one contiguous buffer and returns it.
func (f *FIRFilter[T]) stage(src []T) []T {
	n := len(f.hist) + len(src)
	if cap(f.work) < n {
		f.work = make([]T, n)
	}
	w := f.work[:n]
	copy(w, f.hist)
	copy(w[len(f.hist):], src)
	return w
}

// dot evaluates the filter at w[at], which must have at least len(taps)-1
// samples of history in front of it.
func (f *FIRFilter[T]) dot(w []T, at int) T {
	var sum T
	for j, c := range f.taps {
		sum += c * w[at-j]
	}
	return sum
}

// save keeps the last len(hist) samples of the staged buffer for the next call.
func (f *FIRFilter[T]) save(w []T, consumed int) {
	copy(f.hist, w[consumed:])
}

// FIRDecimator is an FIR filter that keeps only every factor-th output. It is
// the filter and the rate change in one pass: the outputs that are about to
// be thrown away are never computed, so the work is a factor smaller than
// filtering and then discarding.
//
// It does not implement Coefficients, and cannot. A rate changer is
// periodically time-varying rather than time-invariant, so it has no single
// transfer function; measure the FIRFilter with the same taps instead.
type FIRDecimator[T Sample] struct {
	f      FIRFilter[T]
	factor int
	// phase is the index of the next output within the next block, which is
	// what carries the decimation grid across a block boundary.
	phase int
}

// NewFIRDecimator returns a decimator that filters with the given taps and
// keeps every factor-th output. factor must be at least 1; at 1 it is an
// ordinary FIR filter.
func NewFIRDecimator[T Sample](taps []T, factor int) (*FIRDecimator[T], error) {
	if factor < 1 {
		return nil, errors.New("dsp: FIR decimator needs a factor of at least 1")
	}
	f, err := NewFIRFilter(taps)
	if err != nil {
		return nil, err
	}
	return &FIRDecimator[T]{f: *f, factor: factor}, nil
}

// NewRealCoefFIRDecimator is NewFIRDecimator with real taps over complex
// samples; see NewRealCoefFIRFilter.
func NewRealCoefFIRDecimator[C Complex, F Float](taps []F, factor int) (*FIRDecimator[C], error) {
	// See NewRealCoefFIRFilter.
	return NewFIRDecimator(rtoc[C](taps), factor)
}

// Factor returns the decimation factor, so a caller measuring the filter can
// work out the output rate without being told it twice.
func (d *FIRDecimator[T]) Factor() int { return d.factor }

// OutputLen returns how many samples Filter will write for that many inputs,
// which is what dst has to have room for. It depends on the phase the filter is
// currently at, so ask it rather than dividing by the factor.
func (d *FIRDecimator[T]) OutputLen(inputs int) int {
	if inputs <= d.phase {
		return 0
	}
	return (inputs - d.phase + d.factor - 1) / d.factor
}

// Filter writes the decimated output and returns how many samples it wrote,
// which is OutputLen(len(src)). dst must have room for that many, and may be
// the same slice as src.
func (d *FIRDecimator[T]) Filter(dst, src []T) int {
	if len(src) == 0 {
		return 0
	}
	w := d.f.stage(src)
	h := len(d.f.hist)
	out := 0
	for i := d.phase; i < len(src); i += d.factor {
		dst[out] = d.f.dot(w, h+i)
		out++
	}
	d.advance(len(src))
	d.f.save(w, len(src))
	return out
}

// FilterInPlace decimates samples in place and returns the prefix of the same
// slice holding the output, so the caller must not go on using what it passed
// in. Samples that do not complete an output period are carried into the next
// call, so a stream can be filtered in blocks.
func (d *FIRDecimator[T]) FilterInPlace(dst []T) []T {
	return dst[:d.Filter(dst, dst)]
}

// Reset drops the history and the phase, so the filter starts a new stream from
// silence and in phase.
func (d *FIRDecimator[T]) Reset() {
	d.f.Reset()
	d.phase = 0
}

// advance moves the decimation grid past a block of n inputs.
func (d *FIRDecimator[T]) advance(n int) {
	if d.phase >= n {
		d.phase -= n
		return
	}
	k := (n - d.phase + d.factor - 1) / d.factor
	d.phase += k*d.factor - n
}
