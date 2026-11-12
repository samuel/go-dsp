package fsk

import (
	"errors"
	"math"
	"math/cmplx"

	"samuelks.com/dsp"
)

// Discriminator turns FSK samples into one soft decision per sample: the
// magnitude of the input's correlation with the mark tone less its correlation
// with the space tone, each over a sliding window of one symbol. It is positive
// for mark and negative for space, and a unit tone reads about ±1 complex or
// ±0.5 real.
//
// It implements dsp.Processor[In, Out], one output per input. Not safe for
// concurrent use.
type Discriminator[In dsp.Sample, Out dsp.Float] struct {
	n      int // window length in samples
	mark   *dsp.Oscillator[complex128]
	space  *dsp.Oscillator[complex128]
	sumM   complex128 // the correlations over the window
	sumS   complex128
	ringM  []complex128 // the window's products, oldest at pos
	ringS  []complex128
	pos    int
	loM    []complex128 // a block of each tone's conjugate
	loS    []complex128
	scaleN float64
}

// block is how many oscillator samples a Discriminator generates at a time.
const block = 256

// NewDiscriminator returns a discriminator for c whose window holds the last
// symbol's samples, rounded to a whole number, all zero to start. A real In
// cannot tell a tone from its negative, so for real input Mark and Space must
// differ in magnitude.
func NewDiscriminator[In dsp.Sample, Out dsp.Float](c Config) (*Discriminator[In, Out], error) {
	c, err := c.validate()
	if err != nil {
		return nil, err
	}
	if !isComplex[In]() && math.Abs(c.Mark) == math.Abs(c.Space) {
		return nil, errors.New("fsk: real samples cannot tell mark from space when they are the same distance from 0 Hz")
	}
	n := max(int(math.Round(c.samplesPerSymbol())), 2)
	// The conjugates of the tones, so the products are correlations.
	mark, err := dsp.NewOscillator[complex128](-c.Mark / c.Rate)
	if err != nil {
		return nil, err
	}
	space, err := dsp.NewOscillator[complex128](-c.Space / c.Rate)
	if err != nil {
		return nil, err
	}
	return &Discriminator[In, Out]{
		n:      n,
		mark:   mark,
		space:  space,
		ringM:  make([]complex128, n),
		ringS:  make([]complex128, n),
		loM:    make([]complex128, block),
		loS:    make([]complex128, block),
		scaleN: 1 / float64(n),
	}, nil
}

// Delay is how many samples a change of tone in the input takes to reach the
// zero crossing of the output: half the window.
func (d *Discriminator[In, Out]) Delay() int { return d.n / 2 }

// OutputLen returns inputs: one decision per sample.
func (d *Discriminator[In, Out]) OutputLen(inputs int) int { return inputs }

// Reset empties the window and restarts both tones at phase zero.
func (d *Discriminator[In, Out]) Reset() {
	d.mark.Reset()
	d.space.Reset()
	d.sumM, d.sumS = 0, 0
	clear(d.ringM)
	clear(d.ringS)
	d.pos = 0
}

// Process writes one decision per sample of src to dst and returns len(src).
// It panics if dst is shorter than src.
func (d *Discriminator[In, Out]) Process(dst []Out, src []In) int {
	if len(dst) < len(src) {
		panic("fsk: Discriminator.Process dst is shorter than OutputLen(len(src))")
	}
	switch s := any(src).(type) {
	case []float32:
		discriminate(d, dst, s, func(v float32) complex128 { return complex(float64(v), 0) })
	case []float64:
		discriminate(d, dst, s, func(v float64) complex128 { return complex(v, 0) })
	case []complex64:
		discriminate(d, dst, s, func(v complex64) complex128 { return complex128(v) })
	case []complex128:
		discriminate(d, dst, s, func(v complex128) complex128 { return v })
	}
	return len(src)
}

// discriminate is Process at one input type. The window sums are kept running,
// one product in and one out per sample, and recomputed from the window each
// time it comes round, so their rounding never builds up past one window's
// worth.
func discriminate[In dsp.Sample, Out dsp.Float, S any](d *Discriminator[In, Out], dst []Out, src []S, widen func(S) complex128) {
	for start := 0; start < len(src); start += block {
		k := min(block, len(src)-start)
		d.mark.Generate(d.loM[:k])
		d.space.Generate(d.loS[:k])
		for i, v := range src[start : start+k] {
			x := widen(v)
			pm, ps := x*d.loM[i], x*d.loS[i]
			d.sumM += pm - d.ringM[d.pos]
			d.sumS += ps - d.ringS[d.pos]
			d.ringM[d.pos], d.ringS[d.pos] = pm, ps
			if d.pos++; d.pos == d.n {
				d.pos = 0
				d.sumM, d.sumS = sum(d.ringM), sum(d.ringS)
			}
			dst[start+i] = Out((cmplx.Abs(d.sumM) - cmplx.Abs(d.sumS)) * d.scaleN)
		}
	}
}

func sum(x []complex128) complex128 {
	var s complex128
	for _, v := range x {
		s += v
	}
	return s
}

func isComplex[T dsp.Sample]() bool {
	var z T
	switch any(z).(type) {
	case complex64, complex128:
		return true
	}
	return false
}
