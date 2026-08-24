package dspviz

import (
	"math"
	"testing"
)

// TestLevelsFullScale pins the one thing SpectrogramOptions could not say
// before: that the top of the color scale is 0 dBFS. Zero means "choose one",
// so without a separate field the value a file analyzer always wants is the one
// value it cannot ask for.
func TestLevelsFullScale(t *testing.T) {
	sg := &Spectrogram{Peak: -31}

	for _, tc := range []struct {
		name                string
		opt                 SpectrogramOptions
		wantFloor, wantCeil float64
	}{
		// The zero value still auto-ranges both ends, which is what every
		// existing caller relies on.
		{"zero value", SpectrogramOptions{}, -210, -30},

		// A floor on its own still leaves the ceiling automatic. cmd/dspviz
		// passes exactly this when -floor is given, and a filter with gain
		// needs the head room.
		{"floor only", SpectrogramOptions{FloorDB: -120}, -120, -30},

		// The new case.
		{"full scale", SpectrogramOptions{FullScale: true, FloorDB: -120}, -120, 0},

		// FullScale wins over an explicit ceiling rather than the two fighting.
		{"full scale over ceiling", SpectrogramOptions{FullScale: true, CeilDB: -12, FloorDB: -90}, -90, 0},

		// An explicit non-zero ceiling is honored as it always was.
		{"explicit ceiling", SpectrogramOptions{CeilDB: 6, FloorDB: -60}, -60, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			floor, ceil := tc.opt.levels(sg)
			if floor != tc.wantFloor || ceil != tc.wantCeil {
				t.Errorf("levels() = (%g, %g), want (%g, %g)", floor, ceil, tc.wantFloor, tc.wantCeil)
			}
		})
	}
}

// TestImageTwoSidedRows checks that the raster reads a spectrogram whose bins
// start below zero. binRange divides the frequency by the bin spacing, which is
// only correct when bin zero is DC; a two-sided spectrum starts at -rate/2.
//
// The spectrogram is built by hand rather than measured, so the expected row
// is arithmetic rather than another implementation of the same mapping.
func TestImageTwoSidedRows(t *testing.T) {
	const (
		bins = 8
		rate = 8.0
		h    = bins - 1 // one row per bin, so the mapping is exact
	)

	// Freqs run -4, -3, ... 3: the order FFTShift leaves a transform in.
	freqs := make([]float64, bins)
	for i := range freqs {
		freqs[i] = float64(i-bins/2) * rate / bins
	}

	for loud := range bins - 1 {
		frames := make([]float64, bins)
		for i := range frames {
			frames[i] = -120
		}
		frames[loud] = 0

		sg := &Spectrogram{
			Frames: [][]float64{frames},
			Times:  []float64{0},
			Freqs:  freqs,
			Rate:   rate,
			Floor:  -120,
			Peak:   0,
		}

		img := sg.Image(GrayPalette(), SpectrogramOptions{
			Width: 4, Height: h, FullScale: true, FloorDB: -120,
		})

		// Rows run bottom-up, and row j from the bottom holds the bin at
		// freqs[j], so the loud bin lands h-1-loud rows down from the top.
		wantY := h - 1 - loud
		var gotY = -1
		for y := range h {
			if img.NRGBAAt(0, y).R == 0xff {
				if gotY >= 0 {
					t.Fatalf("bin %d: rows %d and %d are both full scale", loud, gotY, y)
				}
				gotY = y
			}
		}
		if gotY != wantY {
			t.Errorf("bin %d (%.0f Hz) drew at row %d, want %d", loud, freqs[loud], gotY, wantY)
		}
	}
}

// TestImageOneSidedUnchanged is the other half of the binRange fix: for a
// spectrum that does start at DC the offset subtracts zero, so nothing about
// the existing behavior may move.
func TestImageOneSidedUnchanged(t *testing.T) {
	const bins = 9
	freqs := make([]float64, bins)
	for i := range freqs {
		freqs[i] = float64(i) * 1000
	}
	for loud := range bins - 1 {
		frames := make([]float64, bins)
		for i := range frames {
			frames[i] = -120
		}
		frames[loud] = 0

		sg := &Spectrogram{
			Frames: [][]float64{frames}, Times: []float64{0}, Freqs: freqs,
			Rate: 16000, Floor: -120, Peak: 0,
		}
		h := bins - 1
		img := sg.Image(GrayPalette(), SpectrogramOptions{
			Width: 2, Height: h, FullScale: true, FloorDB: -120,
		})
		if got, want := img.NRGBAAt(0, h-1-loud).R, uint8(0xff); got != want {
			t.Errorf("bin %d: row %d is %#x, want %#x", loud, h-1-loud, got, want)
		}
	}
}

// TestImageTwoSidedCentersDC is the property a two-sided spectrogram is read
// for: DC lands in the middle of the pane rather than at an edge. Before the
// binRange fix the negative half of the band clamped onto bin zero and the DC
// bin was never drawn at all.
//
// The check is made in frequency rather than in rows, so it does not restate
// the row mapping it is testing.
func TestImageTwoSidedCentersDC(t *testing.T) {
	const (
		bins = 64
		rate = 48000.0
		h    = 256
	)
	freqs := make([]float64, bins)
	frames := make([]float64, bins)
	for i := range freqs {
		freqs[i] = float64(i-bins/2) * rate / bins
		frames[i] = -150
	}
	frames[bins/2] = 0 // DC

	sg := &Spectrogram{
		Frames: [][]float64{frames}, Times: []float64{0}, Freqs: freqs,
		Rate: rate, Floor: -150, Peak: 0,
	}
	img := sg.Image(GrayPalette(), SpectrogramOptions{
		Width: 8, Height: h, FullScale: true, FloorDB: -150,
	})

	brightest, best := -1, -1.0
	for y := range h {
		if v := float64(img.NRGBAAt(0, y).R); v > best {
			best, brightest = v, y
		}
	}
	if best != 0xff {
		t.Fatalf("no row reached full scale; brightest was %g", best)
	}

	// Rows run bottom-up over [Freqs[0], Freqs[bins-1]], so turn the row back
	// into the frequency it stands for and require it to be the DC bin's.
	minF, maxF := freqs[0], freqs[bins-1]
	step := rate / bins
	got := minF + float64(h-1-brightest)/float64(h)*(maxF-minF)
	if math.Abs(got) > step {
		t.Errorf("the DC bin drew at row %d of %d, which is %.0f Hz; want within %.0f Hz of 0",
			brightest, h, got, step)
	}
}
