package dspviz

import (
	"errors"
	"math"
)

// SweepOptions describes a swept sine.
type SweepOptions struct {
	// Low and High bound the sweep in Hz. Zero High selects the input Nyquist
	// frequency, which is what makes a decimator's aliasing visible: the
	// interesting part is what happens to the tone once it passes the *output*
	// Nyquist and can no longer be represented.
	Low, High float64

	// Duration in seconds. Zero selects 8.
	Duration float64

	// Amplitude as a fraction of full scale. Zero selects 0.5, which is -6 dBFS
	// and leaves room for a filter with gain.
	Amplitude float64

	// Fade is the length in seconds of the raised-cosine taper at each end.
	// Zero selects 5 ms. Without it the sweep starts and stops on a
	// discontinuity, and the click smears across the whole plot.
	Fade float64
}

// SweepSignal fills dst with a linear swept sine and returns it.
//
// The sweep is linear in frequency because that is what the spectrogram's axis
// is: on a linear frequency axis the sweep draws as a straight line, and so
// does anything the filter adds -- an alias folding off Nyquist, a harmonic at
// twice the slope -- each at an angle that says what it is. A logarithmic sweep
// turns all of them into curves that are harder to read.
//
// The phase is accumulated in integers, as Tone's is: over an eight-second
// sweep at 48 kHz it reaches 5e6 radians, where float64 has lost seven digits.
// sweepScale is how finely the sweep's endpoints are resolved, in units of a
// hertz; a power of two so the scaling is exact.
const sweepScale = 1 << 10

func SweepSignal(dst []float64, rate float64, opt SweepOptions) []float64 {
	n := len(dst)
	if n == 0 {
		return dst
	}
	low, high := opt.Low, opt.High
	if high <= 0 {
		high = rate / 2
	}
	amp := opt.Amplitude
	if amp == 0 {
		amp = 0.5
	}

	// phi(i)/(2*pi) = (low*i + (high-low)*i^2/(2n)) / rate. Scaling by
	// 2*n*rate*sweepScale makes the numerator integer, so it reduces modulo the
	// period exactly and never grows to lose precision.
	//
	// Rounding the span to a whole number of hertz would move the end of the
	// sweep, so both constants are scaled first -- and that scaling makes
	// b*i*i overflow int64 at thirty seconds and 192 kHz, the longest sweep the
	// server runs. Past the overflow the phase is not imprecise but unrelated:
	// the cosine comes out at +1 where it should be -1.
	//
	// So the numerator is advanced by its own difference, a + b*(2i+1), which
	// stays below the modulus term by term.
	den := 2 * int64(n) * int64(math.Round(rate)) * sweepScale
	a := int64(math.Round(low * 2 * float64(n) * sweepScale))
	b := int64(math.Round((high - low) * sweepScale))
	var num int64
	for i := range dst {
		dst[i] = amp * math.Cos(2*math.Pi*float64(num)/float64(den))
		num = (num + a + b*(2*int64(i)+1)) % den
	}

	fade := opt.Fade
	if fade == 0 {
		fade = 0.005
	}
	if f := int(fade * rate); f > 1 && 2*f < n {
		for i := range f {
			w := 0.5 - 0.5*math.Cos(math.Pi*float64(i)/float64(f))
			dst[i] *= w
			dst[n-1-i] *= w
		}
	}
	return dst
}

// Sweep runs a swept sine through p and returns the spectrogram of what comes
// out. It is the structural view: an alias appears as a straight line mirrored
// off Nyquist, a harmonic as one at a multiple of the slope, quantization
// noise as speckle where there should be black.
//
// It is not the measurement. The spectrogram's floor is its window and what its
// palette can encode, so a 140 dB stopband will not show however carefully it
// is built. Read depth off a tone measurement or Freqz; read shape off here.
func Sweep(p Processor, opt SweepOptions, st STFTOptions) (*Spectrogram, error) {
	if p == nil {
		return nil, errors.New("dspviz: nil processor")
	}
	inRate, outRate := p.Rates()
	dur := opt.Duration
	if dur == 0 {
		dur = 8
	}
	n := int(dur * inRate)
	if n <= 0 {
		return nil, errors.New("dspviz: the sweep is shorter than one sample")
	}

	in := SweepSignal(make([]float64, n), inRate, opt)
	p.Reset()
	out := p.Process(nil, in)
	if len(out) == 0 {
		return nil, errors.New("dspviz: the processor produced no output")
	}
	return SpectrogramOf(out, outRate, st)
}

// SweepChart wraps a spectrogram in axes and a color bar.
func SweepChart(sg *Spectrogram, p Palette, opt SpectrogramOptions) *Chart {
	return sg.chart(p, opt)
}
