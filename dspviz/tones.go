package dspviz

import (
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/samuel/go-dsp/dsp"
)

// ToneTrackOptions describes the two-tone decision a demodulator is making.
type ToneTrackOptions struct {
	// Mark and Space are the two tone frequencies in Hz.
	Mark, Space float64

	// Baud sets the block length to one symbol, which is what an FSK
	// demodulator integrates over. Block overrides it.
	Baud  float64
	Block int

	// Hop is how many samples pass between decisions. Zero selects Block/4.
	Hop int

	// Threshold is the decoder's own threshold on the difference of the two
	// powers, drawn on the chart as a pair of lines. It is the number this whole
	// view exists to check: a threshold on the wrong scale is invisible in a
	// decoder that simply never emits a bit.
	Threshold float64

	// Columns bounds the result. Zero selects 2000.
	Columns int
}

// ToneTrack is a two-tone detector's decision variable against time.
//
// It measures the demodulator rather than the signal: the powers come from
// dsp.Goertzel at the frequencies and block length a decoder would use, so what
// is plotted is the number that decoder is thresholding. Tones in the wrong
// place, a threshold on the wrong scale and a bit clock that never locks are
// three different pictures here, and identical from outside.
type ToneTrack struct {
	Rate        float64
	Mark, Space float64
	Block       int
	Hop         int
	Threshold   float64

	Times []float64
	// MarkPower and SpacePower are the Goertzel powers, unnormalized, exactly as
	// a decoder reads them. Diff is MarkPower - SpacePower.
	MarkPower  []float64
	SpacePower []float64
	Diff       []float64

	// Decisions is how many blocks were examined, Crossings how many of them
	// had a decisive difference, and MaxAbsDiff the largest seen. A Crossings
	// of zero against a large MaxAbsDiff is a threshold on the wrong scale.
	Decisions  int64
	Crossings  int64
	MaxAbsDiff float64
	// Suggested is the quarter point of the decision variable's distribution, so
	// a threshold there is crossed three blocks in four -- a starting point when
	// the one in the decoder turns out to be on the wrong scale entirely.
	Suggested   float64
	Transitions int64 // decisive blocks whose sign differed from the last

	Warnings []string
}

// ToneTrackBuilder computes a ToneTrack from a signal handed over a piece at a
// time.
type ToneTrackBuilder struct {
	rate  float64
	opt   ToneTrackOptions
	block int
	hop   int

	g    *dsp.Goertzel[float64]
	ring []float64
	have int
	drop int64

	f     fold[[3]float64]
	start int64

	decisions   int64
	crossings   int64
	transitions int64
	maxAbs      float64
	lastSign    int
	mag         [decadeBuckets]int64
}

// decadeBuckets covers ten decades of decision variable at a tenth of a decade
// each, which is enough resolution to suggest a threshold and cheap enough to
// keep for a capture of any length.
const decadeBuckets = 100

// NewToneTrackBuilder starts a tone track.
func NewToneTrackBuilder(rate float64, opt ToneTrackOptions) (*ToneTrackBuilder, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("dspviz: a sample rate of %g", rate)
	}
	if opt.Mark <= 0 || opt.Space <= 0 {
		return nil, errors.New("dspviz: a tone track needs two tone frequencies")
	}
	block := opt.Block
	if block <= 0 {
		if opt.Baud <= 0 {
			return nil, errors.New("dspviz: a tone track needs a baud rate or a block length")
		}
		block = int(math.Round(rate / opt.Baud))
	}
	if block < 2 {
		return nil, fmt.Errorf("dspviz: a block of %d samples is shorter than a cycle of anything", block)
	}
	hop := opt.Hop
	if hop <= 0 {
		hop = max(1, block/4)
	}
	cols := opt.Columns
	if cols <= 0 {
		cols = 2000
	}

	g, err := dsp.NewGoertzel[float64]([]float64{opt.Mark, opt.Space}, rate, block)
	if err != nil {
		return nil, err
	}

	b := &ToneTrackBuilder{
		rate: rate, opt: opt, block: block, hop: hop, g: g,
		ring: make([]float64, block),
		// The more decisive of two blocks survives a fold, so a reduced track
		// still shows the largest excursion the decoder ever saw.
		f: newFold(cols, func(a [3]float64, _ int64, c [3]float64, _ int64) [3]float64 {
			if math.Abs(a[2]) >= math.Abs(c[2]) {
				return a
			}
			return c
		}),
	}
	return b, nil
}

// Reset clears the accumulated decisions, so the builder starts a new signal.
func (b *ToneTrackBuilder) Reset() {
	// The options were validated when this builder was made.
	n, err := NewToneTrackBuilder(b.rate, b.opt)
	if err != nil {
		return
	}
	*b = *n
}

