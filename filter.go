package dsp

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"samuelks.com/dsp/internal/view"
)

// IIRFilter is a direct form 1 IIR filter of arbitrary order, over real or
// complex samples at either width. It keeps the delay lines between calls so a
// stream can be filtered in blocks.
type IIRFilter[T Sample] struct {
	b, a      coefs[T]
	pIn, pOut []T
}

// NewIIRFilter returns an IIR filter with the given feedforward (b) and
// feedback (a) coefficients, both normalized by aCoef[0]. It returns an error
// unless the two are the same length and hold at least two coefficients each:
// with one, there is no delay line and y[n] = (b[0]/a[0])*x[n], which is a
// gain rather than a filter.
func NewIIRFilter[T Sample](bCoef, aCoef []T) (*IIRFilter[T], error) {
	b, a, err := normalizeIIR(slices.Clone(bCoef), slices.Clone(aCoef))
	if err != nil {
		return nil, err
	}
	return &IIRFilter[T]{
		b:    coefs[T]{native: b},
		a:    coefs[T]{native: a},
		pIn:  make([]T, len(b)-1),
		pOut: make([]T, len(b)-1),
	}, nil
}

// NewRealCoefIIRFilter returns an IIR filter over complex samples built from
// real coefficients, which is the usual case: a filter with a real impulse
// response has real coefficients whether or not the signal it filters does.
// The complex sample type has to be given explicitly, as in
// NewRealCoefIIRFilter[complex64](b, a). The coefficients are converted to the
// sample's component width and then normalized, and the recursion runs as two
// real channels.
func NewRealCoefIIRFilter[C Complex, F Float](bCoef, aCoef []F) (*IIRFilter[C], error) {
	f := &IIRFilter[C]{}
	var n int
	switch any(*new(C)).(type) {
	case complex64:
		b, a, err := normalizeIIR(convertFloats[float32](bCoef), convertFloats[float32](aCoef))
		if err != nil {
			return nil, err
		}
		f.b.real, f.a.real, n = b, a, len(b)
	case complex128:
		b, a, err := normalizeIIR(convertFloats[float64](bCoef), convertFloats[float64](aCoef))
		if err != nil {
			return nil, err
		}
		f.b.real, f.a.real, n = b, a, len(b)
	}
	f.pIn = make([]C, n-1)
	f.pOut = make([]C, n-1)
	return f, nil
}

// normalizeIIR validates b and a and divides both by a[0] in place.
func normalizeIIR[E Sample](b, a []E) ([]E, []E, error) {
	if len(b) != len(a) {
		return nil, nil, errors.New("dsp: IIR filter needs len(b) == len(a)")
	}
	if len(b) < 2 {
		return nil, nil, errors.New("dsp: IIR filter needs at least two coefficients; one pair is a gain, not a filter")
	}
	if a[0] == 0 {
		return nil, nil, errors.New("dsp: IIR filter needs a non-zero a[0]; every coefficient is normalized by it")
	}
	a0 := a[0]
	for i := range b {
		b[i] /= a0
	}
	for i := range a[1:] {
		a[i+1] /= a0
	}
	// Exactly 1, so the recursion can leave it out. Dividing a[0] by itself is
	// not guaranteed to give that for a complex coefficient.
	a[0] = 1
	return b, a, nil
}

// OutputLen returns inputs: the filter writes one sample per input.
func (f *IIRFilter[T]) OutputLen(inputs int) int { return inputs }

// Process writes one output sample per input sample and returns len(src). It
// panics if dst is shorter than src. The slices may be the same.
func (f *IIRFilter[T]) Process(dst, src []T) int {
	checkDst("IIRFilter", len(dst), len(src))
	switch b := f.b.real.(type) {
	case []float32:
		iirReal(view.C64ToF32(any(dst[:len(src)]).([]complex64)), view.C64ToF32(any(src).([]complex64)),
			b, f.a.real.([]float32), view.C64ToF32(any(f.pIn).([]complex64)), view.C64ToF32(any(f.pOut).([]complex64)))
		return len(src)
	case []float64:
		iirReal(view.C128ToF64(any(dst[:len(src)]).([]complex128)), view.C128ToF64(any(src).([]complex128)),
			b, f.a.real.([]float64), view.C128ToF64(any(f.pIn).([]complex128)), view.C128ToF64(any(f.pOut).([]complex128)))
		return len(src)
	}
	for i, s := range src {
		dst[i] = f.step(s)
	}
	return len(src)
}

