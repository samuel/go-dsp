package fsk

import (
	"math"

	"samuelks.com/dsp"
)

// Modulator turns levels into phase-continuous FSK: a cosine at the mark or
// space frequency for a real T, exp(+j*2*pi*f*n) for a complex one. Each level
// is a byte, nonzero for mark.
//
// It implements dsp.Processor[byte, T]. A symbol is Rate/Baud samples, which
// need not be a whole number, so symbol k ends at sample floor(k*Rate/Baud) and
// OutputLen depends on how many symbols have gone before. Not safe for
// concurrent use.
type Modulator[T dsp.Sample] struct {
	c       Config
	osc     *dsp.Oscillator[T]
	symbols int64 // levels modulated so far
}

// NewModulator returns a modulator for c at phase zero.
func NewModulator[T dsp.Sample](c Config) (*Modulator[T], error) {
	c, err := c.validate()
	if err != nil {
		return nil, err
	}
	osc, err := dsp.NewOscillator[T](c.Space / c.Rate)
	if err != nil {
		return nil, err
	}
	return &Modulator[T]{c: c, osc: osc}, nil
}

// end is the sample symbol k ends at.
func (m *Modulator[T]) end(k int64) int64 {
	return int64(math.Floor(float64(k) * m.c.Rate / m.c.Baud))
}

// OutputLen returns how many samples the next inputs levels take.
func (m *Modulator[T]) OutputLen(inputs int) int {
	return int(m.end(m.symbols+int64(inputs)) - m.end(m.symbols))
}

// Process writes the samples of the levels in src to dst and returns how many
// it wrote, OutputLen(len(src)). It panics if dst is shorter than that.
func (m *Modulator[T]) Process(dst []T, src []byte) int {
	need := m.OutputLen(len(src))
	if len(dst) < need {
		panic("fsk: Modulator.Process dst is shorter than OutputLen(len(src))")
	}
	at := 0
	for _, level := range src {
		f := m.c.Space
		if level != 0 {
			f = m.c.Mark
		}
		_ = m.osc.SetFrequency(f / m.c.Rate) // finite: the Config was validated
		n := int(m.end(m.symbols+1) - m.end(m.symbols))
		m.osc.Generate(dst[at : at+n])
		at += n
		m.symbols++
	}
	return at
}

// Reset returns the modulator to phase zero at the start of a symbol.
func (m *Modulator[T]) Reset() {
	m.osc.Reset()
	m.symbols = 0
}
