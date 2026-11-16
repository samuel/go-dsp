package dsp

import (
	"errors"
	"math"
)

const (
	resamplerBaseTaps = 16
	resamplerMaxTaps  = 512
	resamplerPhases   = 256
	resamplerCutoff   = 0.45 // of the lower Nyquist rate, in cycles per sample
	resamplerBeta     = 7    // Kaiser beta, about 70 dB of stopband
)

// Resampler changes the sample rate by any ratio, output samples per input sample, with a
// polyphase windowed-sinc filter. Its anti-aliasing cutoff is set by the ratio it is built
// with: below the output's Nyquist rate when it lowers the rate, and the input's when it raises
// it. SetRatio adjusts the ratio afterwards without a glitch, which is how a consumer follows
// the drift between two clocks; a large change should build a new Resampler, whose filter fits
// the new ratio.
//
// The coefficients are at the sample's component width, float32 for float32 and complex64, so
// the filter runs the Dot kernels at every width.
//
// The output is delayed by about half the filter: the first output is the first input, read
// with the filter centered on it and the samples before it taken as silence. Not safe for
// concurrent use.
type Resampler[T Sample] struct {
	// The interpolator at the sample's component width; the other is nil.
	interp32 *SincInterpolator[float32]
	interp64 *SincInterpolator[float64]
	ratio    float64
	step     float64 // input samples per output sample, 1/ratio
	taps     int     // points each output reads
	half     int     // taps/2-1: points read before the position
	// hist holds the input still needed: the points before the position that the next output
	// reads, and what follows.
	hist []T
	// The next output is at hist[ipos] plus frac, in [0, 1). Keeping the whole part separate
	// makes dropping consumed input exact, so the output does not depend on block sizes.
	ipos int
	frac float64
}

// NewResampler returns a resampler that writes ratio outputs per input, which must be
// positive and finite. The type of T cannot be inferred and has to be given, as in
// NewResampler[float32](48000.0 / 44100).
func NewResampler[T Sample](ratio float64) (*Resampler[T], error) {
	if !(ratio > 0) || math.IsInf(ratio, 0) {
		return nil, errors.New("dsp: resampler ratio must be positive and finite")
	}
	// Lowering the rate needs a longer filter to keep the same transition in output terms.
	taps := resamplerBaseTaps
	if ratio < 1 {
		taps = min(2*int(math.Ceil(resamplerBaseTaps/ratio/2)), resamplerMaxTaps)
	}
	cutoff := resamplerCutoff * min(1, ratio)
	r := &Resampler[T]{taps: taps, half: taps/2 - 1}
	var z T
	var err error
	switch any(z).(type) {
	case float32, complex64:
		r.interp32, err = NewSincInterpolator[float32](taps, resamplerPhases, cutoff, resamplerBeta)
	default:
		r.interp64, err = NewSincInterpolator[float64](taps, resamplerPhases, cutoff, resamplerBeta)
	}
	if err != nil {
		return nil, err
	}
	r.ratio, r.step = ratio, 1/ratio
	r.Reset()
	return r, nil
}

// Ratio returns the output samples per input sample, as last set.
func (r *Resampler[T]) Ratio() float64 { return r.ratio }

// SetRatio changes the ratio from the next output on, keeping the filter and the stream
// position. ratio must be positive and finite.
func (r *Resampler[T]) SetRatio(ratio float64) error {
	if !(ratio > 0) || math.IsInf(ratio, 0) {
		return errors.New("dsp: resampler ratio must be positive and finite")
	}
	r.ratio, r.step = ratio, 1/ratio
	return nil
}

// OutputLen returns how many samples Process will write for that many inputs. It depends on
// the position carried from earlier calls, so it is not inputs*Ratio.
func (r *Resampler[T]) OutputLen(inputs int) int {
	n := 0
	limit := len(r.hist) + inputs - r.taps/2
	for ipos, frac := r.ipos, r.frac; ipos < limit; ipos, frac = advance(ipos, frac, r.step) {
		n++
	}
	return n
}

// advance moves a position split into whole and fractional parts by step.
func advance(ipos int, frac, step float64) (int, float64) {
	frac += step
	whole := math.Floor(frac)
	return ipos + int(whole), frac - whole
}

// Process writes the resampled output and returns how many samples it wrote, which is
// OutputLen(len(src)). It panics if dst is shorter than that. dst may share memory with src,
// since src is copied before any output is written.
func (r *Resampler[T]) Process(dst, src []T) int {
	n := r.OutputLen(len(src))
	checkDst("Resampler", len(dst), n)
	r.hist = append(r.hist, src...)
	if r.interp32 != nil {
		resample(r, r.interp32, dst[:n])
	} else {
		resample(r, r.interp64, dst[:n])
	}
	// Drop the points no later output reads.
	if drop := min(r.ipos-r.half, len(r.hist)); drop > 0 {
		r.hist = append(r.hist[:0], r.hist[drop:]...)
		r.ipos -= drop
	}
	return n
}

// resample fills dst from the history, advancing the position by one step per output.
func resample[T Sample, F Float](r *Resampler[T], interp *SincInterpolator[F], dst []T) {
	for k := range dst {
		first := r.ipos - r.half
		dst[k] = interp.Interpolate(r.hist[first:first+r.taps], r.frac)
		r.ipos, r.frac = advance(r.ipos, r.frac, r.step)
	}
}

// Reset returns the resampler to the start of a stream: silence before the next input, which
// the first output is read at.
func (r *Resampler[T]) Reset() {
	r.hist = append(r.hist[:0], make([]T, r.half)...)
	r.ipos, r.frac = r.half, 0
}
