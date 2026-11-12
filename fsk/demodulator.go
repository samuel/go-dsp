package fsk

import (
	"math"

	"samuelks.com/dsp"
)

// Demodulator is a Discriminator into a Slicer: FSK samples in, one level per
// symbol out. The Slicer's clock starts where the Discriminator's delay puts
// the first symbol boundary. Not safe for concurrent use.
type Demodulator[In dsp.Sample] struct {
	disc    *Discriminator[In, float64]
	slicer  *Slicer[float64]
	scratch []float64
}

// NewDemodulator returns a demodulator for c.
func NewDemodulator[In dsp.Sample](c Config) (*Demodulator[In], error) {
	c, err := c.validate()
	if err != nil {
		return nil, err
	}
	disc, err := NewDiscriminator[In, float64](c)
	if err != nil {
		return nil, err
	}
	// A boundary at sample 0 of the input reaches the output Delay samples
	// later, so that is where the clock should read a whole symbol.
	delay := float64(disc.Delay()) / c.samplesPerSymbol()
	phase := math.Ceil(delay) - delay
	return &Demodulator[In]{
		disc:    disc,
		slicer:  newSlicer[float64](c, phase, phase > 0),
		scratch: make([]float64, block),
	}, nil
}

// Demodulate appends to dst the level of every symbol src completes, 1 for
// mark and 0 for space, and returns the extended slice.
func (d *Demodulator[In]) Demodulate(dst []byte, src []In) []byte {
	for start := 0; start < len(src); start += len(d.scratch) {
		chunk := src[start:min(start+len(d.scratch), len(src))]
		soft := d.scratch[:len(chunk)]
		d.disc.Process(soft, chunk)
		dst = d.slicer.Slice(dst, soft)
	}
	return dst
}

// Reset returns both stages to their initial state.
func (d *Demodulator[In]) Reset() {
	d.disc.Reset()
	d.slicer.Reset()
}
