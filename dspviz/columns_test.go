package dspviz

import (
	"image/color"
	"math"
	"testing"
)

// TestColumnsBounded checks the count lands where the doubling merge says it
// should, for every input length rather than a convenient one.
func TestColumnsBounded(t *testing.T) {
	for _, cap := range []int{2, 4, 8, 16, 100, 101} {
		for frames := range 500 {
			c := newColumns(cap)
			for i := range frames {
				c.add([]float64{float64(i)}, float64(i))
			}
			cols, times := c.result()

			if len(cols) != len(times) {
				t.Fatalf("cap %d, %d frames: %d columns and %d times", cap, frames, len(cols), len(times))
			}
			if len(cols) > c.cap {
				t.Fatalf("cap %d, %d frames: %d columns exceeds the cap", cap, frames, len(cols))
			}
			// Below the cap nothing is folded at all.
			if frames <= c.cap && len(cols) != frames {
				t.Fatalf("cap %d, %d frames: %d columns, want one per frame", cap, frames, len(cols))
			}
			// Above it the count never falls below half, or the reduction would
			// be throwing away resolution it was asked to keep.
			if frames > c.cap && len(cols) < c.cap/2 {
				t.Fatalf("cap %d, %d frames: %d columns, want at least %d", cap, frames, len(cols), c.cap/2)
			}
			if len(times) > 0 && times[0] != 0 {
				t.Fatalf("cap %d, %d frames: the first column starts at %v", cap, frames, times[0])
			}
			for i := 1; i < len(times); i++ {
				if times[i] <= times[i-1] {
					t.Fatalf("cap %d, %d frames: times are not increasing at %d", cap, frames, i)
				}
			}
		}
	}
}

// TestColumnsPreservesMaxima checks that per bin, the maximum over the columns is
// the maximum over every frame. Exactly -- max is idempotent, so nothing is lost
// and nothing is invented.
func TestColumnsPreservesMaxima(t *testing.T) {
	const bins = 5
	for _, cap := range []int{2, 4, 6, 32} {
		for _, frames := range []int{1, 2, 3, 7, 8, 9, 63, 64, 65, 200, 1000} {
			c := newColumns(cap)
			want := make([]float64, bins)
			for i := range want {
				want[i] = math.Inf(-1)
			}
			for f := range frames {
				row := make([]float64, bins)
				for b := range row {
					// Something with a distinct maximum in a distinct frame per
					// bin, so a reduction that dropped a frame would show.
					row[b] = math.Sin(float64(f*7+b*13)) * float64(b+1)
					want[b] = math.Max(want[b], row[b])
				}
				c.add(row, float64(f))
			}
			cols, _ := c.result()

			got := make([]float64, bins)
			for i := range got {
				got[i] = math.Inf(-1)
			}
			for _, col := range cols {
				for b := range col {
					got[b] = math.Max(got[b], col[b])
				}
			}
			for b := range want {
				if got[b] != want[b] {
					t.Fatalf("cap %d, %d frames: bin %d max is %v over the columns, %v over the frames",
						cap, frames, b, got[b], want[b])
				}
			}
		}
	}
}

// TestColumnsUnbounded is the zero case: no reduction at all, which is what
// every existing caller gets.
func TestColumnsUnbounded(t *testing.T) {
	c := newColumns(0)
	for i := range 1000 {
		c.add([]float64{float64(i)}, float64(i))
	}
	cols, times := c.result()
	if len(cols) != 1000 {
		t.Fatalf("%d columns, want every one of 1000", len(cols))
	}
	for i := range cols {
		if cols[i][0] != float64(i) || times[i] != float64(i) {
			t.Fatalf("column %d = %v at %v, want it untouched", i, cols[i][0], times[i])
		}
	}
}

