package dsp

import (
	"errors"
	"math"
	"math/cmplx"
)

// Costas is a Costas loop: it tracks the carrier of a suppressed-carrier PSK signal, BPSK
// (order 2) or QPSK (order 4), and writes the signal derotated by it. BPSK locks with its
// symbols on the real axis and QPSK with its symbols at odd multiples of 45°, each with the
// ambiguity of the modulation (180° for BPSK, 90° for QPSK) that a differential code or a
// sync word resolves.
//
// The phase detector is the angle of the derotated sample raised to the order, divided by the
// order, which is the phase error modulo the ambiguity whatever the signal's amplitude. The
// loop filter is PLL's. Its state is float64 at every width. Not safe for concurrent use.
type Costas[C Complex] struct {
	order       int
	alpha, beta float64 // proportional and integral gains
	maxFreq     float64 // radians per sample
	phase, freq float64 // radians, radians per sample
}

// NewCostas returns a Costas loop of the given order, 2 for BPSK or 4 for QPSK, with the given
// noise bandwidth in cycles per sample, that tracks a carrier within maxFreq cycles per sample
// of 0 Hz. The loop runs once per sample, so a signal at several samples per symbol wants a
// bandwidth a few times narrower than the symbol rate. The loop starts at 0 Hz.
func NewCostas[C Complex](order int, loopBandwidth, maxFreq float64) (*Costas[C], error) {
	switch {
	case order != 2 && order != 4:
		return nil, errors.New("dsp: Costas loop order must be 2 (BPSK) or 4 (QPSK)")
	case !(loopBandwidth > 0 && loopBandwidth < 0.5):
		return nil, errors.New("dsp: Costas loop bandwidth must be in (0, 0.5) cycles per sample")
	case !(maxFreq >= 0 && maxFreq <= 0.5):
		return nil, errors.New("dsp: Costas loop frequency range must be in [0, 0.5] cycles per sample")
	}
	alpha, beta := loopGains(loopBandwidth)
	return &Costas[C]{order: order, alpha: alpha, beta: beta, maxFreq: 2 * math.Pi * maxFreq}, nil
}

// Frequency returns the tracked carrier frequency in cycles per sample.
func (c *Costas[C]) Frequency() float64 { return c.freq / (2 * math.Pi) }

// Order returns the loop's order: 2 for BPSK, 4 for QPSK.
func (c *Costas[C]) Order() int { return c.order }

// OutputLen returns inputs: the loop writes one sample per input.
func (c *Costas[C]) OutputLen(inputs int) int { return inputs }

// Process writes src derotated by the tracked carrier to dst and returns len(src). It panics
// if dst is shorter than src. dst may be src.
func (c *Costas[C]) Process(dst, src []C) int {
	checkDst("Costas", len(dst), len(src))
	for i, x := range src {
		s, co := math.Sincos(c.phase)
		v := complex128(x) * complex(co, -s)
		dst[i] = C(v)
		var err float64
		if c.order == 2 {
			// The angle of v² is twice the error from the nearest point on the real axis.
			err = cmplx.Phase(v*v) / 2
		} else {
			// v⁴ of a point at 45° is -1, so the angle of -v⁴ is four times the error.
			v2 := v * v
			err = cmplx.Phase(-v2*v2) / 4
		}
		if v == 0 {
			err = 0
		}
		c.freq = max(-c.maxFreq, min(c.maxFreq, c.freq+c.beta*err))
		c.phase = math.Remainder(c.phase+c.freq+c.alpha*err, 2*math.Pi)
	}
	return len(src)
}

// Reset returns the loop to 0 Hz and phase zero.
func (c *Costas[C]) Reset() { c.phase, c.freq = 0, 0 }
