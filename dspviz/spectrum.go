package dspviz

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"

	"github.com/samuel/go-dsp/dsp"
)

// SpectrumOptions tunes ToneSpectrum. The zero value is the default throughout.
type SpectrumOptions struct {
	// Frame is the transform length, rounded up to a power of two. Zero selects
	// 65536, which is 1.4 seconds at 48 kHz and gives bins 0.7 Hz apart.
	Frame int

	// Settle is how many samples are discarded before the frame. Zero derives
	// one, as ResponseOptions does.
	Settle int

	// Amplitude of the tone, as a fraction of full scale. Zero selects 1.
	Amplitude float64

	// Window is applied before the transform. Nil uses no window at all, which
	// is correct and best when the tone is coherent -- and it is, unless the
	// processor changes the sample rate.
	Window func([]float64)
}

// Spectrum is a magnitude spectrum in dB relative to full scale.
type Spectrum struct {
	Rate  float64
	Freq  []float64
	MagDB []float64

	// Fundamental is the frequency of the injected tone, and Bin is where it
	// landed, so the distortion measurements below need not search for it.
	Fundamental float64
	Bin         int

	Warnings []string
}

// ToneSpectrum drives p with a single tone and transforms what comes out. It is
// the plot that shows what a filter adds rather than what it removes:
// harmonics, intermodulation, quantization noise and, for a rate changer, the
// aliases.
func ToneSpectrum(p Processor, freq float64, opt SpectrumOptions) (*Spectrum, error) {
	if p == nil {
		return nil, errors.New("dspviz: nil processor")
	}
	inRate, outRate := p.Rates()

	frame := roundPow2(opt.Frame, defaultFrame)
	amp := opt.Amplitude
	if amp == 0 {
		amp = 1
	}
	settle := opt.Settle
	if settle <= 0 {
		settle = settleFor(p, frame)
	}
	settle = (settle + frame - 1) / frame * frame

	hz, k := CoherentFreq(freq, inRate, frame)
	in := Tone(make([]float64, settle+frame), k, frame, amp)

	p.Reset()
	out := p.Process(nil, in)

	want := int(math.Round(float64(frame) * outRate / inRate))
	if want <= 0 || want > len(out) {
		return nil, fmt.Errorf("dspviz: the processor returned %d samples, too few for a frame of %d", len(out), want)
	}
	// Take a power-of-two frame ending at the tail, so the transform length is
	// one the FFT accepts.
	n := 1
	for n<<1 <= want {
		n <<= 1
	}
	tail := out[len(out)-n:]

	s := &Spectrum{Rate: outRate, Fundamental: hz}

	buf := make([]float64, n)
	copy(buf, tail)
	// A rectangular window's coherent gain is its length, which is what an
	// unwindowed frame gets.
	gain := float64(n)
	if opt.Window != nil {
		w := make([]float64, n)
		opt.Window(w)
		for i := range buf {
			buf[i] *= w[i]
		}
		gain, _ = dsp.WindowGain(w)
	}

	f, err := dsp.NewFFT[complex128](n)
	if err != nil {
		return nil, err
	}
	spec := make([]complex128, n)
	dsp.ForwardReal(f, spec, buf)

	// Half the bins, scaled so a sine at the drive amplitude reads 0 dB. The
	// factor of two is the negative-frequency image, which a real signal puts
	// in the mirror bin.
	half := n/2 + 1
	s.Freq = make([]float64, half)
	s.MagDB = make([]float64, half)
	for i := range half {
		s.Freq[i] = float64(i) * outRate / float64(n)
		m := 2 * cmplx.Abs(spec[i]) / gain
		if i == 0 || i == n/2 {
			m /= 2 // DC and Nyquist have no mirror
		}
		// Relative to the drive amplitude rather than to full scale: dividing by
		// amp puts a transparent processor's fundamental at 0 dB whatever level
		// it was driven at, which is what makes the distortion figures read off
		// this curve comparable between runs. The axis label says so.
		s.MagDB[i] = dbAmplitude(m / amp)
	}
	s.Bin = int(math.Round(hz * float64(n) / outRate))
	if s.Bin >= half {
		s.Bin = half - 1
	}

	if inRate != outRate {
		s.Warnings = append(s.Warnings, fmt.Sprintf(
			"the rate change from %g to %g leaves the tone off-bin at the output; "+
				"without a window the leakage bounds this plot near -13 dB", inRate, outRate))
	}
	if opt.Window == nil && inRate == outRate {
		// No window is the right choice here and worth saying, since it looks
		// like an omission.
		s.Warnings = append(s.Warnings, "coherent tone, rectangular window: the floor is the arithmetic, not a window's sidelobes")
	}
	return s, nil
}

