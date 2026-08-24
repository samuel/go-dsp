package dspviz

import (
	"math"
	"testing"
)

func TestHistogramCountsEverySample(t *testing.T) {
	h := mustHistogram(t, HistogramOptions{Buckets: 8, Complex: false})
	x := []float64{-1, -0.75, -0.25, 0, 0.25, 0.6, 0.9999}
	if err := h.Write(x); err != nil {
		t.Fatal(err)
	}
	hg := closedHistogram(t, h)
	var total int64
	for _, c := range hg.Count {
		total += c
	}
	if total != int64(len(x)) || hg.Samples != int64(len(x)) || hg.Outside != 0 {
		t.Fatalf("counted %d of %d samples, %d outside", total, len(x), hg.Outside)
	}
	if hg.Count[0] != 1 {
		t.Errorf("the most negative sample is not in the first bucket")
	}
	if hg.Count[len(hg.Count)-1] != 1 {
		t.Errorf("a sample one step below full scale is not in the last bucket")
	}
	if hg.Min != -1 || math.Abs(hg.Max-0.9999) > 1e-12 {
		t.Errorf("extremes %v..%v, want -1..0.9999", hg.Min, hg.Max)
	}
}

// TestHistogramSeesCoarseQuantization is what the plot is for: a signal
// quantized more coarsely than its container leaves a comb of empty buckets,
// which is unmistakable on the chart and countable here.
func TestHistogramSeesCoarseQuantization(t *testing.T) {
	fine := mustHistogram(t, HistogramOptions{Buckets: 256, Complex: false})
	coarse := mustHistogram(t, HistogramOptions{Buckets: 256, Complex: false})
	x := make([]float64, 20000)
	q := make([]float64, len(x))
	for i := range x {
		x[i] = 0.9 * math.Sin(float64(i)*0.017)
		// Eight bits of signal in the same range.
		q[i] = math.Round(x[i]*128) / 128
	}
	if err := fine.Write(x); err != nil {
		t.Fatal(err)
	}
	if err := coarse.Write(q); err != nil {
		t.Fatal(err)
	}
	fineN := closedHistogram(t, fine).Occupied()
	coarseN := closedHistogram(t, coarse).Occupied()
	if fineN <= coarseN {
		t.Errorf("the coarse signal occupies %d buckets and the fine one %d; the comb is invisible",
			coarseN, fineN)
	}
}

func TestHistogramOutsideFullScale(t *testing.T) {
	h := mustHistogram(t, HistogramOptions{Buckets: 16, Complex: false})
	if err := h.Write([]float64{2, -2, math.NaN(), 0}); err != nil {
		t.Fatal(err)
	}
	hg := closedHistogram(t, h)
	if hg.Outside != 3 {
		t.Errorf("%d samples outside full scale, want 3", hg.Outside)
	}
	if hg.Samples != 4 {
		t.Errorf("%d samples counted, want 4", hg.Samples)
	}
}

func TestHistogramComplexKeepsThreeCurves(t *testing.T) {
	h := mustHistogram(t, HistogramOptions{Buckets: 64, Complex: true})
	z := make([]complex128, 1000)
	for i := range z {
		th := 2 * math.Pi * float64(i) / 64
		z[i] = complex(0.5*math.Cos(th), 0.5*math.Sin(th))
	}
	if err := h.WriteComplex(z); err != nil {
		t.Fatal(err)
	}
	// A constant-envelope signal puts every magnitude in one bucket, while I and
	// Q are spread across the arcsine distribution.
	hg := closedHistogram(t, h)
	var occupied int
	for _, c := range hg.CountMag {
		if c > 0 {
			occupied++
		}
	}
	if occupied != 1 {
		t.Errorf("|z| occupies %d buckets, want 1 for a constant envelope", occupied)
	}
	if hg.Occupied() < 10 {
		t.Errorf("I occupies %d buckets, want it spread out", hg.Occupied())
	}
}

func TestHistogramReset(t *testing.T) {
	h := mustHistogram(t, HistogramOptions{Buckets: 8, Complex: false})
	if err := h.Write([]float64{0.5, -0.5}); err != nil {
		t.Fatal(err)
	}
	h.Reset()
	if _, err := h.Close(); err == nil {
		t.Error("Close after Reset returned a histogram; Reset left the counts behind")
	}
	if err := h.Write([]float64{0.25}); err != nil {
		t.Fatal(err)
	}
	hg := closedHistogram(t, h)
	if hg.Samples != 1 || hg.Occupied() != 1 || hg.Min != 0.25 {
		t.Errorf("after Reset and one sample: %d samples, %d buckets, min %v", hg.Samples, hg.Occupied(), hg.Min)
	}
}

// TestHistogramCountsFullScale checks that a sample of exactly +1.0 lands in the
// top bucket rather than being counted as out of range.
//
// The buckets tile [-1, 1), so the top edge is the start of the one after and
// +1.0 falls one past the end. A float file can hold that sample, and reporting
// it as outside says the signal left a range it only touched.
func TestHistogramCountsFullScale(t *testing.T) {
	h := mustHistogram(t, HistogramOptions{Buckets: 16, Complex: false})
	if err := h.Write([]float64{-1, 0, 1}); err != nil {
		t.Fatal(err)
	}
	hg := closedHistogram(t, h)
	if hg.Outside != 0 {
		t.Errorf("Outside = %d, want 0; -1, 0 and +1 are all in range", hg.Outside)
	}
	if n := hg.Count[len(hg.Count)-1]; n != 1 {
		t.Errorf("the top bucket holds %d, want the +1.0 sample", n)
	}
	if n := hg.Count[0]; n != 1 {
		t.Errorf("the bottom bucket holds %d, want the -1.0 sample", n)
	}

	// Past the rails is still outside.
	if err := h.Write([]float64{1.5, -1.5}); err != nil {
		t.Fatal(err)
	}
	if n := closedHistogram(t, h).Outside; n != 2 {
		t.Errorf("Outside = %d, want 2", n)
	}
}

// mustHistogram builds a builder for the tests, which all use options the
// constructor accepts.
func mustHistogram(t *testing.T, opt HistogramOptions) *HistogramBuilder {
	t.Helper()
	b, err := NewHistogramBuilder(opt)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// closedHistogram closes a builder for a test that only wants the counts.
func closedHistogram(t *testing.T, b *HistogramBuilder) *Histogram {
	t.Helper()
	h, err := b.Close()
	if err != nil {
		t.Fatal(err)
	}
	return h
}
