package dspviz

import (
	"fmt"
	"math"
	"slices"
)

// PSD is an averaged power spectrum with peak-hold and min-hold, in dBFS per
// bin.
//
// The three curves separate what a spectrum can hold: a steady carrier puts all
// three on top of one another; noise leaves peak and minimum straddling the
// average by the spread of a chi-squared with two degrees of freedom; an
// intermittent spur shows in the peak alone. A spectrogram cannot tell which of
// those it is looking at, which is why every RF monitor draws all three.
type PSD struct {
	Rate float64

	// Center is the frequency an I/Q capture was tuned to, for labeling only.
	// Freq stays in baseband offsets, as a two-sided Spectrogram's does.
	Center float64

	Freq  []float64
	AvgDB []float64
	MaxDB []float64
	MinDB []float64

	// Frames is how many transforms went into the average.
	Frames int

	// NoiseBW is the equivalent noise bandwidth of one bin in Hz, which is
	// rate*sum(w^2)/sum(w)^2 for the analysis window. The levels are per bin
	// rather than per hertz, matching Spectrum and the spectrogram's color bar;
	// this is the number that converts them.
	NoiseBW float64

	// Floor is the decibel floor the levels are clipped at.
	Floor float64

	Warnings []string
}

// PSD returns the averaged spectrum accumulated alongside the spectrogram. It
// may be called at any point, and again after Close.
//
// The frames are averaged as linear power, which is why this is accumulated
// here: the Spectrogram's rows are decibels, and averaging decibels is a
// geometric mean of power, putting noise about 2.5 dB below its true mean while
// leaving a tone alone.
func (b *SpectrogramBuilder) PSD() *PSD {
	p := &PSD{
		Rate:   b.rate,
		Center: b.center,
		Freq:   slices.Clone(b.freqs),
		Frames: b.nframes,
		Floor:  b.floor,
	}
	if b.gain > 0 {
		p.NoiseBW = b.rate * b.gain2 / (b.gain * b.gain)
	}
	n := len(b.freqs)
	p.AvgDB = make([]float64, n)
	p.MaxDB = make([]float64, n)
	p.MinDB = make([]float64, n)
	if b.nframes == 0 {
		for i := range n {
			p.AvgDB[i], p.MaxDB[i], p.MinDB[i] = b.floor, b.floor, b.floor
		}
		p.Warnings = append(p.Warnings, "no whole frame was written; the spectrum is empty")
		return p
	}
	inv := 1 / float64(b.nframes)
	for i := range n {
		p.AvgDB[i] = b.floorDB(dbPower(b.powSum[i] * inv))
		p.MaxDB[i] = b.floorDB(dbPower(b.powHi[i]))
		p.MinDB[i] = b.floorDB(dbPower(b.powLo[i]))
	}
	return p
}

// floorDB clips a level from below without touching the spectrogram's peak.
func (b *SpectrogramBuilder) floorDB(db float64) float64 {
	if db < b.floor || math.IsNaN(db) {
		return b.floor
	}
	return db
}

// PSDOf is the whole-signal form: it averages the spectrum of x and throws the
// spectrogram away.
func PSDOf(x []float64, rate float64, opt STFTOptions) (*PSD, error) {
	b, err := NewSpectrogramBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	// One column is all the spectrogram this needs, so a long signal costs
	// nothing beyond the bins themselves.
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return psdResult(b)
}

// PSDOfComplex is PSDOf for an I/Q signal, giving a two-sided spectrum.
func PSDOfComplex(x []complex128, rate float64, opt STFTOptions) (*PSD, error) {
	opt.Complex = true
	b, err := NewSpectrogramBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	if err := b.WriteComplex(x); err != nil {
		return nil, err
	}
	return psdResult(b)
}

func psdResult(b *SpectrogramBuilder) (*PSD, error) {
	p := b.PSD()
	if p.Frames == 0 {
		return nil, fmt.Errorf("dspviz: fewer samples than one %d point frame", b.size)
	}
	return p, nil
}

// TwoSided reports whether the spectrum covers negative frequencies, which is
// what a complex signal's does. It is derived from Freq rather than stored, so
// it cannot disagree with them.
func (p *PSD) TwoSided() bool { return len(p.Freq) > 0 && p.Freq[0] < 0 }

// MeanSquare returns the mean square of the signal the spectrum was measured
// from, recovered from the average curve.
//
// This is Parseval's theorem in this package's own scaling, and the check that
// the window normalization is right -- nothing on a plot would reveal a wrong
// one, which scales every level equally. The equivalent noise bandwidth
// converts a sum over bins back into total power, for a tone as much as noise.
func (p *PSD) MeanSquare() float64 {
	if p.Frames == 0 || p.NoiseBW <= 0 || len(p.Freq) < 2 {
		return 0
	}
	var sum float64
	for _, db := range p.AvgDB {
		sum += math.Pow(10, db/10)
	}
	binHz := p.Freq[1] - p.Freq[0]
	bins := p.NoiseBW / binHz // the equivalent noise bandwidth, in bins
	if p.TwoSided() {
		// Every bin is its own frequency, so there is no mirror to account for.
		return sum / bins
	}
	// A one-sided level already carries the mirror bin's share, which is the
	// factor of two that comes back out here.
	return sum / (2 * bins)
}

// PeakBin returns the loudest bin of the average curve and its level, ignoring
// the skip bins on either side of zero frequency; skip of zero ignores none.
//
// The skip keeps a two-sided spectrum's DC spur, a receiver artifact rather
// than a signal, out of the answer. It finds zero by frequency because FFTShift
// puts it in the middle of the array; skipping by index would drop the band
// edges instead.
func (p *PSD) PeakBin(skip int) (bin int, db float64) {
	bin, db = -1, math.Inf(-1)
	dc := -1
	if skip > 0 {
		dc = slices.Index(p.Freq, 0)
	}
	for i, v := range p.AvgDB {
		if dc >= 0 && i >= dc-skip && i <= dc+skip {
			continue
		}
		if v > db {
			bin, db = i, v
		}
	}
	return bin, db
}