// Write adds samples.
func (b *ToneTrackBuilder) Write(x []float64) error {
	for len(x) > 0 {
		if b.drop > 0 {
			n := min(b.drop, int64(len(x)))
			x = x[n:]
			b.drop -= n
			continue
		}
		n := copy(b.ring[b.have:b.block], x)
		b.have += n
		x = x[n:]
		if b.have < b.block {
			return nil
		}
		b.decide()
		b.start += int64(b.hop)
		if b.hop < b.have {
			copy(b.ring, b.ring[b.hop:b.have])
			b.have -= b.hop
		} else {
			b.drop = int64(b.hop - b.have)
			b.have = 0
		}
	}
	return nil
}

func (b *ToneTrackBuilder) decide() {
	b.g.Reset()
	b.g.Feed(b.ring[:b.block])
	p := b.g.Power()
	mark, space := p[0], p[1]
	diff := mark - space

	b.decisions++
	a := math.Abs(diff)
	b.maxAbs = math.Max(b.maxAbs, a)
	if a > 0 {
		// A tenth of a decade per bucket, centered so that 1.0 lands in the
		// middle of the range.
		i := int(math.Round(dbPower(a))) + decadeBuckets/2
		b.mag[max(0, min(i, decadeBuckets-1))]++
	}
	if a > b.opt.Threshold {
		b.crossings++
		s := 1
		if diff < 0 {
			s = -1
		}
		if b.lastSign != 0 && s != b.lastSign {
			b.transitions++
		}
		b.lastSign = s
	}

	b.f.add([3]float64{mark, space, diff}, float64(b.start)/b.rate)
}

// Close finishes the track.
func (b *ToneTrackBuilder) Close() (*ToneTrack, error) {
	if len(b.f.rows) == 0 {
		return nil, fmt.Errorf("dspviz: fewer samples than one %d sample block", b.block)
	}
	t := &ToneTrack{
		Rate: b.rate, Mark: b.opt.Mark, Space: b.opt.Space,
		Block: b.block, Hop: b.hop, Threshold: b.opt.Threshold,
		Times:       b.f.times,
		Decisions:   b.decisions,
		Crossings:   b.crossings,
		Transitions: b.transitions,
		MaxAbsDiff:  b.maxAbs,
		Suggested:   b.percentile(0.25),
	}
	n := len(b.f.rows)
	t.MarkPower = make([]float64, n)
	t.SpacePower = make([]float64, n)
	t.Diff = make([]float64, n)
	for i, r := range b.f.rows {
		t.MarkPower[i], t.SpacePower[i], t.Diff[i] = r[0], r[1], r[2]
	}

	switch {
	case b.opt.Threshold > 0 && b.crossings == 0:
		t.Warnings = append(t.Warnings, fmt.Sprintf(
			"the difference never reaches the threshold of %g; the largest was %.4g, so no bit was ever emitted",
			b.opt.Threshold, b.maxAbs))
	case b.opt.Threshold > 0 && b.crossings == b.decisions:
		t.Warnings = append(t.Warnings, "every block is decisive, so the threshold is not doing anything")
	}
	if b.transitions == 0 && b.crossings > 0 {
		t.Warnings = append(t.Warnings, "the decision never changes sign: one tone is present and the other is not")
	}
	return t, nil
}

// percentile reads a fraction off the magnitude histogram, in the units of the
// decision variable.
func (b *ToneTrackBuilder) percentile(f float64) float64 {
	var total int64
	for _, c := range b.mag {
		total += c
	}
	if total == 0 {
		return 0
	}
	// At least one sample: a fraction of a small total rounds to zero, and a
	// cumulative count is at least zero from the first bucket, so the answer
	// would be the bottom of the histogram whether or not anything is in it.
	want := max(int64(f*float64(total)), 1)
	var run int64
	for i, c := range b.mag {
		run += c
		if run >= want {
			return math.Pow(10, float64(i-decadeBuckets/2)/10)
		}
	}
	return b.maxAbs
}

// ToneTrackOf is the whole-signal form.
func ToneTrackOf(x []float64, rate float64, opt ToneTrackOptions) (*ToneTrack, error) {
	b, err := NewToneTrackBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return b.Close()
}

