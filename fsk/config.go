package fsk

import (
	"fmt"
	"math"
)

// DefaultLoopGain is the share of each zero crossing's timing error the Slicer
// corrects its clock by when Config.LoopGain is zero. It locks within a few
// transitions and tracks a transmitter clock a few percent off.
const DefaultLoopGain = 0.25

// Config describes a binary FSK signal. Every type in the package takes one,
// and each constructor validates it the same way.
type Config struct {
	// Mark and Space are the tones for a 1 level and a 0 level, in Hz. Either
	// may be negative for complex baseband, where they sit either side of the
	// carrier.
	Mark, Space float64

	// Baud is the symbol rate. Rate/Baud is the samples per symbol, which
	// need not be a whole number but must be at least 2.
	Baud float64

	// Rate is the sample rate in Hz.
	Rate float64

	// LoopGain is the share of each zero crossing's timing error the Slicer
	// corrects its clock by, in (0, 1). Zero selects DefaultLoopGain.
	LoopGain float64
}

// validate checks c and returns it with the defaults filled in.
func (c Config) validate() (Config, error) {
	for name, v := range map[string]float64{
		"Mark": c.Mark, "Space": c.Space, "Baud": c.Baud, "Rate": c.Rate, "LoopGain": c.LoopGain,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return c, fmt.Errorf("fsk: %s is %v", name, v)
		}
	}
	switch {
	case c.Rate <= 0:
		return c, fmt.Errorf("fsk: a sample rate of %g Hz", c.Rate)
	case c.Baud <= 0:
		return c, fmt.Errorf("fsk: a symbol rate of %g baud", c.Baud)
	case c.Rate/c.Baud < 2:
		return c, fmt.Errorf("fsk: %g baud at %g Hz is under two samples a symbol", c.Baud, c.Rate)
	case c.Mark == c.Space:
		return c, fmt.Errorf("fsk: mark and space are both %g Hz", c.Mark)
	case math.Abs(c.Mark) >= c.Rate/2 || math.Abs(c.Space) >= c.Rate/2:
		return c, fmt.Errorf("fsk: mark %g Hz and space %g Hz must be below the Nyquist frequency, %g Hz",
			c.Mark, c.Space, c.Rate/2)
	case c.LoopGain < 0 || c.LoopGain >= 1:
		return c, fmt.Errorf("fsk: a loop gain of %g, outside (0, 1)", c.LoopGain)
	}
	if c.LoopGain == 0 {
		c.LoopGain = DefaultLoopGain
	}
	return c, nil
}

// samplesPerSymbol is Rate/Baud.
func (c Config) samplesPerSymbol() float64 { return c.Rate / c.Baud }
