package dsp

import (
	"errors"
	"math"
	"math/cmplx"
)

// SDFT is a sliding DFT: it tracks one bin of an n point transform over the n
// most recent samples, updating it in constant time per sample rather than
// recomputing the transform.
//
// The sample type cannot be inferred from the constructor's arguments and has
// to be given, as in NewSDFT[complex128](k, n, nil).
type SDFT[T Complex] struct {
	i int
	w []T
	s []T
	x []T
	e []T
}

// NewSDFT returns a sliding DFT of bin k of an n point transform. window holds
// frequency-domain window coefficients, centered on k, which apply a window by
// combining len(window) adjacent bins: see HannFreqCoeff and the others
// alongside it. An empty window means no windowing.
//
// n must be positive and at least as wide as window. k may be any integer; bins
// repeat every n, so it is reduced into [0, n).
func NewSDFT[T Complex](k, n int, window []float64) (*SDFT[T], error) {
	if n <= 0 {
		return nil, errors.New("dsp: SDFT needs a positive number of points")
	}
	if len(window) > n {
		return nil, errors.New("dsp: SDFT window is wider than the transform")
	}
	win := []T{1}
	if len(window) > 0 {
		win = make([]T, len(window))
		for i, w := range window {
			win[i] = T(complex(w, 0))
		}
	}
	sd := &SDFT[T]{
		w: win,
		x: make([]T, n),
		e: make([]T, len(win)),
		s: make([]T, len(win)),
	}
	for i := range win {
		j := sdftBin(k-len(win)/2+i, n)
		sd.e[i] = T(cmplx.Exp(complex(0, 2*math.Pi*float64(j)/float64(n))))
	}
	return sd, nil
}

// sdftBin reduces a bin index into [0, n). Bin j and bin j+n are the same bin,
// and the window straddles k, so the index can fall either side of the range.
func sdftBin(j, n int) int {
	return ((j % n) + n) % n
}

// FilterOne feeds one sample and returns bin k over the n most recent samples.
// The recursion runs on the unit circle with no damping, so rounding error
// accumulates over a long stream -- faster at complex64 than at complex128.
func (sd *SDFT[T]) FilterOne(x T) T {
	i := (sd.i + 1) % len(sd.x)
	x0 := sd.x[i]
	sd.x[i] = x
	sd.i = i
	xd := x - x0
	var sum T
	for i, w := range sd.w {
		s := (xd + sd.s[i]) * sd.e[i]
		sd.s[i] = s
		sum += w * s
	}
	return sum
}

// Reset clears the sample history and the accumulators, so the transform starts
// a new stream from silence.
func (sd *SDFT[T]) Reset() {
	clear(sd.x)
	clear(sd.s)
	sd.i = 0
}
