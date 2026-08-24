package dspviz

import (
	"errors"
	"fmt"
	"math"
)

// Envelope reduces x to n columns, keeping each column's minimum and maximum
// and the index of the first sample of it. The index, not a time: Envelope is
// handed a slice and no sample rate, so the caller divides.
//
// Envelope rather than Decimate, which is what a rate changer does: this keeps
// two numbers per column and throws the rest away, where a decimator low-passes
// and resamples.
//
// The extremes, not the mean. A single-sample transient is what a waveform
// overview exists to show, and a mean erases it.
//
// It also plugs the hole in the chart layer, which has no decimation of its own:
// a million-point series is a twenty megabyte SVG, and a larger one is a
// non-starter.
func Envelope(x []float64, n int) (xs, mins, maxs []float64) {
	if n <= 0 || len(x) == 0 {
		return nil, nil, nil
	}
	if n > len(x) {
		n = len(x)
	}
	xs = make([]float64, n)
	mins = make([]float64, n)
	maxs = make([]float64, n)
	for i := range n {
		lo := len(x) * i / n
		hi := max(lo+1, len(x)*(i+1)/n)
		hi = min(hi, len(x))
		mn, mx := x[lo], x[lo]
		for _, v := range x[lo:hi] {
			mn, mx = math.Min(mn, v), math.Max(mx, v)
		}
		xs[i], mins[i], maxs[i] = float64(lo), mn, mx
	}
	return xs, mins, maxs
}

// Waveform is a min/max envelope over time.
type Waveform struct {
	Rate    float64
	Times   []float64 // start of each column, in seconds
	Min     []float64
	Max     []float64
	Samples int64
}

// WaveformOptions tunes an envelope.
type WaveformOptions struct {
	// Columns bounds the result the way STFTOptions.Columns bounds a
	// spectrogram. Zero selects 1800, which is twice a wide pane.
	Columns int
}

// WaveformBuilder reduces a signal to a bounded envelope as it arrives.
type WaveformBuilder struct {
	rate float64
	opt  WaveformOptions
	f    fold[[2]float64] // {max, min}
}

// NewWaveformBuilder starts an envelope.
func NewWaveformBuilder(rate float64, opt WaveformOptions) (*WaveformBuilder, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("dspviz: a sample rate of %g", rate)
	}
	columns := opt.Columns
	if columns <= 0 {
		columns = 1800
	}
	return &WaveformBuilder{
		rate: rate,
		opt:  opt,
		// Maximum and minimum, which take no notice of how many samples are
		// behind either side: max and min are idempotent.
		f: newFold(columns, func(a [2]float64, _ int64, b [2]float64, _ int64) [2]float64 {
			return [2]float64{math.Max(a[0], b[0]), math.Min(a[1], b[1])}
		}),
	}, nil
}

// Reset clears the envelope, so the builder starts a new signal.
func (b *WaveformBuilder) Reset() {
	n, err := NewWaveformBuilder(b.rate, b.opt)
	if err != nil {
		return
	}
	*b = *n
}

// Write adds samples.
func (b *WaveformBuilder) Write(x []float64) error {
	for _, v := range x {
		b.f.add([2]float64{v, v}, float64(b.f.n)/b.rate)
	}
	return nil
}

// WriteComplex adds the magnitude of I/Q samples, which is the envelope a
// complex capture has instead of a waveform.
func (b *WaveformBuilder) WriteComplex(x []complex128) error {
	for _, z := range x {
		m := math.Hypot(real(z), imag(z))
		b.f.add([2]float64{m, -m}, float64(b.f.n)/b.rate)
	}
	return nil
}

// Close finishes the envelope.
func (b *WaveformBuilder) Close() (*Waveform, error) {
	if len(b.f.rows) == 0 {
		return nil, errors.New("dspviz: no samples")
	}
	w := &Waveform{Rate: b.rate, Times: b.f.times, Samples: b.f.n}
	w.Min = make([]float64, len(b.f.rows))
	w.Max = make([]float64, len(b.f.rows))
	for i, r := range b.f.rows {
		w.Max[i], w.Min[i] = r[0], r[1]
	}
	return w, nil
}

// WaveformOf is the whole-signal form.
func WaveformOf(x []float64, rate float64, opt WaveformOptions) (*Waveform, error) {
	b, err := NewWaveformBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return b.Close()
}

// zigzag interleaves an envelope into the single alternating series a waveform
// is drawn as: minimum then maximum for each column, so one path covers the
// whole band the samples occupied. Two filled series instead would draw the
// area between them as though the signal were never there, which is the
// opposite of the truth.
func zigzag(times, mins, maxs []float64) (xs, ys []float64) {
	n := min(len(times), min(len(mins), len(maxs)))
	xs = make([]float64, 0, 2*n)
	ys = make([]float64, 0, 2*n)
	for i := range n {
		xs = append(xs, times[i], times[i])
		ys = append(ys, mins[i], maxs[i])
	}
	return xs, ys
}

// WaveformChart draws the envelope against time.
func WaveformChart(w *Waveform, opt ChartOptions) *Chart {
	xs, ys := zigzag(w.Times, w.Min, w.Max)
	peak := math.Max(maxOf(w.Max), -minOf(w.Min))
	span := math.Max(peak*1.05, 1e-6)

	c := &Chart{
		Subtitle: fmt.Sprintf("%d samples in %d columns, peak %.4f (%.2f dBFS)",
			w.Samples, len(w.Times), peak, DB(peak)),
		X:      Axis{Label: "time (s)", Min: 0, Max: timeAxisEnd(w.Times, 1/w.Rate)},
		Y:      Axis{Label: "amplitude", Min: -span, Max: span},
		Series: []Series{{Name: "waveform", X: xs, Y: ys, Width: 1}},
		Notes: []string{
			"each column is the minimum and the maximum of the samples in it, never their mean: " +
				"a transient one sample wide is what this plot is for",
		},
	}
	return opt.apply(c, "Waveform")
}
