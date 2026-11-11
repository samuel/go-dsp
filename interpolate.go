package dsp

import (
	"errors"
	"math"

	"samuelks.com/dsp/internal/view"
)

// The interpolators in this file all read a signal at a fractional position.
//
// x is measured in src: 0 is the first sample of src, 1 the second, and
// the fractional part selects between neighbors. Every function shares one
// out-of-range policy — a position before the first sample or after the last
// holds the nearest edge value, rather than fading to zero, which would put a
// step discontinuity at both ends of every buffer. An empty buffer, or an x
// that is NaN, gives zero.

// splitPos separates x into the index of the sample at or below it and the
// fraction between that sample and the next, reporting false for an empty
// buffer or a NaN x. Positions far outside the buffer are pinned to just
// outside it: converting an out-of-range float to int is implementation-defined
// in Go, so the infinities are kept away from the conversion.
func splitPos[T Float](n int, x T) (xi int, frac T, ok bool) {
	switch {
	case n == 0 || x != x: //nolint:gocritic // x != x is the NaN test; math.IsNaN takes a float64
		return 0, 0, false
	case x < -1:
		return -1, 0, true
	case x > T(n):
		return n, 0, true
	}
	fl := T(math.Floor(float64(x)))
	return int(fl), x - fl, true
}

// sampleAt returns src[i] with i clamped to the ends of the buffer.
func sampleAt[T Sample](src []T, i int) T {
	if i < 0 {
		i = 0
	} else if i >= len(src) {
		i = len(src) - 1
	}
	return src[i]
}

// InterpLinear interpolates between the two src either side of x.
func InterpLinear[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	lo := sampleAt(src, xi)
	hi := sampleAt(src, xi+1)
	return lo + frac*(hi-lo)
}

// InterpHermite4p3o interpolates using 4-point, 3rd-order Hermite (x-form).
func InterpHermite4p3o[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	var s [4]T
	for i := range s {
		s[i] = sampleAt(src, xi-1+i)
	}
	c0 := s[1]
	c1 := 1.0 / 2.0 * (s[2] - s[0])
	c2 := s[0] - 5.0/2.0*s[1] + 2.0*s[2] - 1.0/2.0*s[3]
	c3 := 1.0/2.0*(s[3]-s[0]) + 3.0/2.0*(s[1]-s[2])
	return ((c3*frac+c2)*frac+c1)*frac + c0
}

