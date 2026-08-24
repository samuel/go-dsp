package dspviz

import (
	"errors"
	"fmt"
	"math"
)

// Histogram counts samples into equal buckets across full scale.
//
// It is diagnostic out of all proportion to its cost. Clipping is a spike at the
// rails; a DC offset is a distribution that is not centered; quantization coarser
// than the container is a comb of empty buckets, and dither is the absence of
// one; an eight-bit signal in a sixteen-bit file is unmistakable. For an I/Q
// capture the magnitude is the Rayleigh check -- noise alone has a known shape,
// and anything else in the picture is signal.
type Histogram struct {
	// Centers is the amplitude at the middle of each bucket, running from -1 to
	// +1 for a real signal and from 0 to 1 for a magnitude.
	Centers []float64

	Count    []int64 // real samples, or I for a complex signal
	CountQ   []int64 // Q, only for a complex signal
	CountMag []int64 // |z|, only for a complex signal

	Samples int64
	Min     float64
	Max     float64
	Outside int64 // samples past full scale, which a float file may hold

	Complex bool
}

// HistogramOptions tunes a histogram.
type HistogramOptions struct {
	// Buckets is how finely full scale is divided. Zero selects 4096, which
	// resolves twelve bits of a comb.
	Buckets int

	// Complex counts I, Q and the magnitude separately rather than one real
	// distribution.
	Complex bool
}

// HistogramBuilder counts a signal handed over a piece at a time.
type HistogramBuilder struct {
	h Histogram
}

// NewHistogramBuilder returns a builder for the given options.
func NewHistogramBuilder(opt HistogramOptions) (*HistogramBuilder, error) {
	if opt.Buckets < 0 {
		return nil, fmt.Errorf("dspviz: %d buckets", opt.Buckets)
	}
	buckets := opt.Buckets
	if buckets == 0 {
		buckets = 4096
	}
	b := &HistogramBuilder{h: Histogram{
		Centers: make([]float64, buckets),
		Count:   make([]int64, buckets),
		Complex: opt.Complex,
		Min:     math.Inf(1),
		Max:     math.Inf(-1),
	}}
	for i := range buckets {
		b.h.Centers[i] = -1 + (float64(i)+0.5)*2/float64(buckets)
	}
	if opt.Complex {
		b.h.CountQ = make([]int64, buckets)
		b.h.CountMag = make([]int64, buckets)
	}
	return b, nil
}

// Reset clears the counts.
func (b *HistogramBuilder) Reset() {
	clear(b.h.Count)
	clear(b.h.CountQ)
	clear(b.h.CountMag)
	b.h.Samples, b.h.Outside = 0, 0
	b.h.Min, b.h.Max = math.Inf(1), math.Inf(-1)
}

// Write adds real samples.
func (b *HistogramBuilder) Write(x []float64) error {
	for _, v := range x {
		b.h.Min, b.h.Max = math.Min(b.h.Min, v), math.Max(b.h.Max, v)
		b.h.Samples++
		b.bump(b.h.Count, v)
	}
	return nil
}

// WriteComplex adds I/Q samples, counting I, Q and the magnitude separately.
//
// The magnitude is counted on the same scale as the two components, so it fills
// the upper half of the axis. That is the honest place for it: a full-scale
// complex exponential has unit magnitude, exactly as a full-scale real sine has
// unit amplitude.
func (b *HistogramBuilder) WriteComplex(x []complex128) error {
	for _, z := range x {
		i, q := real(z), imag(z)
		m := math.Hypot(i, q)
		b.h.Min = math.Min(b.h.Min, math.Min(i, q))
		b.h.Max = math.Max(b.h.Max, math.Max(i, q))
		b.h.Samples++
		b.bump(b.h.Count, i)
		b.bump(b.h.CountQ, q)
		b.bump(b.h.CountMag, m)
	}
	return nil
}

// Close returns the counts. The builder keeps them, so it stays usable and a
// second Close returns the same histogram with whatever arrived in between.
func (b *HistogramBuilder) Close() (*Histogram, error) {
	if b.h.Samples == 0 {
		return nil, errors.New("dspviz: no samples")
	}
	h := b.h
	return &h, nil
}

// HistogramOf is the whole-signal form.
func HistogramOf(x []float64, opt HistogramOptions) (*Histogram, error) {
	b, err := NewHistogramBuilder(opt)
	if err != nil {
		return nil, err
	}
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return b.Close()
}

func (b *HistogramBuilder) bump(into []int64, v float64) {
	if math.IsNaN(v) {
		b.h.Outside++
		return
	}
	i := int((v + 1) / 2 * float64(len(into)))
	if i == len(into) && v <= 1 {
		// Exactly +1.0 lands one past the top bucket, because the buckets tile
		// [-1, 1) and the top edge is the start of the one after. It belongs in
		// the last bucket: a float file can hold a sample of 1.0, and counting
		// it as out of range reports a signal that merely touches full scale as
		// having left it.
		i--
	}
	if i < 0 || i >= len(into) {
		b.h.Outside++
		return
	}
	into[i]++
}

// Occupied returns how many buckets hold at least one sample, the same fact
// the statistics' effective-bit-depth estimate reads the other way around.
func (h *Histogram) Occupied() int {
	var n int
	for _, c := range h.Count {
		if c > 0 {
			n++
		}
	}
	return n
}

// HistogramChart draws the distribution on a logarithmic count axis.
//
// A bucket nothing landed in cannot be drawn on a logarithmic axis and leaves a
// gap in the line rather than a point on the floor, which is what makes a comb
// read as a comb.
func HistogramChart(h *Histogram, opt ChartOptions) *Chart {
	series := []Series{{Name: "count", X: h.Centers, Y: counts(h.Count), Width: 1}}
	if h.Complex {
		series = []Series{
			{Name: "I", X: h.Centers, Y: counts(h.Count), Width: 1},
			{Name: "Q", X: h.Centers, Y: counts(h.CountQ), Width: 1},
			{Name: "|z|", X: h.Centers, Y: counts(h.CountMag), Width: 1},
		}
	}

	var notes []string
	if h.Outside > 0 {
		notes = append(notes, fmt.Sprintf("%d samples are outside full scale and are not counted", h.Outside))
	}
	occupied := h.Occupied()
	if occupied > 0 && occupied < len(h.Count)/2 {
		notes = append(notes, fmt.Sprintf(
			"only %d of %d buckets hold anything: the signal is quantized more coarsely than the file",
			occupied, len(h.Count)))
	}

	c := &Chart{
		Subtitle: fmt.Sprintf("%d samples in %d buckets, from %.4f to %.4f",
			h.Samples, len(h.Count), h.Min, h.Max),
		X:      Axis{Label: "amplitude", Min: -1, Max: 1},
		Y:      Axis{Label: "samples", Log: true},
		Series: series,
		Notes:  notes,
	}
	return opt.apply(c, "Amplitude distribution")
}

func counts(c []int64) []float64 {
	y := make([]float64, len(c))
	for i, v := range c {
		y[i] = float64(v)
	}
	return y
}
