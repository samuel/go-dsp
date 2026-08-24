package dspviz

import (
	"math"
	"testing"
)

// TestLevelClosedForms checks the meter against the arithmetic: a sine of
// amplitude a has RMS a/sqrt(2) and a crest factor of sqrt(2), whatever the
// window length, and both are exact enough to compare tightly.
func TestLevelClosedForms(t *testing.T) {
	const (
		rate = 8000.0
		amp  = 0.5
	)
	// Whole cycles in every window, so no window straddles a partial one.
	n := 8000
	x := make([]float64, n)
	for i := range x {
		x[i] = amp * math.Cos(2*math.Pi*float64((100*i)%1000)/1000)
	}
	l, err := LevelOf(x, rate, LevelOptions{WindowSec: 0.05})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := l.WindowSec, 0.05; math.Abs(got-want) > 1e-12 {
		t.Errorf("window %v s, want %v", got, want)
	}
	wantRMS := DB(amp / math.Sqrt2)
	wantPeak := DB(amp)
	for i := range l.RMSDB {
		if math.Abs(l.RMSDB[i]-wantRMS) > 1e-9 {
			t.Fatalf("window %d: rms %v dBFS, want %v", i, l.RMSDB[i], wantRMS)
		}
		if math.Abs(l.PeakDB[i]-wantPeak) > 1e-9 {
			t.Fatalf("window %d: peak %v dBFS, want %v", i, l.PeakDB[i], wantPeak)
		}
		if math.Abs(l.CrestDB[i]-DB(math.Sqrt2)) > 1e-9 {
			t.Fatalf("window %d: crest %v dB, want %v", i, l.CrestDB[i], DB(math.Sqrt2))
		}
	}
}

// TestLevelFoldIsExact is why the reducer folds mean squares rather than taking
// a maximum of them: two equal windows merged give the mean square of the pair
// exactly, so a reduced level plot reads the same as an unreduced one.
func TestLevelFoldIsExact(t *testing.T) {
	const rate = 1000.0
	// A whole power of two of windows, so the reducer's doubling lands on an
	// exact number of source windows per column and the test can say which.
	x := make([]float64, 1024*100)
	for i := range x {
		// Something whose level really varies, so a wrong fold would show.
		x[i] = 0.5 * math.Sin(2*math.Pi*float64(i)/37) * (1 + 0.9*math.Sin(2*math.Pi*float64(i)/9000))
	}
	full, err := LevelOf(x, rate, LevelOptions{WindowSec: 0.1, Columns: 0})
	if err != nil {
		t.Fatal(err)
	}
	small, err := LevelOf(x, rate, LevelOptions{WindowSec: 0.1, Columns: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(small.RMSDB) > 8 {
		t.Fatalf("%d columns, want at most 8", len(small.RMSDB))
	}
	// Every reduced column is the exact mean square of the windows folded into
	// it, and its peak is their exact peak.
	per := len(full.RMSDB) / len(small.RMSDB)
	for i := range small.RMSDB {
		var sum float64
		peak := math.Inf(-1)
		for j := i * per; j < (i+1)*per && j < len(full.RMSDB); j++ {
			sum += math.Pow(10, full.RMSDB[j]/10)
			peak = math.Max(peak, full.PeakDB[j])
		}
		want := 10 * math.Log10(sum/float64(per))
		if math.Abs(small.RMSDB[i]-want) > 1e-9 {
			t.Errorf("column %d: rms %v dBFS, want %v", i, small.RMSDB[i], want)
		}
		if math.Abs(small.PeakDB[i]-peak) > 1e-12 {
			t.Errorf("column %d: peak %v dBFS, want %v", i, small.PeakDB[i], peak)
		}
	}
}

func TestLevelNeedsAWholeWindow(t *testing.T) {
	if _, err := LevelOf(make([]float64, 10), 8000, LevelOptions{}); err == nil {
		t.Error("a signal shorter than one window should not produce a level")
	}
	if _, err := NewLevelBuilder(0, LevelOptions{}); err == nil {
		t.Error("a rate of zero should be an error")
	}
}
