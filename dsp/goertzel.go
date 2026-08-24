package dsp

import (
	"errors"
	"math"
)

// Goertzel detects the power at a set of frequencies in a block of real
// samples. It is equivalent to evaluating a DFT at only the bins of interest,
// which is much cheaper than a full FFT for a handful of frequencies.
//
// The recursion and the results are float64 whatever the sample width is; the
// type parameter only says what Feed accepts, and it is inferred from the slice
// passed to it.
type Goertzel[T Float] struct {
	freq      []*goertzel
	blockSize int
	fed       int
	pow       []float64
	cplx      []complex128
}

// ComplexGoertzel is Goertzel for complex samples. It runs the recursion
// separately over the real and imaginary parts and combines the two, so unlike
// Goertzel it tells a positive frequency from its negative image -- which also
// makes a negative target frequency meaningful here.
type ComplexGoertzel[T Complex] struct {
	freq      []*goertzel
	blockSize int
	fed       int
	pow       []float64
	cplx      []complex128
}

type goertzel struct {
	coeff    float64
	cos, sin float64
	q1, q2   float64
	q1i, q2i float64
}

// newGoertzelBins builds the per-frequency state shared by both detectors. Each
// target is rounded to the nearest of the blockSize bins at sampleRate; a
// frequency that is not a bin center leaks into its neighbors.
func newGoertzelBins(targetFreqs []float64, sampleRate float64, blockSize int) ([]*goertzel, error) {
	if !(sampleRate > 0) {
		return nil, errors.New("dsp: Goertzel needs a positive sample rate")
	}
	if blockSize <= 0 {
		return nil, errors.New("dsp: Goertzel needs a positive block size")
	}
	freq := make([]*goertzel, len(targetFreqs))
	for i, f := range targetFreqs {
		// k is the closest bin for the frequency.
		k := math.Round(float64(blockSize) * f / sampleRate)
		w := 2 * math.Pi * k / float64(blockSize)
		sin, cos := math.Sincos(w)
		freq[i] = &goertzel{
			coeff: 2 * cos,
			cos:   cos,
			sin:   sin,
		}
	}
	return freq, nil
}

// power returns the squared magnitude of the bin, for a detector fed real
// samples. It is the closed form of |Complex()|^2: expanding
// |q1*cos - q2 + j*q1*sin|^2 cancels the trigonometry and leaves this.
func (g *goertzel) power() float64 {
	return g.q1*g.q1 + g.q2*g.q2 - g.q1*g.q2*g.coeff
}

// value returns the complex value of the bin, for a detector fed real samples.
func (g *goertzel) value() complex128 {
	return complex(g.q1*g.cos-g.q2, g.q1*g.sin)
}

// complexValue returns the complex value of the bin, for a detector fed complex
// samples, combining the two recursions.
func (g *goertzel) complexValue() complex128 {
	return complex(
		g.q1*g.cos-g.q2-g.q1i*g.sin,
		g.q1*g.sin+g.q1i*g.cos-g.q2i,
	)
}

func (g *goertzel) reset() {
	g.q1, g.q2, g.q1i, g.q2i = 0, 0, 0, 0
}

// NewGoertzel returns a Goertzel for the given frequencies in Hz, which need not
// be whole numbers. Feed blockSize samples, read Power or Complex, then Reset
// before the next block. sampleRate is a float64 like the biquad constructors
// take, so a rate that is not a whole number of Hz need not be rounded first.
func NewGoertzel[T Float](targetFreqs []float64, sampleRate float64, blockSize int) (*Goertzel[T], error) {
	bins, err := newGoertzelBins(targetFreqs, sampleRate, blockSize)
	if err != nil {
		return nil, err
	}
	return &Goertzel[T]{
		freq:      bins,
		blockSize: blockSize,
		pow:       make([]float64, len(targetFreqs)),
		cplx:      make([]complex128, len(targetFreqs)),
	}, nil
}

// Reset clears the filter state so the next block starts fresh.
func (g *Goertzel[T]) Reset() {
	for _, freq := range g.freq {
		freq.reset()
	}
	g.fed = 0
}