// InterpOptimal2x4p4o interpolates using optimal 2x (4-point, 4th-order) (z-form).
//
// The "2x" is a requirement, not a label: the coefficients minimize error over
// the lower half of the band, assuming the signal is oversampled by at least
// two. Fed a signal that uses its whole band, this is worse than the plainer
// Lagrange and Hermite interpolators above. InterpOptimal2x6p5o is the same
// assumption over six points.
func InterpOptimal2x4p4o[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	const middle = 1

	var s [4]T
	for i := range s {
		s[i] = sampleAt(src, xi-middle+i)
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	c0 := even1*0.45645918406487612 + even2*0.04354173901996461
	c1 := odd1*0.47236675362442071 + odd2*0.17686613581136501
	c2 := even1*-0.253674794204558521 + even2*0.25371918651882464
	c3 := odd1*-0.37917091811631082 + odd2*0.11952965967158000
	c4 := even1*0.04252164479749607 + even2*-0.04289144034653719

	z := frac - 1.0/2.0
	return (((c4*z+c3)*z+c2)*z+c1)*z + c0
}

// InterpOptimal2x6p5o interpolates using optimal 2x (6-point, 5th-order) (z-form).
// It assumes a 2x oversampled input, as InterpOptimal2x4p4o does; see the note there.
func InterpOptimal2x6p5o[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	const middle = 2

	var s [6]T
	for i := range s {
		s[i] = sampleAt(src, xi-middle+i)
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	even3 := s[middle+3] + s[middle-2]
	odd3 := s[middle+3] - s[middle-2]
	c0 := even1*0.40513396007145713 + even2*0.09251794438424393 + even3*0.00234806603570670
	c1 := odd1*0.28342806338906690 + odd2*0.21703277024054901 + odd3*0.01309294748731515
	c2 := even1*-0.191337682540351941 + even2*0.16187844487943592 + even3*0.02946017143111912
	c3 := odd1*-0.16471626190554542 + odd2*-0.00154547203542499 + odd3*0.03399271444851909
	c4 := even1*0.03845798729588149 + even2*-0.05712936104242644 + even3*0.01866750929921070
	c5 := odd1*0.04317950185225609 + odd2*-0.01802814255926417 + odd3*0.00152170021558204

	z := frac - 1.0/2.0
	return ((((c5*z+c4)*z+c3)*z+c2)*z+c1)*z + c0
}

// SincInterpolator is a polyphase windowed-sinc interpolator. It splits the
// interval between two samples into Phases steps, each with its own filter of
// Len taps: a sinc centered on the step, multiplied by a Kaiser window centered
// on the same point. The window's half-length is Len/2, the farthest any
// phase's taps reach, so the phases together sample one prototype filter
// whose window edges fall on its end taps, as LowPassFIR's do. The
// coefficients are computed in float64, normalized per phase to a DC gain of
// 1, and stored as F.
//
// F is the coefficient width; the sample type is a type parameter of each
// method, so one table serves real and complex signals at either width. A
// complex sample is filtered as two real channels, with no complex multiply.
// Filtering accumulates at the sample's component width.
//
// At reads src at a position, with the out-of-range policy of the functions
// above. Interpolate and Apply instead read Len points the caller has
// gathered, with point Len/2-1 the sample at or below the position, which is
// where looping or another edge policy goes. Positions are float64 rather
// than a sample type, since the samples may be complex.
//
// It is immutable once built and safe for concurrent use.
type SincInterpolator[F Float] struct {
	taps, phases int
	table        []F       // phases rows of taps coefficients
	window       []float64 // the Kaiser window behind each table entry
}

// NewSincInterpolator returns an interpolator of taps points, which must be
// even and at least 2, and phases fractional steps. cutoff is in cycles per
// sample, in (0, 0.5], and beta is the Kaiser window parameter. The coefficient
// width cannot be inferred and has to be given, as in
// NewSincInterpolator[float32](16, 256, 0.45, 6).
func NewSincInterpolator[F Float](taps, phases int, cutoff, beta float64) (*SincInterpolator[F], error) {
	switch {
	case taps < 2 || taps%2 != 0:
		return nil, errors.New("dsp: sinc interpolator needs an even number of taps, at least 2")
	case phases < 1:
		return nil, errors.New("dsp: sinc interpolator needs at least one phase")
	case !(cutoff > 0 && cutoff <= 0.5):
		return nil, errors.New("dsp: sinc interpolator cutoff must be in (0, 0.5] cycles per sample")
	case !(beta >= 0) || math.IsInf(beta, 1):
		return nil, errors.New("dsp: sinc interpolator Kaiser beta must be finite and not negative")
	}
	s := &SincInterpolator[F]{
		taps:   taps,
		phases: phases,
		table:  make([]F, taps*phases),
		window: make([]float64, taps*phases),
	}
	halfLen := float64(taps / 2)
	row := make([]float64, taps)
	for p := range phases {
		alpha := float64(p) / float64(phases)
		win := s.window[p*taps : (p+1)*taps]
		var sum float64
		for k := range taps {
			t := float64(k) - float64(taps/2-1) - alpha
			win[k] = kaiserAt(t, halfLen, beta)
			row[k] = sinc(2*cutoff*t) * win[k]
			sum += row[k]
		}
		// A beta large enough to overflow besselI0 makes the window NaN.
		if sum == 0 || math.IsInf(sum, 0) || math.IsNaN(sum) {
			return nil, errors.New("dsp: sinc interpolator phase has no finite, non-zero DC gain")
		}
		setReal(s.table[p*taps:(p+1)*taps], row, sum)
	}
	return s, nil
}

// Len returns the number of points each interpolation reads.
func (s *SincInterpolator[F]) Len() int { return s.taps }

// Phases returns the number of fractional steps.
func (s *SincInterpolator[F]) Phases() int { return s.phases }

// PhaseIndex returns the phase for a fraction in [0, 1). A fraction outside
// that range, or NaN, selects the nearest end.
func (s *SincInterpolator[F]) PhaseIndex(frac float64) int {
	switch {
	case !(frac > 0):
		return 0
	case frac >= 1:
		return s.phases - 1
	}
	return min(int(frac*float64(s.phases)), s.phases-1)
}

// PhaseCoeffs returns the coefficients for phase, which is in [0, Phases).
// The slice is shared with the interpolator and must not be modified.
func (s *SincInterpolator[F]) PhaseCoeffs(phase int) []F {
	return s.table[phase*s.taps : (phase+1)*s.taps : (phase+1)*s.taps]
}

// DownsampleCoeffs writes to dst the coefficients for phase with the cutoff
// lowered to cutoff cycles per input sample, in (0, 0.5]: the anti-aliasing
// filter for reading the signal at a step above one sample. It uses the same
// window as PhaseCoeffs and normalizes to a DC gain of 1; Apply reads with the
// result. It panics if dst holds fewer than Len values or cutoff is out of
// range.
func (s *SincInterpolator[F]) DownsampleCoeffs(dst []F, phase int, cutoff float64) {
	switch {
	case len(dst) < s.taps:
		panic("dsp: sinc interpolator coefficient buffer is shorter than Len")
	case !(cutoff > 0 && cutoff <= 0.5):
		panic("dsp: sinc interpolator cutoff must be in (0, 0.5] cycles per sample")
	}
	win := s.window[phase*s.taps : (phase+1)*s.taps]
	// sin(w*t) for consecutive taps is computed by rotation from the first tap.
	w := 2 * math.Pi * cutoff
	t := -float64(s.taps/2-1) - float64(phase)/float64(s.phases)
	sin, cos := math.Sincos(w * t)
	stepSin, stepCos := math.Sincos(w)
	// A stack buffer covers the usual sizes without allocating per call.
	var stack [64]float64
	var row []float64
	if s.taps <= len(stack) {
		row = stack[:s.taps]
	} else {
		row = make([]float64, s.taps)
	}
	var sum float64
	for k := range row {
		v := 1.0
		if wt := w * t; math.Abs(wt) > 1e-9 {
			v = sin / wt
		}
		row[k] = v * win[k]
		sum += row[k]
		sin, cos = sin*stepCos+cos*stepSin, cos*stepCos-sin*stepSin
		t++
	}
	setReal(dst[:s.taps], row, sum)
}

// Apply returns the sum of buf[k]*coeffs[k], which reads the signal with
// coefficients from PhaseCoeffs or DownsampleCoeffs. buf must hold
// len(coeffs) points.
func (s *SincInterpolator[F]) Apply[T Sample](buf []T, coeffs []F) T {
	return realDot(buf[:len(coeffs)], coeffs)
}

// Interpolate returns the signal at fraction frac past point Len/2-1 of buf,
// which holds Len points.
func (s *SincInterpolator[F]) Interpolate[T Sample](buf []T, frac float64) T {
	return s.Apply(buf, s.PhaseCoeffs(s.PhaseIndex(frac)))
}

// At returns src at position x, gathering the Len points around it with the
// edges held as the interpolators above do. An empty src, or an x that is
// NaN, gives zero.
func (s *SincInterpolator[F]) At[T Sample](src []T, x float64) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	coeffs := s.PhaseCoeffs(s.PhaseIndex(frac))
	first := xi - (s.taps/2 - 1)
	if first >= 0 && first+s.taps <= len(src) {
		return s.Apply(src[first:], coeffs)
	}
	var stack [64]T
	var buf []T
	if s.taps <= len(stack) {
		buf = stack[:s.taps]
	} else {
		buf = make([]T, s.taps)
	}
	for k := range buf {
		buf[k] = sampleAt(src, first+k)
	}
	return s.Apply(buf, coeffs)
}

// realDot returns the sum of x[k]*c[k] over len(c) samples of x, multiplying a
// complex sample by each real coefficient part by part. The sum accumulates at
// the component width of T.
func realDot[T Sample, F Float](x []T, c []F) T {
	var z T
	switch p := any(&z).(type) {
	case *float32:
		*p, _ = realDotInterleaved(any(x).([]float32), c, 1)
	case *float64:
		*p, _ = realDotInterleaved(any(x).([]float64), c, 1)
	case *complex64:
		re, im := realDotInterleaved(view.C64ToF32(any(x).([]complex64)), c, 2)
		*p = complex(re, im)
	case *complex128:
		re, im := realDotInterleaved(view.C128ToF64(any(x).([]complex128)), c, 2)
		*p = complex(re, im)
	default:
		panic("dsp: realDot: no case for this width")
	}
	return z
}

// realDotInterleaved returns the sums of x[lanes*k+l]*c[k] for lanes l of 0
// and, when lanes is 2, 1: a real dot product over one real channel or the two
// channels of an interleaved complex signal.
func realDotInterleaved[E, F Float](x []E, c []F, lanes int) (re, im E) {
	if lanes == 1 {
		x = x[:len(c)]
		for k, v := range c {
			re += x[k] * E(v)
		}
		return re, 0
	}
	x = x[:2*len(c)]
	for k, v := range c {
		re += x[2*k] * E(v)
		im += x[2*k+1] * E(v)
	}
	return re, im
}
