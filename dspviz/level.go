package dspviz

import (
	"errors"
	"fmt"
	"math"
)

// LevelOptions tunes a level meter.
type LevelOptions struct {
	// WindowSec is the integration time in seconds. Zero selects 50 ms, which
	// is short enough to see a syllable and long enough that the RMS of a low
	// tone is a level rather than a ripple.
	WindowSec float64

	// Columns bounds the result the way STFTOptions.Columns bounds a
	// spectrogram. Zero selects 1800.
	Columns int
}

// Level is RMS, true peak and crest factor against time, all in decibels.
//
// It is kept separate from the waveform rather than drawn on it, because a
// linear amplitude axis cannot show the structure 60 dB down that a decibel axis
// makes obvious -- which is how you find where the interesting two hundred
// milliseconds of a thirty minute capture are.
type Level struct {
	Rate      float64
	WindowSec float64
	Times     []float64
	RMSDB     []float64
	PeakDB    []float64
	CrestDB   []float64
}

// LevelBuilder computes a Level from a signal handed over a piece at a time.
type LevelBuilder struct {
	rate float64
	opt  LevelOptions
	win  int
	f    fold[[2]float64] // {mean square, peak}

	sumSq float64
	peak  float64
	have  int
	start int64
}

// NewLevelBuilder starts a level meter.
func NewLevelBuilder(rate float64, opt LevelOptions) (*LevelBuilder, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("dspviz: a sample rate of %g", rate)
	}
	sec := opt.WindowSec
	if sec <= 0 {
		sec = 0.05
	}
	win := max(1, int(math.Round(sec*rate)))
	cols := opt.Columns
	if cols <= 0 {
		cols = 1800
	}
	return &LevelBuilder{
		rate: rate,
		opt:  opt,
		win:  win,
		// Windows merge into one covering both: the mean square of the pair is
		// the weighted mean of theirs, and its peak is the greater of theirs.
		// Neither is an approximation, which is why a reduced level plot reads
		// the same as an unreduced one -- and the weights are why the merge is
		// handed the row counts at all.
		f: newFold(cols, func(a [2]float64, an int64, b [2]float64, bn int64) [2]float64 {
			n := float64(an + bn)
			return [2]float64{
				(a[0]*float64(an) + b[0]*float64(bn)) / n,
				math.Max(a[1], b[1]),
			}
		}),
	}, nil
}

// Reset clears the accumulated windows, so the builder starts a new signal.
func (b *LevelBuilder) Reset() {
	// The options were validated when this builder was made.
	n, err := NewLevelBuilder(b.rate, b.opt)
	if err != nil {
		return
	}
	*b = *n
}

// Write adds samples.
func (b *LevelBuilder) Write(x []float64) error {
	for _, v := range x {
		b.sumSq += v * v
		b.peak = math.Max(b.peak, math.Abs(v))
		b.have++
		if b.have == b.win {
			b.emit()
		}
	}
	return nil
}

// WriteComplex adds the magnitude of I/Q samples.
func (b *LevelBuilder) WriteComplex(x []complex128) error {
	for _, z := range x {
		m := math.Hypot(real(z), imag(z))
		b.sumSq += m * m
		b.peak = math.Max(b.peak, m)
		b.have++
		if b.have == b.win {
			b.emit()
		}
	}
	return nil
}

func (b *LevelBuilder) emit() {
	b.f.add([2]float64{b.sumSq / float64(b.have), b.peak}, float64(b.start)/b.rate)
	b.start += int64(b.have)
	b.sumSq, b.peak, b.have = 0, 0, 0
}

// Close finishes the meter. A partial last window is dropped rather than
// reported as a quiet one, which is what it would look like.
func (b *LevelBuilder) Close() (*Level, error) {
	if len(b.f.rows) == 0 {
		return nil, errors.New("dspviz: fewer samples than one integration window")
	}
	l := &Level{
		Rate:      b.rate,
		WindowSec: float64(b.win) / b.rate,
		Times:     b.f.times,
	}
	n := len(b.f.rows)
	l.RMSDB = make([]float64, n)
	l.PeakDB = make([]float64, n)
	l.CrestDB = make([]float64, n)
	for i, r := range b.f.rows {
		rms := DB(math.Sqrt(r[0]))
		pk := DB(r[1])
		l.RMSDB[i], l.PeakDB[i], l.CrestDB[i] = rms, pk, pk-rms
	}
	return l, nil
}

// LevelOf is the whole-signal form.
func LevelOf(x []float64, rate float64, opt LevelOptions) (*Level, error) {
	b, err := NewLevelBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return b.Close()
}

// LevelChart draws the three curves on a decibel axis.
func LevelChart(l *Level, opt ChartOptions) *Chart {
	floor := floorFor(opt.FloorDB, -120, minOf(l.RMSDB))

	c := &Chart{
		Subtitle: fmt.Sprintf("%.0f ms windows, peak %.2f dBFS, loudest RMS %.2f dBFS",
			l.WindowSec*1000, maxOf(l.PeakDB), maxOf(l.RMSDB)),
		X: Axis{Label: "time (s)", Min: 0, Max: timeAxisEnd(l.Times, l.WindowSec)},
		Y: Axis{Label: "level (dBFS)", Min: floorTo(floor, 10), Max: ceilTo(maxOf(l.PeakDB)+3, 3)},
		Series: []Series{
			{Name: "peak", X: l.Times, Y: l.PeakDB, Width: 1},
			{Name: "rms", X: l.Times, Y: l.RMSDB, Width: 1.5},
			{Name: "crest", X: l.Times, Y: l.CrestDB, Width: 1, Dashed: true},
		},
	}
	return opt.apply(c, "Level against time")
}
