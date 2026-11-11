package dsp

import (
	"errors"
	"math"
	"slices"
)

// FIRFilter convolves a signal with a fixed set of taps: the finite impulse
// response filter that firpm designs and that a windowed sinc produces. It
// keeps the samples that straddle a block boundary between calls, so a stream
// can be filtered in blocks.
//
// The taps are the b coefficients of a transfer function whose a is 1, which is
// what RealCoefficients returns. Unlike IIRFilter there is no
// feedback, which is why the response is finite and the phase can be exactly
// linear: a symmetric tap set delays every frequency by the same (len(taps)-1)/2
// samples.
type FIRFilter[T Sample] struct {
	taps coefs[T]
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
		taps: coefs[T]{native: slices.Clone(taps)},
		hist: make([]T, len(taps)-1),
	}, nil
}

// NewRealCoefFIRFilter returns an FIR filter over complex samples built from
// real taps, which is the usual case for radio: a filter with a real impulse
// response has real taps whether or not the signal it filters does. The complex
// sample type has to be given explicitly, as in
// NewRealCoefFIRFilter[complex64](taps). The taps are stored at the sample's
// component width, and each output costs two real multiplies per tap rather
// than a complex one.
func NewRealCoefFIRFilter[C Complex, F Float](taps []F) (*FIRFilter[C], error) {
	if len(taps) == 0 {
		return nil, errors.New("dsp: FIR filter needs at least one tap")
	}
	return &FIRFilter[C]{
		taps: realCoefs[C](taps),
		hist: make([]C, len(taps)-1),
	}, nil
}

// OutputLen returns inputs: the filter writes one sample per input.
func (f *FIRFilter[T]) OutputLen(inputs int) int { return inputs }

// Process writes one output sample per input sample and returns len(src). It
// panics if dst is shorter than src. The slices may be the same.
func (f *FIRFilter[T]) Process(dst, src []T) int {
	checkDst("FIRFilter", len(dst), len(src))
	if len(src) == 0 {
		return 0
	}
	w := f.stage(src)
	f.taps.fir(dst, 1, w, len(f.hist), 1, len(src))
	f.save(w, len(src))
	return len(src)
}

// ProcessOne filters a single sample. Process stages a whole block at once, so
// prefer it when there is a block to filter.
func (f *FIRFilter[T]) ProcessOne(x T) T {
	var buf [1]T
	buf[0] = x
	f.Process(buf[:], buf[:])
	return buf[0]
}

// Taps returns a copy of the taps at the sample type.
func (f *FIRFilter[T]) Taps() []T { return f.taps.widen() }

// RealCoefficients returns the taps in float64 as the b of a transfer function,
// and a nil a: an FIR filter has no feedback, so its denominator is 1, and
// Freqz reads a nil a as that 1. ok is false, and b nil, when any tap has a
// non-zero imaginary part.
func (f *FIRFilter[T]) RealCoefficients() (b, a []float64, ok bool) {
	b, ok = f.taps.float64s()
	return b, nil, ok
}

// Len returns the number of taps.
func (f *FIRFilter[T]) Len() int { return f.taps.len() }

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

// save keeps the last len(hist) samples of the staged buffer for the next call.
func (f *FIRFilter[T]) save(w []T, consumed int) {
	copy(f.hist, w[consumed:])
}

// FIRDecimator is an FIR filter that keeps only every factor-th output. It is
// the filter and the rate change in one pass: the outputs that are about to
// be thrown away are never computed, so the work is a factor smaller than
// filtering and then discarding.
//
// It does not implement RealCoefficients, and cannot. A rate changer is
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
	if factor < 1 {
		return nil, errors.New("dsp: FIR decimator needs a factor of at least 1")
	}
	f, err := NewRealCoefFIRFilter[C](taps)
	if err != nil {
		return nil, err
	}
	return &FIRDecimator[C]{f: *f, factor: factor}, nil
}

// Ratio returns the input rate to the output rate, (factor, 1).
func (d *FIRDecimator[T]) Ratio() (in, out int) { return d.factor, 1 }

// OutputLen returns how many samples Process will write for that many inputs,
// which is what dst has to have room for. It depends on the phase the filter is
// currently at, so ask it rather than dividing by the factor.
func (d *FIRDecimator[T]) OutputLen(inputs int) int {
	if inputs <= d.phase {
		return 0
	}
	return (inputs - d.phase + d.factor - 1) / d.factor
}

