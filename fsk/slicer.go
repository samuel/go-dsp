package fsk

import (
	"math"

	"samuelks.com/dsp"
)

// Slicer recovers the symbol clock from a stream of soft decisions and turns
// it into levels, 1 for mark (a positive decision) and 0 for space, one per
// symbol.
//
// The clock is a fractional phase, in symbols, so the samples per symbol need
// not be a whole number. Each zero crossing of the input is a symbol boundary;
// the clock is moved toward it by LoopGain of the error, and a level is taken
// where the clock passes mid-symbol. Between transitions the clock free-runs
// at the configured rate. Not safe for concurrent use.
type Slicer[T dsp.Float] struct {
	step  float64 // symbols per sample
	gain  float64
	start float64 // the phase Reset returns to
	lead  bool    // whether the clock starts inside a symbol that is not data

	phase   float64 // position in the current symbol, [0, 1)
	decided bool    // whether this symbol's level has been taken
	prev    float64 // the previous input sample
	since   float64 // symbols since the clock was last corrected
}

// NewSlicer returns a slicer for c whose clock starts at a symbol boundary.
func NewSlicer[T dsp.Float](c Config) (*Slicer[T], error) {
	c, err := c.validate()
	if err != nil {
		return nil, err
	}
	return newSlicer[T](c, 0, false), nil
}

// newSlicer returns a slicer whose clock starts at phase, which a Demodulator
// sets so the first boundary it expects is where its discriminator's delay
// puts it. With lead set, the symbol the clock starts in is the delay itself
// and has no level to take.
func newSlicer[T dsp.Float](c Config, phase float64, lead bool) *Slicer[T] {
	return &Slicer[T]{step: 1 / c.samplesPerSymbol(), gain: c.LoopGain, start: phase, lead: lead, phase: phase, decided: lead, since: 1}
}

// Reset returns the clock to where it started and forgets the last sample.
func (s *Slicer[T]) Reset() {
	s.phase, s.decided, s.prev, s.since = s.start, s.lead, 0, 1
}

// Slice appends to dst the level of every symbol whose middle src reaches, and
// returns the extended slice. A NaN sample counts as zero.
func (s *Slicer[T]) Slice(dst []byte, src []T) []byte {
	for _, v := range src {
		x := float64(v)
		if x != x {
			x = 0
		}
		p := s.phase + s.step

		// The level at mid-symbol, interpolated between this sample and the
		// last.
		if !s.decided && p >= 0.5 {
			t := (0.5 - s.phase) / s.step
			var level byte
			if s.prev+t*(x-s.prev) > 0 {
				level = 1
			}
			dst = append(dst, level)
			s.decided = true
		}

		// A sign change is a symbol boundary: the clock should read a whole
		// number of symbols where the input crossed zero. Noise makes the
		// input chatter across zero around a transition, and every one of
		// those crossings errs the same way, so only the first in each half
		// symbol corrects the clock.
		s.since += s.step
		if (x > 0) != (s.prev > 0) && s.since >= 0.5 {
			t := 0.5
			if d := s.prev - x; d != 0 {
				t = s.prev / d
			}
			if !(t >= 0 && t <= 1) { // NaN, from infinities
				t = 0.5
			}
			at := s.phase + t*s.step
			p -= s.gain * (at - math.Round(at))
			s.since = 0
		}

		switch {
		case p >= 1:
			p--
			s.decided = false
		case p < 0:
			// Pulled back into the symbol before, whose level is taken.
			p++
			s.decided = true
		}
		s.phase, s.prev = p, x
	}
	return dst
}