// Feed accumulates samples into the filter state. A block may be fed across
// several calls, but not more than blockSize samples in total: past that the
// recursion has run longer than the bin it was built for, and the result would
// be silently wrong.
func (g *Goertzel[T]) Feed(src []T) {
	g.fed = checkGoertzelFed(g.fed, len(src), g.blockSize)
	for _, samp := range src {
		s := float64(samp)
		for _, freq := range g.freq {
			q0 := freq.coeff*freq.q1 - freq.q2 + s
			freq.q2 = freq.q1
			freq.q1 = q0
		}
	}
}

// Power returns the squared magnitude at each target frequency, in the order
// the frequencies were given to NewGoertzel. The returned slice is reused by
// every call. It panics unless a whole block has been fed.
func (g *Goertzel[T]) Power() []float64 {
	checkGoertzelBlock(g.fed, g.blockSize)
	for i, freq := range g.freq {
		g.pow[i] = freq.power()
	}
	return g.pow
}

// Complex returns the complex value at each target frequency, in the order the
// frequencies were given to NewGoertzel. The returned slice is reused by every
// call. It panics unless a whole block has been fed.
func (g *Goertzel[T]) Complex() []complex128 {
	checkGoertzelBlock(g.fed, g.blockSize)
	for i, freq := range g.freq {
		g.cplx[i] = freq.value()
	}
	return g.cplx
}

// NewComplexGoertzel returns a ComplexGoertzel for the given frequencies in Hz,
// which may be negative. See NewGoertzel for how the frequencies and blockSize
// are used.
func NewComplexGoertzel[T Complex](targetFreqs []float64, sampleRate float64, blockSize int) (*ComplexGoertzel[T], error) {
	bins, err := newGoertzelBins(targetFreqs, sampleRate, blockSize)
	if err != nil {
		return nil, err
	}
	return &ComplexGoertzel[T]{
		freq:      bins,
		blockSize: blockSize,
		pow:       make([]float64, len(targetFreqs)),
		cplx:      make([]complex128, len(targetFreqs)),
	}, nil
}

// Reset clears the filter state so the next block starts fresh.
func (g *ComplexGoertzel[T]) Reset() {
	for _, freq := range g.freq {
		freq.reset()
	}
	g.fed = 0
}

// Feed accumulates samples into the filter state, under the same rules as
// Goertzel.Feed.
func (g *ComplexGoertzel[T]) Feed(src []T) {
	g.fed = checkGoertzelFed(g.fed, len(src), g.blockSize)
	for _, samp := range src {
		s := complex128(samp)
		for _, freq := range g.freq {
			q0 := freq.coeff*freq.q1 - freq.q2 + real(s)
			freq.q2 = freq.q1
			freq.q1 = q0
			q0 = freq.coeff*freq.q1i - freq.q2i + imag(s)
			freq.q2i = freq.q1i
			freq.q1i = q0
		}
	}
}

// Power returns the squared magnitude at each target frequency, in the order
// the frequencies were given to NewComplexGoertzel. The returned slice is
// reused by every call. It panics unless a whole block has been fed.
func (g *ComplexGoertzel[T]) Power() []float64 {
	for i, c := range g.Complex() {
		g.pow[i] = real(c)*real(c) + imag(c)*imag(c)
	}
	return g.pow
}

// Complex returns the complex value at each target frequency, in the order the
// frequencies were given to NewComplexGoertzel. The returned slice is reused
// by every call. It panics unless a whole block has been fed.
func (g *ComplexGoertzel[T]) Complex() []complex128 {
	checkGoertzelBlock(g.fed, g.blockSize)
	for i, freq := range g.freq {
		g.cplx[i] = freq.complexValue()
	}
	return g.cplx
}

func checkGoertzelFed(fed, n, blockSize int) int {
	if fed+n > blockSize {
		panic("dsp: Goertzel fed more than blockSize samples; Reset between blocks")
	}
	return fed + n
}

func checkGoertzelBlock(fed, blockSize int) {
	if fed != blockSize {
		panic("dsp: Goertzel result read before a whole block was fed")
	}
}
