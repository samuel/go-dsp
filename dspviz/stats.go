package dspviz

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/bits"
)

// minClipRun is how many samples in a row have to sit on the rail before it is
// clipping rather than a peak that happens to touch full scale. A
// peak-normalized file has one sample at the rail by construction, so a bare
// count of railed samples reports every such file as clipped.
const minClipRun = 3

// StatsOptions tunes what the statistics accumulate.
type StatsOptions struct {
	Rate float64

	// Bits is the container's bits per sample, which the effective bit depth
	// estimate needs to recover the integer codes. Zero skips that estimate,
	// which is what a float file gets.
	Bits int

	// SilenceDB is the level a window has to stay below to count as silent.
	// Zero selects -60.
	SilenceDB float64

	// SilenceWindow is how long that window is, in seconds. Zero selects 20 ms.
	// Silence is measured over a window rather than per sample because every
	// waveform passes through zero.
	SilenceWindow float64

	// Complex accumulates I/Q, which adds the imbalance figures and measures the
	// amplitude statistics on the magnitude.
	Complex bool
}

// SignalStats is what a single pass over a recording can say about it, without
// any transform.
type SignalStats struct {
	Rate     float64 `json:"rate"`
	Samples  int64   `json:"samples"`
	Duration float64 `json:"duration"`

	// Complex says the amplitude figures were measured on the magnitude of an
	// I/Q signal, which has no waveform. DC and the zero-crossing figures are
	// then meaningless and are left at zero rather than filled in with a number
	// that would be read as one; IQ carries the offsets that do mean something.
	Complex bool `json:"complex,omitempty"`

	DC     float64 `json:"dc"`
	RMS    float64 `json:"rms"`
	Peak   float64 `json:"peak"`
	Crest  float64 `json:"crest"`
	RMSDB  float64 `json:"rmsDB"`
	PeakDB float64 `json:"peakDB"`

	// Clipped counts every sample sitting on a rail, ClippedRuns the runs they
	// fall in, and SustainedRuns only the runs at least minClipRun long. The
	// last is the one that means clipping; the first two are what it is read
	// from.
	Clipped       int64 `json:"clipped"`
	ClippedRuns   int   `json:"clippedRuns"`
	SustainedRuns int   `json:"sustainedRuns"`

	// EffectiveBits is how many bits of the container the signal actually uses,
	// from the trailing zeros of the integer codes. It explains a noise floor
	// well above expectation in one line, but any processing at all -- a
	// mixdown, a gain change, a resample -- fills the low bits in.
	EffectiveBits int    `json:"effectiveBits"`
	BitsNote      string `json:"bitsNote,omitempty"`

	ZeroCrossings   int64   `json:"zeroCrossings"`
	ZeroCrossRateHz float64 `json:"zeroCrossRateHz"`

	SilentFraction float64 `json:"silentFraction"`
	LongestSilence float64 `json:"longestSilence"`

	IQ *IQStats `json:"iq,omitempty"`

	// Spectral is filled in from a PSD measured in the same pass. It is
	// separate because everything above needs no transform at all.
	Spectral *SpectralStats `json:"spectral,omitempty"`

	Notes []string `json:"notes,omitempty"`
}

// StatsBuilder accumulates SignalStats from blocks of samples.
type StatsBuilder struct {
	opt      StatsOptions
	clipAt   float64
	scale    float64 // codes per unit amplitude, or 0 for a float format
	silentAt float64
	silWin   int

	n                  int64
	sum, sumSq, peak   float64
	clipped            int64
	runs, sustained    int
	inRun              int
	crossings          int64
	prevPos, havePrev  bool
	codeOr             uint64
	codesIntegral      bool
	silSum             float64
	silHave            int
	silCount, silTotal int64
	silRun, silRunBest int64
	cross              crossSums
}

// NewStatsBuilder starts a statistics pass.
func NewStatsBuilder(opt StatsOptions) (*StatsBuilder, error) {
	if opt.Rate <= 0 {
		return nil, fmt.Errorf("dspviz: a sample rate of %g", opt.Rate)
	}
	b := &StatsBuilder{opt: opt, clipAt: 1, codesIntegral: true}
	if opt.Bits > 1 {
		full := math.Ldexp(1, opt.Bits-1)
		b.scale = full
		// The positive rail is one step short of +1, so a threshold there
		// catches both ends.
		b.clipAt = (full - 1) / full
	}
	db := opt.SilenceDB
	if db == 0 {
		db = -60
	}
	b.silentAt = Amplitude(db)
	sec := opt.SilenceWindow
	if sec <= 0 {
		sec = 0.02
	}
	b.silWin = max(1, int(math.Round(sec*opt.Rate)))
	return b, nil
}

// StatsOf is the whole-signal form.
func StatsOf(x []float64, opt StatsOptions) (*SignalStats, error) {
	b, err := NewStatsBuilder(opt)
	if err != nil {
		return nil, err
	}
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return b.Close()
}

// StatsOfComplex is StatsOf for an I/Q signal. Options.Complex is set for the
// caller; analyzing complex samples as real would silently misread them.
func StatsOfComplex(z []complex128, opt StatsOptions) (*SignalStats, error) {
	opt.Complex = true
	b, err := NewStatsBuilder(opt)
	if err != nil {
		return nil, err
	}
	if err := b.WriteComplex(z); err != nil {
		return nil, err
	}
	return b.Close()
}

// Reset clears the accumulated state.
func (b *StatsBuilder) Reset() {
	// The options were validated when this builder was made.
	n, err := NewStatsBuilder(b.opt)
	if err != nil {
		return
	}
	*b = *n
}

// Write adds real samples.
func (b *StatsBuilder) Write(x []float64) error {
	for _, v := range x {
		b.amplitude(v)
		b.rail(math.Abs(v))
		b.sign(v)
		b.code(v)
	}
	return nil
}