// ProcessOne filters a single sample. It runs the same arithmetic in the same
// order as Process, so filtering a block and filtering its samples one at a
// time give bit-identical streams.
func (f *IIRFilter[T]) ProcessOne(s T) T {
	if f.b.native != nil {
		return f.step(s)
	}
	buf := [1]T{s}
	f.Process(buf[:], buf[:])
	return buf[0]
}

// step advances the recursion with native coefficients by one sample and
// returns the output.
func (f *IIRFilter[T]) step(s T) T {
	b, a := f.b.native, f.a.native
	sum := b[0] * s
	for j, p := range f.pIn {
		sum += b[j+1]*p - a[j+1]*f.pOut[j]
	}
	for i := len(f.pIn) - 1; i > 0; i-- {
		f.pIn[i] = f.pIn[i-1]
		f.pOut[i] = f.pOut[i-1]
	}
	f.pIn[0] = s
	f.pOut[0] = sum
	return sum
}

// Coefficients returns copies of the feedforward and feedback coefficients at
// the sample type, as the constructor normalized them, so a[0] is exactly 1.
func (f *IIRFilter[T]) Coefficients() (b, a []T) {
	return f.b.widen(), f.a.widen()
}

// RealCoefficients returns copies of the normalized coefficients in float64,
// the b and a Freqz takes. ok is false, and both nil, when any coefficient has
// a non-zero imaginary part.
func (f *IIRFilter[T]) RealCoefficients() (b, a []float64, ok bool) {
	b, okB := f.b.float64s()
	a, okA := f.a.float64s()
	if !okB || !okA {
		return nil, nil, false
	}
	return b, a, true
}

// Reset clears the delay lines, so the filter starts a new stream from silence.
func (f *IIRFilter[T]) Reset() {
	clear(f.pIn)
	clear(f.pOut)
}

// DCFilter is a one-pole DC blocker: a high-pass with its zero at DC and a
// real pole just below 1, which removes a constant offset. It takes real or
// complex samples; on complex IQ it removes the spike at the center of the
// spectrum, such as an RTL-SDR's, filtering the real and imaginary parts
// independently. State is kept between calls.
//
// It runs in direct form 1, y[n] = x[n] - x[n-1] + a·y[n-1], so the state is
// the previous input and output and does not depend on the pole: once the
// output has settled, SetCutoff moves the pole without a transient.
//
// The pole and the state are float64 whatever the sample type is, as
// Biquad's are and for the same reason: the recursion's pole sits just inside
// the unit circle, exactly where a narrower accumulator shows up as drift. The sample type therefore has to be given, as in
// NewDCFilter[complex64](0.0002).
type DCFilter[T Sample] struct {
	cutoff float64
	a      float64
	// The previous input and output: index 0 for a real sample or a complex
	// sample's real part, 1 for its imaginary part.
	x, y [2]float64
}

// MaxDCFilterCutoff is the highest cutoff a DCFilter accepts, acos(3/4)/2π
// cycles per sample, where the pole reaches 0. Above it the pole would go
// negative and the filter would stop being a DC blocker.
const MaxDCFilterCutoff = 0.11502672808130794

// NewDCFilter returns a DC blocker whose -3 dB point is at cutoff, in cycles
// per sample, which must be in (0, MaxDCFilterCutoff]. A cutoff in Hz is
// cutoffHz/sampleRate.
func NewDCFilter[T Sample](cutoff float64) (*DCFilter[T], error) {
	f := &DCFilter[T]{}
	if err := f.SetCutoff(cutoff); err != nil {
		return nil, err
	}
	return f, nil
}

// SetCutoff moves the -3 dB point to cutoff, in cycles per sample, and keeps
// the state, so a running stream sees no reset and, once settled, no
// transient. An invalid cutoff returns an
// error and leaves the filter as it was.
func (f *DCFilter[T]) SetCutoff(cutoff float64) error {
	if !(cutoff > 0 && cutoff <= MaxDCFilterCutoff) {
		return fmt.Errorf("dsp: DC blocker cutoff %v must be in (0, %.4f] cycles per sample", cutoff, MaxDCFilterCutoff)
	}
	f.cutoff = cutoff
	f.a = dcPole(cutoff)
	return nil
}