// ToneTrackChart draws the two powers and the difference against time, with the
// threshold across them.
//
// The axis is linear and in the detector's own units on purpose. A decibel axis
// would hide the failure this exists for: a threshold set on one scale against a
// signal on another does not look wrong in decibels.
func ToneTrackChart(t *ToneTrack, opt ChartOptions) *Chart {
	series := []Series{
		{Name: fmt.Sprintf("mark %.0f Hz", t.Mark), X: t.Times, Y: t.MarkPower, Width: 1},
		{Name: fmt.Sprintf("space %.0f Hz", t.Space), X: t.Times, Y: t.SpacePower, Width: 1},
		{Name: "difference", X: t.Times, Y: t.Diff, Width: 1.5},
	}
	if t.Threshold > 0 {
		hi := make([]float64, len(t.Times))
		lo := make([]float64, len(t.Times))
		for i := range hi {
			hi[i], lo[i] = t.Threshold, -t.Threshold
		}
		series = append(series,
			Series{Name: "threshold", X: t.Times, Y: hi, Width: 1, Dashed: true},
			Series{X: t.Times, Y: lo, Width: 1, Dashed: true})
	}

	c := &Chart{
		Subtitle: fmt.Sprintf("%d sample blocks, %d of %d decisive, largest difference %.4g",
			t.Block, t.Crossings, t.Decisions, t.MaxAbsDiff),
		X:      Axis{Label: "time (s)", Min: 0, Max: timeAxisEnd(t.Times, float64(t.Block)/t.Rate)},
		Y:      Axis{Label: "Goertzel power"},
		Series: series,
		Notes:  slices.Clone(t.Warnings),
	}
	return opt.apply(c, "Tone pair decision")
}

// TonePeak is one narrow peak in an averaged spectrum.
type TonePeak struct {
	Hz    float64
	DB    float64
	SNRDB float64
}

// TonePeaks returns the n strongest narrow peaks in a spectrum, no two of them
// within minHz of each other.
//
// It is how the tones a capture actually holds are found, as against the ones a
// command line claimed: the spacing of the top two is the FSK shift.
func TonePeaks(p *PSD, n int, minHz float64) []TonePeak {
	if p == nil || len(p.AvgDB) < 3 {
		return nil
	}
	floor := p.NoiseFloorDB(20)
	var peaks []TonePeak
	for i := 1; i < len(p.AvgDB)-1; i++ {
		if p.AvgDB[i] < p.AvgDB[i-1] || p.AvgDB[i] < p.AvgDB[i+1] {
			continue
		}
		peaks = append(peaks, TonePeak{Hz: p.Freq[i], DB: p.AvgDB[i], SNRDB: p.AvgDB[i] - floor})
	}
	slices.SortFunc(peaks, func(a, b TonePeak) int {
		switch {
		case a.DB > b.DB:
			return -1
		case a.DB < b.DB:
			return 1
		}
		return 0
	})

	var out []TonePeak
	for _, pk := range peaks {
		if len(out) == n {
			break
		}
		near := false
		for _, o := range out {
			if math.Abs(o.Hz-pk.Hz) < minHz {
				near = true
				break
			}
		}
		if !near {
			out = append(out, pk)
		}
	}
	return out
}

// WriteSummary prints what the track says, and what the spectrum it was measured
// alongside says the tones really are.
func (t *ToneTrack) WriteSummary(w io.Writer, peaks []TonePeak) error {
	var b strings.Builder
	fmt.Fprintf(&b, "tones         mark %.1f Hz, space %.1f Hz, shift %.1f Hz\n",
		t.Mark, t.Space, math.Abs(t.Mark-t.Space))
	fmt.Fprintf(&b, "block         %d samples (%.2f ms), hop %d\n",
		t.Block, float64(t.Block)/t.Rate*1000, t.Hop)
	fmt.Fprintf(&b, "decisions     %d blocks, %d over the threshold of %g, %d sign changes\n",
		t.Decisions, t.Crossings, t.Threshold, t.Transitions)
	fmt.Fprintf(&b, "difference    largest %.4g, a threshold of %.4g would be crossed three times in four\n",
		t.MaxAbsDiff, t.Suggested)

	if len(peaks) > 0 {
		for i, pk := range peaks {
			fmt.Fprintf(&b, "peak %d        %.1f Hz at %.1f dBFS, %.1f dB over the noise\n",
				i+1, pk.Hz, pk.DB, pk.SNRDB)
		}
		if len(peaks) >= 2 {
			lo, hi := peaks[0].Hz, peaks[1].Hz
			if lo > hi {
				lo, hi = hi, lo
			}
			fmt.Fprintf(&b, "measured      shift %.1f Hz; try -mark %.0f -space %.0f\n", hi-lo, lo, hi)
		}
	}
	for _, w := range t.Warnings {
		fmt.Fprintf(&b, "note          %s\n", w)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