// Peak returns the highest bin within one part in a thousand of near, and its
// level.
func (s *Spectrum) Peak(near float64) (hz, db float64) {
	best := -1
	for i, f := range s.Freq {
		if math.Abs(f-near) > near*0.001+s.binWidth() {
			continue
		}
		if best < 0 || s.MagDB[i] > s.MagDB[best] {
			best = i
		}
	}
	if best < 0 {
		return 0, math.Inf(-1)
	}
	return s.Freq[best], s.MagDB[best]
}

func (s *Spectrum) binWidth() float64 {
	if len(s.Freq) < 2 {
		return 0
	}
	return s.Freq[1] - s.Freq[0]
}

// SFDR returns the spurious-free dynamic range in dB: how far the largest thing
// that is not the fundamental sits below the fundamental. It is negative, and
// more negative is better.
//
// The fundamental's own main lobe is excluded, along with DC, which carries any
// offset rather than any distortion.
func (s *Spectrum) SFDR() float64 {
	if len(s.MagDB) == 0 {
		return 0
	}
	fund := s.MagDB[s.Bin]
	worst := math.Inf(-1)
	for i, v := range s.MagDB {
		if i < 2 || nearBin(i, s.Bin, 2) {
			continue
		}
		worst = math.Max(worst, v)
	}
	return worst - fund
}

// THD returns the total harmonic distortion in dB: the power in the first
// harmonics above the fundamental, relative to the fundamental.
//
// A harmonic above Nyquist is folded back to where it actually appears rather
// than being ignored, since that is where the energy really is. With the
// fundamental on an odd bin of a power-of-two transform no harmonic can land
// back on the fundamental's own bin, which is exactly why CoherentFreq forces
// it odd.
func (s *Spectrum) THD(harmonics int) float64 {
	if len(s.MagDB) == 0 || harmonics < 2 {
		return math.Inf(-1)
	}
	fund := Amplitude(s.MagDB[s.Bin])
	if fund == 0 {
		return math.Inf(-1)
	}
	var power float64
	for h := 2; h <= harmonics; h++ {
		i := foldBin(s.Bin*h, len(s.MagDB)-1)
		if nearBin(i, s.Bin, 2) || i < 2 {
			continue
		}
		a := Amplitude(s.MagDB[i])
		power += a * a
	}
	if power == 0 {
		return math.Inf(-1)
	}
	return dbAmplitude(math.Sqrt(power) / fund)
}

// THDN returns total harmonic distortion plus noise: everything that is not the
// fundamental, relative to the fundamental.
func (s *Spectrum) THDN() float64 {
	if len(s.MagDB) == 0 {
		return math.Inf(-1)
	}
	fund := Amplitude(s.MagDB[s.Bin])
	if fund == 0 {
		return math.Inf(-1)
	}
	var power float64
	for i, v := range s.MagDB {
		if i < 2 || nearBin(i, s.Bin, 2) {
			continue
		}
		a := Amplitude(v)
		power += a * a
	}
	if power == 0 {
		return math.Inf(-1)
	}
	return dbAmplitude(math.Sqrt(power) / fund)
}

// foldBin reflects a bin index back into [0, half] the way a frequency above
// Nyquist folds back into the band.
func foldBin(k, half int) int {
	if half <= 0 {
		return 0
	}
	period := 2 * half
	k %= period
	if k < 0 {
		k += period
	}
	if k > half {
		k = period - k
	}
	return k
}

func nearBin(i, k, width int) bool { return i >= k-width && i <= k+width }

func roundPow2(n, def int) int {
	if n <= 0 {
		return def
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// SpectrumChart draws a magnitude spectrum, annotated with the distortion
// figures read off it.
func SpectrumChart(s *Spectrum, opt ChartOptions) *Chart {
	floor := floorFor(opt.FloorDB, -180, minOf(s.MagDB))

	c := &Chart{
		Subtitle: fmt.Sprintf("%.1f Hz: SFDR %.1f dB, THD %.1f dB, THD+N %.1f dB",
			s.Fundamental, s.SFDR(), s.THD(10), s.THDN()),
		X:      Axis{Label: hzLabel, Min: 0, Max: s.Rate / 2},
		Y:      Axis{Label: "level (dB relative to the drive amplitude)", Min: floorTo(floor, 20), Max: 6},
		Series: []Series{{Name: "spectrum", X: s.Freq, Y: s.MagDB, Width: 1}},
		Notes:  s.Warnings,
	}
	if !opt.Linear {
		// A spectrum is conventionally drawn on a linear frequency axis: the
		// harmonics of a tone are evenly spaced, and that is the pattern the
		// plot exists to show.
		c.X.Log = false
	}
	return opt.apply(c, "Tone spectrum")
}