// NoiseFloorDB estimates the noise floor as a percentile of the average curve,
// in dBFS per bin. A percentile below the middle is robust to any number of
// tones; a mean over the whole spectrum is pulled up by whatever signal is in
// it.
func (p *PSD) NoiseFloorDB(percentile float64) float64 {
	if len(p.AvgDB) == 0 {
		return math.Inf(-1)
	}
	s := slices.Clone(p.AvgDB)
	slices.Sort(s)
	i := int(percentile / 100 * float64(len(s)-1))
	return s[max(0, min(i, len(s)-1))]
}

// Flatness returns the spectral flatness, or Wiener entropy: the geometric mean
// of the power spectrum over its arithmetic mean, in [0, 1].
//
// One is white noise and zero is a pure tone, so it is the single number that
// says whether a capture holds a signal at all. DC is left out, since an offset
// is not a spectrum.
func (p *PSD) Flatness() float64 {
	var logSum, sum float64
	var n int
	for i, db := range p.AvgDB {
		if p.Freq[i] == 0 {
			continue
		}
		pw := math.Pow(10, db/10)
		if pw <= 0 {
			continue
		}
		logSum += math.Log(pw)
		sum += pw
		n++
	}
	if n == 0 || sum == 0 {
		return 0
	}
	return math.Exp(logSum/float64(n)) / (sum / float64(n))
}

// OccupiedBandwidth returns the band holding the given fraction of the total
// power, with the rest split evenly between the two ends. The 99% figure is the
// conventional one for saying how wide a transmission is.
func OccupiedBandwidth(p *PSD, fraction float64) (lo, hi float64) {
	if len(p.Freq) < 2 || fraction <= 0 || fraction > 1 {
		return 0, 0
	}
	pw := make([]float64, len(p.AvgDB))
	var total float64
	for i, db := range p.AvgDB {
		pw[i] = math.Pow(10, db/10)
		total += pw[i]
	}
	if total <= 0 {
		return 0, 0
	}
	tail := (1 - fraction) / 2 * total
	var run float64
	lo, hi = p.Freq[0], p.Freq[len(p.Freq)-1]
	for i, v := range pw {
		run += v
		if run >= tail {
			lo = p.Freq[i]
			break
		}
	}
	run = 0
	for i, p0 := range slices.Backward(pw) {
		run += p0
		if run >= tail {
			hi = p.Freq[i]
			break
		}
	}
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo, hi
}

// BandwidthAtDB returns how far either side of the loudest bin the average
// curve stays within down decibels of it, which is the -3, -20 or -60 dB width
// of whatever is strongest in the capture.
func BandwidthAtDB(p *PSD, down float64) (lo, hi float64) {
	bin, peak := p.PeakBin(0)
	if bin < 0 {
		return 0, 0
	}
	target := peak - math.Abs(down)
	lo, hi = p.Freq[bin], p.Freq[bin]
	for i := bin; i >= 0 && p.AvgDB[i] >= target; i-- {
		lo = p.Freq[i]
	}
	for i := bin; i < len(p.AvgDB) && p.AvgDB[i] >= target; i++ {
		hi = p.Freq[i]
	}
	return lo, hi
}

// PSDChart draws the three curves, with the estimated noise floor as a dashed
// line across them.
func PSDChart(p *PSD, opt ChartOptions) *Chart {
	if len(p.Freq) == 0 {
		// No frequency axis to label. NewSpectrogramBuilder rejects the frame
		// size that produces this, so it takes a hand-built PSD to get here;
		// an empty chart beats indexing Freq[0].
		return opt.apply(&Chart{}, "Averaged spectrum")
	}
	floor := floorOr(opt.FloorDB, math.Max(p.Floor, minOf(p.MinDB)-6))
	noise := p.NoiseFloorDB(20)
	flat := make([]float64, len(p.Freq))
	for i := range flat {
		flat[i] = noise
	}

	lo, hi := OccupiedBandwidth(p, 0.99)
	c := &Chart{
		Subtitle: fmt.Sprintf("%d frames, %.2f Hz of noise bandwidth per bin, noise floor %.1f dBFS, 99%% of the power in %.0f Hz",
			p.Frames, p.NoiseBW, noise, hi-lo),
		X: Axis{Label: hzLabel, Min: p.Freq[0], Max: p.Freq[len(p.Freq)-1]},
		Y: Axis{Label: "level (dBFS per bin)", Min: floorTo(floor, 10), Max: ceilTo(maxOf(p.MaxDB)+6, 10)},
		Series: []Series{
			{Name: "peak", X: p.Freq, Y: p.MaxDB, Width: 1},
			{Name: "average", X: p.Freq, Y: p.AvgDB, Width: 1.5},
			{Name: "minimum", X: p.Freq, Y: p.MinDB, Width: 1},
			{Name: "noise floor", X: p.Freq, Y: flat, Width: 1, Dashed: true},
		},
		Notes: append(slices.Clone(p.Warnings),
			fmt.Sprintf("levels are per bin, not per hertz; one bin is %.3f Hz of noise bandwidth", p.NoiseBW)),
	}
	if p.TwoSided() {
		c.X.Label = "frequency offset (Hz)"
	}
	title := "Averaged spectrum"
	if p.Center != 0 {
		title = "Averaged spectrum, center " + hzString(p.Center)
	}
	return opt.apply(c, title)
}