// Process writes the decimated output and returns how many samples it wrote,
// which is OutputLen(len(src)). It panics if dst is shorter than that. dst may
// be the same slice as src. Samples that do not complete an output period are
// carried into the next call.
func (d *FIRDecimator[T]) Process(dst, src []T) int {
	checkDst("FIRDecimator", len(dst), d.OutputLen(len(src)))
	if len(src) == 0 {
		return 0
	}
	w := d.f.stage(src)
	out := d.OutputLen(len(src))
	d.f.taps.fir(dst, 1, w, len(d.f.hist)+d.phase, d.factor, out)
	d.advance(len(src))
	d.f.save(w, len(src))
	return out
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

// FIRInterpolator raises the sample rate by a whole-number factor: it follows
// each input sample with factor-1 zeros and filters the result with taps,
// writing factor outputs per input. It is the zero-stuffing and the filter in
// one pass: output n uses only the taps that land on input samples, one
// polyphase branch of every factor-th tap, so the multiplies by the stuffed
// zeros are never done. It keeps the inputs that straddle a block boundary
// between calls, so a stream can be filtered in blocks.
//
// The taps are the filter at the output rate, normally a low-pass at
// 0.5/factor cycles per output sample. Zero-stuffing divides the signal's
// level by factor, so the taps need a DC gain of factor to keep it, as from
// LowPassFIR followed by NormalizeTaps(taps, factor). As with FIRFilter, a
// symmetric tap set delays the output by (len(taps)-1)/2 output samples.
//
// Like FIRDecimator it does not implement RealCoefficients; measure the FIRFilter
// with the same taps at the output rate instead.
type FIRInterpolator[T Sample] struct {
	// f carries the input history and staging; its taps are unused, since
	// each output takes one of branches instead.
	f FIRFilter[T]
	// branches[p] holds taps p, p+factor, p+2*factor, ..., zero-padded to a
	// common length, which is what output phase p applies to the inputs.
	branches []coefs[T]
}

// NewFIRInterpolator returns an interpolator that raises the rate by factor
// and filters with the given taps, which are copied. factor must be at least
// 1; at 1 it is an ordinary FIR filter.
func NewFIRInterpolator[T Sample](taps []T, factor int) (*FIRInterpolator[T], error) {
	switch {
	case factor < 1:
		return nil, errors.New("dsp: FIR interpolator needs a factor of at least 1")
	case len(taps) == 0:
		return nil, errors.New("dsp: FIR interpolator needs at least one tap")
	}
	return newFIRInterpolator(coefs[T]{native: taps}, factor), nil
}

func newFIRInterpolator[T Sample](taps coefs[T], factor int) *FIRInterpolator[T] {
	n := (taps.len() + factor - 1) / factor
	return &FIRInterpolator[T]{
		f:        FIRFilter[T]{hist: make([]T, n-1)},
		branches: taps.polyphase(factor),
	}
}

// NewRealCoefFIRInterpolator is NewFIRInterpolator with real taps over complex
// samples; see NewRealCoefFIRFilter.
func NewRealCoefFIRInterpolator[C Complex, F Float](taps []F, factor int) (*FIRInterpolator[C], error) {
	switch {
	case factor < 1:
		return nil, errors.New("dsp: FIR interpolator needs a factor of at least 1")
	case len(taps) == 0:
		return nil, errors.New("dsp: FIR interpolator needs at least one tap")
	}
	return newFIRInterpolator(realCoefs[C](taps), factor), nil
}

// Ratio returns the input rate to the output rate, (1, factor).
func (it *FIRInterpolator[T]) Ratio() (in, out int) { return 1, len(it.branches) }

// OutputLen returns how many samples Process will write for that many inputs,
// which is inputs*factor. It panics if that does not fit in an int, which a
// 32-bit platform reaches at realistic sizes.
func (it *FIRInterpolator[T]) OutputLen(inputs int) int {
	if inputs > math.MaxInt/len(it.branches) {
		panic("dsp: FIR interpolator output length overflows int")
	}
	return inputs * len(it.branches)
}

// Process writes the interpolated output and returns how many samples it
// wrote, which is OutputLen(len(src)). It panics if dst is shorter than that.
// dst may share memory with src, since src is copied before any output is
// written.
func (it *FIRInterpolator[T]) Process(dst, src []T) int {
	factor := len(it.branches)
	checkDst("FIRInterpolator", len(dst)/factor, len(src)) // len(src)*factor could overflow
	if len(src) == 0 {
		return 0
	}
	w := it.f.stage(src)
	for p := range it.branches {
		it.branches[p].fir(dst[p:], factor, w, len(it.f.hist), 1, len(src))
	}
	it.f.save(w, len(src))
	return len(src) * factor
}

// Reset clears the history, so the filter starts a new stream from silence.
func (it *FIRInterpolator[T]) Reset() { it.f.Reset() }

// Upsample is FIRInterpolator applied to a whole buffer at zero phase: the
// filter is centered on tap len(taps)/2 rather than delaying the output, and
// the signal is taken as zero outside src. It writes len(src)*factor samples
// to dst, which must not overlap src. It allocates a working buffer; use
// FIRInterpolator to filter a stream in blocks.
func Upsample[T Sample](dst, src []T, factor int, taps []T) error {
	if factor >= 1 && len(src) > len(dst)/factor { // len(src)*factor could overflow
		return errors.New("dsp: upsample dst is shorter than len(src)*factor")
	}
	it, err := NewFIRInterpolator(taps, factor)
	if err != nil {
		return err
	}
	// Output i is the causal output i+center, so the inputs run on with
	// enough zeros to produce the last of those.
	center := len(taps) / 2
	in := make([]T, len(src)+(center+factor-1)/factor)
	copy(in, src)
	out := make([]T, it.OutputLen(len(in)))
	it.Process(out, in)
	copy(dst, out[center:center+len(src)*factor])
	return nil
}