// WriteComplex adds I/Q samples. Peak and RMS are measured on the magnitude,
// which is what a complex signal has instead of a waveform, while clipping is
// measured on the two components separately -- see rail. The three cross sums
// come along for the imbalance figures.
func (b *StatsBuilder) WriteComplex(x []complex128) error {
	for _, z := range x {
		i, q := real(z), imag(z)
		b.amplitude(math.Hypot(i, q))
		b.rail(math.Max(math.Abs(i), math.Abs(q)))
		b.code(i)
		b.code(q)
		b.cross.add(i, q)
	}
	return nil
}

// amplitude accumulates everything that depends only on the size of a sample.
// The rail test is rail's, because for a complex signal the two are measured on
// different numbers.
func (b *StatsBuilder) amplitude(v float64) {
	b.n++
	b.sum += v
	b.sumSq += v * v
	b.peak = math.Max(b.peak, math.Abs(v))

	b.silSum += v * v
	b.silHave++
	if b.silHave == b.silWin {
		b.silTotal++
		if math.Sqrt(b.silSum/float64(b.silHave)) < b.silentAt {
			b.silCount++
			b.silRun++
			b.silRunBest = max(b.silRunBest, b.silRun)
		} else {
			b.silRun = 0
		}
		b.silSum, b.silHave = 0, 0
	}
}

// rail counts the samples sitting on the full-scale rail and their runs. a is
// a sample's largest component: its own magnitude for a real sample, the larger
// of |I| and |Q| for an I/Q pair.
//
// The magnitude would be the wrong basis. A converter runs out of codes per
// component, not per magnitude: I and Q at 0.99 are inside the rails while
// their magnitude is 1.40, so measuring on the magnitude flags an ordinary
// capture as clipped.
func (b *StatsBuilder) rail(a float64) {
	if a >= b.clipAt {
		b.clipped++
		b.inRun++
		if b.inRun == 1 {
			b.runs++
		}
		if b.inRun == minClipRun {
			b.sustained++
		}
	} else {
		b.inRun = 0
	}
}

func (b *StatsBuilder) sign(v float64) {
	pos := v >= 0
	if b.havePrev && pos != b.prevPos {
		b.crossings++
	}
	b.prevPos, b.havePrev = pos, true
}

// code folds a sample back into the integer it was read from, so the trailing
// zeros of every code together say how many bits the signal really uses.
func (b *StatsBuilder) code(v float64) {
	if b.scale == 0 {
		return
	}
	c := v * b.scale
	r := math.Round(c)
	if math.Abs(c-r) > 1e-6 {
		b.codesIntegral = false
		return
	}
	b.codeOr |= uint64(int64(r))
}

// Close returns the statistics. An analysis with no samples is an error rather
// than a document of zeros, as it is for every other builder here.
func (b *StatsBuilder) Close() (*SignalStats, error) {
	if b.n == 0 {
		return nil, errors.New("dspviz: no samples")
	}
	s := &SignalStats{
		Rate:          b.opt.Rate,
		Complex:       b.opt.Complex,
		Samples:       b.n,
		Clipped:       b.clipped,
		ClippedRuns:   b.runs,
		SustainedRuns: b.sustained,
		ZeroCrossings: b.crossings,
	}
	s.Duration = float64(b.n) / b.opt.Rate
	if !b.opt.Complex {
		// b.sum accumulated magnitudes for a complex signal, so its mean is not
		// an offset and must not be published as one. IQ.DCI and IQ.DCQ carry
		// the two offsets that do mean something.
		s.DC = b.sum / float64(b.n)
		s.ZeroCrossRateHz = float64(b.crossings) / s.Duration
	}
	s.RMS = math.Sqrt(b.sumSq / float64(b.n))
	s.Peak = b.peak
	s.RMSDB = DB(s.RMS)
	s.PeakDB = DB(s.Peak)
	if s.RMS > 0 {
		s.Crest = s.Peak / s.RMS
	}
	if b.silTotal > 0 {
		s.SilentFraction = float64(b.silCount) / float64(b.silTotal)
		s.LongestSilence = float64(b.silRunBest*int64(b.silWin)) / b.opt.Rate
	}

	switch {
	case b.scale == 0:
		s.BitsNote = "a float format carries no integer codes to count"
	case !b.codesIntegral:
		s.BitsNote = "the samples are not on the container's code grid, so this is not an estimate of anything"
	case b.codeOr == 0:
		s.BitsNote = "every sample is zero"
	default:
		s.EffectiveBits = b.opt.Bits - bits.TrailingZeros64(b.codeOr)
		if s.EffectiveBits < b.opt.Bits {
			s.BitsNote = fmt.Sprintf("the low %d bits of every sample are zero, so the noise floor is %.0f dB above the container's",
				b.opt.Bits-s.EffectiveBits, 6.02*float64(b.opt.Bits-s.EffectiveBits))
		}
	}

	if b.opt.Complex {
		s.IQ = b.iq()
	}
	return s, nil
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(v, hi)) }

// There is deliberately no check for interleaved I/Q read as one real channel:
// the third mistake worth catching and the only one this package does not.
// Alternating I and Q gives a transform A(2w) + exp(-jw)B(2w), whose halves add
// at one image and cancel at the other, so a complex exponential comes out as a
// single clean tone at rate/2 - f -- indistinguishable from a real recording.
// Workable signatures depend on the receiver's imbalance, a property of the
// hardware rather than of the mistake, and a check that fires on some receivers
// reads as a clean bill of health when it stays silent on others.

// WriteJSON writes the same numbers as JSON, from this struct's own tags; see
// json.go for why they are sanitized on the way out.
func (s *SignalStats) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sanitize(s))
}
