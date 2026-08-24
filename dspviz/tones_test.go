package dspviz

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// fsk builds a two-tone keyed signal, alternating one symbol at a time.
func fsk(n int, rate, mark, space, baud float64) []float64 {
	x := make([]float64, n)
	per := int(math.Round(rate / baud))
	for i := range x {
		f := mark
		if (i/per)%2 == 1 {
			f = space
		}
		x[i] = 0.5 * math.Sin(2*math.Pi*f*float64(i)/rate)
	}
	return x
}

// TestToneTrackFollowsTheKeying checks that the decision variable changes
// sign once per symbol, and it does so on the same numbers a
// decoder built on dsp.Goertzel would be looking at.
func TestToneTrackFollowsTheKeying(t *testing.T) {
	const (
		rate  = 11025.0
		mark  = 1600.0
		space = 1800.0
		baud  = 300.0
	)
	x := fsk(int(rate), rate, mark, space, baud)
	tr, err := ToneTrackOf(x, rate, ToneTrackOptions{
		Mark: mark, Space: space, Baud: baud, Threshold: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Block != 37 {
		t.Errorf("block %d samples, want 37 (one symbol at %g baud)", tr.Block, baud)
	}
	// One second of alternating symbols at 300 baud is 300 symbols and so about
	// 300 sign changes, minus whatever the block straddling a symbol edge eats.
	if tr.Transitions < 250 || tr.Transitions > 320 {
		t.Errorf("%d sign changes, want about 300", tr.Transitions)
	}
	if tr.Crossings == 0 {
		t.Fatal("nothing crossed the threshold")
	}
}

// TestToneTrackReportsAThresholdOnTheWrongScale is the failure the view exists
// to name. Nothing about a decoder that emits no bits says whether the tones are
// wrong, the clock never locks, or the threshold is simply on a different scale
// than the signal -- and this is the last of those, stated in one line.
func TestToneTrackReportsAThresholdOnTheWrongScale(t *testing.T) {
	const rate = 11025.0
	x := fsk(int(rate)/2, rate, 1600, 1800, 300)
	tr, err := ToneTrackOf(x, rate, ToneTrackOptions{
		Mark: 1600, Space: 1800, Baud: 300, Threshold: 1e9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Crossings != 0 {
		t.Fatalf("%d crossings at an absurd threshold", tr.Crossings)
	}
	if len(tr.Warnings) == 0 || !strings.Contains(tr.Warnings[0], "never reaches the threshold") {
		t.Errorf("the warnings do not name the problem: %v", tr.Warnings)
	}
	// And the suggested threshold is one the signal would actually cross.
	if tr.Suggested <= 0 || tr.Suggested > tr.MaxAbsDiff {
		t.Errorf("suggested threshold %v against a largest difference of %v", tr.Suggested, tr.MaxAbsDiff)
	}

	var buf bytes.Buffer
	if err := tr.WriteSummary(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "0 over the threshold") {
		t.Errorf("the summary hides the crossing count:\n%s", buf.String())
	}
}

// TestToneTrackReportsOneToneOnly separates the other failure: the tones are in
// the wrong place, so one of them is never there and the decision never changes
// sign however good the threshold is.
func TestToneTrackReportsOneToneOnly(t *testing.T) {
	const rate = 8000.0
	x := make([]float64, 8000)
	for i := range x {
		x[i] = 0.5 * math.Sin(2*math.Pi*1200*float64(i)/rate)
	}
	tr, err := ToneTrackOf(x, rate, ToneTrackOptions{
		Mark: 1200, Space: 2200, Baud: 1200, Threshold: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Transitions != 0 {
		t.Errorf("%d sign changes on a single steady tone, want 0", tr.Transitions)
	}
	if len(tr.Warnings) == 0 {
		t.Error("a decision that never changes sign should be reported")
	}
}

// TestTonePeaksFindTheShift reads the tones the capture really holds, which is
// what says a command line is for a different mode: a 200 Hz shift against flags
// asking for 1000 is not a tuning error.
func TestTonePeaksFindTheShift(t *testing.T) {
	const (
		rate = 11025.0
		size = 2048
	)
	x := fsk(64*size, rate, 1600, 1800, 300)
	p, err := PSDOf(x, rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	peaks := TonePeaks(p, 2, 100)
	if len(peaks) != 2 {
		t.Fatalf("found %d peaks, want 2", len(peaks))
	}
	lo, hi := peaks[0].Hz, peaks[1].Hz
	if lo > hi {
		lo, hi = hi, lo
	}
	if math.Abs(lo-1600) > 30 || math.Abs(hi-1800) > 30 {
		t.Errorf("peaks at %v and %v Hz, want 1600 and 1800", lo, hi)
	}
	if math.Abs((hi-lo)-200) > 40 {
		t.Errorf("measured shift %v Hz, want 200", hi-lo)
	}
	for _, pk := range peaks {
		if pk.SNRDB < 10 {
			t.Errorf("peak at %v Hz is only %v dB over the noise", pk.Hz, pk.SNRDB)
		}
	}
}

func TestToneTrackValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		rate float64
		opt  ToneTrackOptions
	}{
		{"no rate", 0, ToneTrackOptions{Mark: 1, Space: 2, Baud: 1}},
		{"no tones", 8000, ToneTrackOptions{Baud: 300}},
		{"no block", 8000, ToneTrackOptions{Mark: 1200, Space: 2200}},
		{"block of one", 8000, ToneTrackOptions{Mark: 1200, Space: 2200, Baud: 9000}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewToneTrackBuilder(tt.rate, tt.opt); err == nil {
				t.Error("want an error")
			}
		})
	}
	if _, err := ToneTrackOf(make([]float64, 4), 8000, ToneTrackOptions{
		Mark: 1200, Space: 2200, Baud: 300,
	}); err == nil {
		t.Error("a signal shorter than one block should not produce a track")
	}
}