// TestSpectrogramColumnsBoundMemory is the reason Columns exists: the frames are
// the only part that grows with the length of the input.
func TestSpectrogramColumnsBoundMemory(t *testing.T) {
	const rate = 8000
	x := testSignal(200000, rate)

	full, err := SpectrogramOf(x, rate, STFTOptions{Size: 256})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Frames) < 2000 {
		t.Fatalf("only %d frames, too few for this to be testing anything", len(full.Frames))
	}

	for _, cols := range []int{2, 16, 200, 1801} {
		got, err := SpectrogramOf(x, rate, STFTOptions{Size: 256, Columns: cols})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(got.Frames); n > cols+1 {
			t.Errorf("Columns %d gave %d frames", cols, n)
		}
		// Reducing must not change what the peak was, or the color scale would
		// move with the pane width.
		if got.Peak != full.Peak {
			t.Errorf("Columns %d: Peak = %v, want %v", cols, got.Peak, full.Peak)
		}
		if len(got.Freqs) != len(full.Freqs) {
			t.Errorf("Columns %d: %d bins, want %d", cols, len(got.Freqs), len(full.Freqs))
		}
	}
}

// TestSpectrogramReducedRasterIsIdentical tests the columns comment: because
// max is associative and idempotent, folding frames into columns and then into
// pixels gives the same pixel as folding every frame into pixels directly.
//
// That holds when the column boundaries line up with the pixel grid, which the
// doubling merge reaches when the frame count is the cap times a power of two --
// so the signal length is chosen to land there.
func TestSpectrogramReducedRasterIsIdentical(t *testing.T) {
	const (
		rate  = 8000
		size  = 128
		hop   = size / 4
		width = 64
	)
	opt := SpectrogramOptions{Width: width, Height: 40, FullScale: true, FloorDB: -120}

	for _, mult := range []int{1, 2, 4, 8, 16} {
		frames := width * mult
		x := testSignal(size+(frames-1)*hop, rate)

		full, err := SpectrogramOf(x, rate, STFTOptions{Size: size})
		if err != nil {
			t.Fatal(err)
		}
		if len(full.Frames) != frames {
			t.Fatalf("mult %d: %d frames, want %d", mult, len(full.Frames), frames)
		}
		reduced, err := SpectrogramOf(x, rate, STFTOptions{Size: size, Columns: width})
		if err != nil {
			t.Fatal(err)
		}
		if len(reduced.Frames) != width {
			t.Fatalf("mult %d: reduced to %d columns, want exactly %d", mult, len(reduced.Frames), width)
		}

		want := full.Image(GrayPalette(), opt)
		got := reduced.Image(GrayPalette(), opt)
		for y := range 40 {
			for px := range width {
				if got.NRGBAAt(px, y) != want.NRGBAAt(px, y) {
					t.Fatalf("mult %d: pixel (%d,%d) differs from the unreduced raster", mult, px, y)
				}
			}
		}
	}
}

// TestSpectrogramReducedKeepsEveryPeak pins the guarantee that survives when
// the columns do not line up with the pixels: a feature may move by up to a
// column, but none goes missing. Per frequency band, the loudest pixel is as
// loud as unreduced -- a dropped peak is a stopband that is not there.
func TestSpectrogramReducedKeepsEveryPeak(t *testing.T) {
	const (
		rate   = 8000
		width  = 64
		height = 40
	)
	x := testSignal(100000, rate)
	opt := SpectrogramOptions{Width: width, Height: height, FullScale: true, FloorDB: -120}

	full, err := SpectrogramOf(x, rate, STFTOptions{Size: 128})
	if err != nil {
		t.Fatal(err)
	}
	want := full.Image(GrayPalette(), opt)

	rowMax := func(img interface {
		NRGBAAt(x, y int) color.NRGBA
	}, y int) uint8 {
		var m uint8
		for px := range width {
			if v := img.NRGBAAt(px, y).R; v > m {
				m = v
			}
		}
		return m
	}

	// Deliberately awkward counts, including fewer columns than pixels.
	for _, cols := range []int{70, 100, 137, 300, 1000} {
		reduced, err := SpectrogramOf(x, rate, STFTOptions{Size: 128, Columns: cols})
		if err != nil {
			t.Fatal(err)
		}
		got := reduced.Image(GrayPalette(), opt)
		for y := range height {
			if g, w := rowMax(got, y), rowMax(want, y); g != w {
				t.Errorf("Columns %d: row %d peaks at %d, want %d", cols, y, g, w)
			}
		}
	}
}