// dcPole returns the pole that puts the blocker's -3 dB point at cutoff.
// Setting |H|² = 1/2 for H(z) = (1 - z⁻¹)/(1 - a·z⁻¹) at ω = 2π·cutoff gives
// a² - 2a·cos ω + 4cos ω - 3 = 0, whose root inside the unit circle is
// cos ω - √((1 - cos ω)(3 - cos ω)). For small ω it is about 1 - ω. 1 - cos ω
// is computed as 2sin²(ω/2), which does not cancel at the small cutoffs a DC
// blocker usually has.
func dcPole(cutoff float64) float64 {
	h := math.Sin(math.Pi * cutoff)
	s := 2 * h * h
	return max(1-s-math.Sqrt(s*(2+s)), 0)
}

// Cutoff returns the -3 dB point in cycles per sample, as last given.
func (f *DCFilter[T]) Cutoff() float64 { return f.cutoff }

// Pole returns the pole the cutoff gives.
func (f *DCFilter[T]) Pole() float64 { return f.a }

// OutputLen returns inputs: the filter writes one sample per input.
func (f *DCFilter[T]) OutputLen(inputs int) int { return inputs }

// Process writes one output sample per input sample and returns len(src). It
// panics if dst is shorter than src. The slices may be the same.
func (f *DCFilter[T]) Process(dst, src []T) int {
	checkDst("DCFilter", len(dst), len(src))
	dst = dst[:len(src)]
	switch d := any(dst).(type) {
	case []float32:
		dcReal(d, any(src).([]float32), f.a, &f.x[0], &f.y[0])
	case []float64:
		dcReal(d, any(src).([]float64), f.a, &f.x[0], &f.y[0])
	case []complex64:
		dcInterleaved(view.C64ToF32(d), view.C64ToF32(any(src).([]complex64)), f.a, &f.x, &f.y)
	case []complex128:
		dcInterleaved(view.C128ToF64(d), view.C128ToF64(any(src).([]complex128)), f.a, &f.x, &f.y)
	default:
		panic("dsp: DCFilter: no case for this width")
	}
	return len(src)
}

// dcReal runs the blocker over real samples, updating the previous input px
// and output py.
func dcReal[F Float](dst, src []F, a float64, px, py *float64) {
	xp, yp := *px, *py
	for i, v := range src {
		x := float64(v)
		yp = x - xp + a*yp
		xp = x
		dst[i] = F(yp)
	}
	*px, *py = xp, yp
}

// dcInterleaved runs the blocker over interleaved real and imaginary parts,
// each with its own previous input and output.
func dcInterleaved[F Float](dst, src []F, a float64, px, py *[2]float64) {
	dst = dst[:len(src)]
	xr, xi, yr, yi := px[0], px[1], py[0], py[1]
	for i := 0; i+1 < len(src); i += 2 {
		r, m := float64(src[i]), float64(src[i+1])
		yr = r - xr + a*yr
		yi = m - xi + a*yi
		xr, xi = r, m
		dst[i] = F(yr)
		dst[i+1] = F(yi)
	}
	*px, *py = [2]float64{xr, xi}, [2]float64{yr, yi}
}

// ProcessOne filters a single sample with the same arithmetic as Process, so
// the two give bit-identical streams.
func (f *DCFilter[T]) ProcessOne(x T) T {
	buf := [1]T{x}
	f.Process(buf[:], buf[:])
	return buf[0]
}

// RealCoefficients returns the blocker written as an ordinary IIR filter: b is
// {1, -1} for the zero at DC and a is {1, -pole}, so Freqz and the analysis
// tools can treat it like any other filter. ok is always true.
func (f *DCFilter[T]) RealCoefficients() (b, a []float64, ok bool) {
	return []float64{1, -1}, []float64{1, -f.a}, true
}

// Reset clears the filter state, so it starts a new stream from silence.
func (f *DCFilter[T]) Reset() {
	f.x, f.y = [2]float64{}, [2]float64{}
}
