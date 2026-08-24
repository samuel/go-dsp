package dspviz

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
)

// Image renders the spectrogram as a raster, time across and frequency up.
//
// Where several bins or frames fall in one pixel it takes the loudest of them
// rather than the mean. That matters: an alias or a spur is one bin wide, and
// averaging it against its silent neighbors is exactly how a plot comes to show
// a clean stopband that is not there.
func (sg *Spectrogram) Image(p Palette, opt SpectrogramOptions) *image.NRGBA {
	if len(sg.Freqs) < 2 {
		// No frequency axis to raster. NewSpectrogramBuilder rejects the frame
		// size that produces this, so it takes a hand-built Spectrogram to get
		// here; returning an empty image beats indexing Freqs[1].
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	w, h := opt.size()
	floor, ceil := opt.levels(sg)
	if p == nil {
		p = ViridisPalette()
	}

	bins := len(sg.Freqs)
	maxF := opt.MaxFreq
	if maxF <= 0 || maxF > sg.Freqs[bins-1] {
		maxF = sg.Freqs[bins-1]
	}
	// A logarithmic frequency axis cannot draw a two-sided spectrum: half of it
	// is negative and the other half passes through zero. Ignoring the request
	// beats erroring, since an "everything" run sets it once for every chart.
	logFreq := opt.LogFreq && !sg.TwoSided()
	minF := sg.Freqs[0]
	if logFreq {
		minF = math.Max(sg.Freqs[1], 1)
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	span := ceil - floor

	for px := range w {
		// Frames covered by this column.
		f0 := len(sg.Frames) * px / w
		f1 := max(f0+1, len(sg.Frames)*(px+1)/w)
		f1 = min(f1, len(sg.Frames))

		for py := range h {
			// Rows run bottom-up: y = h-1 is the lowest frequency.
			lo, hi := binRange(h-1-py, h, bins, minF, maxF, sg, logFreq)
			worst := math.Inf(-1)
			for fi := f0; fi < f1; fi++ {
				row := sg.Frames[fi]
				for b := lo; b < hi && b < len(row); b++ {
					worst = math.Max(worst, row[b])
				}
			}
			if math.IsInf(worst, -1) {
				worst = floor
			}
			img.SetNRGBA(px, py, toNRGBA(p((worst-floor)/span)))
		}
	}
	return img
}

// binRange returns the half-open range of frequency bins that fall in row y of
// h, counting from the bottom.
func binRange(y, h, bins int, minF, maxF float64, sg *Spectrogram, logFreq bool) (lo, hi int) {
	fa := freqAtRow(float64(y)/float64(h), minF, maxF, logFreq)
	fb := freqAtRow(float64(y+1)/float64(h), minF, maxF, logFreq)
	step := sg.Freqs[1] - sg.Freqs[0]
	if step <= 0 {
		return 0, bins
	}
	// Offset by the first bin rather than dividing the frequency. A
	// one-sided spectrum starts at 0 Hz and the two are the same thing there,
	// but a two-sided one starts at -rate/2, and reading it as though bin zero
	// were DC puts every row half a band away from where it belongs.
	lo = int((fa - sg.Freqs[0]) / step)
	hi = int(math.Ceil((fb - sg.Freqs[0]) / step))
	lo = max(0, min(lo, bins-1))
	hi = max(lo+1, min(hi, bins))
	return lo, hi
}

func freqAtRow(t, minF, maxF float64, logFreq bool) float64 {
	if logFreq {
		if minF <= 0 {
			minF = 1
		}
		return math.Exp(math.Log(minF) + t*(math.Log(maxF)-math.Log(minF)))
	}
	return minF + t*(maxF-minF)
}

func toNRGBA(c color.RGBA) color.NRGBA { return color.NRGBA(c) }

// WritePNG writes the bare raster, with no axes.
func (sg *Spectrogram) WritePNG(w io.Writer, p Palette, opt SpectrogramOptions) error {
	return png.Encode(w, sg.Image(p, opt))
}

// WriteSVG writes the spectrogram with labeled axes and a color bar.
//
// The data pane is a PNG embedded as a data URI inside an otherwise ordinary
// chart. That is what makes a labeled spectrogram possible with only the
// standard library: image/png can draw the pane but no text, and SVG has text
// but would need one rectangle per bin for the pane. Together they give axes,
// ticks and a color bar in one self-contained file.
func (sg *Spectrogram) WriteSVG(w io.Writer, p Palette, opt SpectrogramOptions) error {
	return sg.chart(p, opt).WriteSVG(w)
}

// chart builds the surround. The raster goes in as an Image on the Chart, which
// is otherwise an ordinary line plot with no series.
func (sg *Spectrogram) chart(p Palette, opt SpectrogramOptions) *Chart {
	if len(sg.Freqs) < 2 || len(sg.Times) == 0 {
		// Nothing to draw and no axis to label; see the note in Image.
		return &Chart{Title: opt.Title}
	}
	w, h := opt.size()
	floor, ceil := opt.levels(sg)

	var buf bytes.Buffer
	if err := png.Encode(&buf, sg.Image(p, opt)); err != nil {
		// png.Encode on an in-memory image and an in-memory buffer cannot fail
		// for any reason a caller could act on, so the picture simply comes out
		// without its pane rather than the whole call growing an error.
		buf.Reset()
	}

	twoSided := sg.TwoSided()
	logFreq := opt.LogFreq && !twoSided

	maxF := opt.MaxFreq
	if maxF <= 0 {
		maxF = sg.Freqs[len(sg.Freqs)-1]
	}
	minF := sg.Freqs[0]
	if logFreq {
		minF = math.Max(sg.Freqs[1], 1)
	}
	endT := timeAxisEnd(sg.Times, 1/sg.Rate)

	// A plain "Spectrogram": it draws a filter sweep and a recording alike, and
	// every caller with something more specific passes a title.
	title := "Spectrogram"
	if opt.Title != "" {
		title = opt.Title
	}

	// A two-sided axis is in offsets from wherever the receiver was tuned, so the
	// ticks stay small numbers and the tuner frequency is named once in the
	// subtitle. Putting absolute frequencies on the ticks reads well until a
	// 100 MHz center meets the axis formatter and every label becomes 1.00e+08.
	yLabel := hzLabel
	subtitle := fmt.Sprintf("%.0f dB of range, %.0f to %.0f dBFS", ceil-floor, floor, ceil)
	if twoSided {
		yLabel = "frequency offset (Hz)"
		band := hzString(sg.Rate)
		if sg.Center != 0 {
			subtitle = fmt.Sprintf("center %s, %s wide; %s", hzString(sg.Center), band, subtitle)
		} else {
			subtitle = fmt.Sprintf("%s wide, two-sided; %s", band, subtitle)
		}
	}

	notes := []string{
		"structure, not depth: a spectrogram's floor is its window and its palette, " +
			"so read a stopband's depth from a tone measurement instead",
		sg.analysisNote(),
	}
	if opt.LogFreq && twoSided {
		notes = append(notes, "drawn on a linear axis: a two-sided spectrum has no logarithmic one")
	}

	c := &Chart{
		Title:    title,
		Subtitle: subtitle,
		Width:    w + 150,
		Height:   h + 130,
		X:        Axis{Label: "time (s)", Min: 0, Max: endT},
		Y:        Axis{Label: yLabel, Min: minF, Max: maxF, Log: logFreq},
		Image:    buf.Bytes(),
		ColorBar: &ColorBar{Palette: p, Min: floor, Max: ceil, Label: "dBFS"},
		Notes:    notes,
	}
	return c
}

// analysisNote says what the analysis itself can resolve, so that the depth of
// the color scale is never read as the depth of the measurement.
func (sg *Spectrogram) analysisNote() string {
	binHz := 0.0
	if len(sg.Freqs) > 1 {
		binHz = sg.Freqs[1] - sg.Freqs[0]
	}
	// The transform length, back out of the bins kept: a two-sided spectrum
	// keeps all of them and a one-sided one keeps half plus the Nyquist bin.
	size := len(sg.Freqs)
	if !sg.TwoSided() && size > 1 {
		size = 2 * (size - 1)
	}
	n := fmt.Sprintf("%d point frames: %.3f Hz per bin, %.3f Hz of noise bandwidth",
		size, binHz, sg.NoiseBW)
	if sg.SidelobeDB < 0 {
		n += fmt.Sprintf(", window sidelobes %.0f dB down", sg.SidelobeDB)
	}
	return n
}

// hzString writes a frequency at a readable scale, for a label rather than for a
// measurement.
func hzString(f float64) string {
	switch a := math.Abs(f); {
	case a >= 1e9:
		return fmt.Sprintf("%.6f GHz", f/1e9)
	case a >= 1e6:
		return fmt.Sprintf("%.6f MHz", f/1e6)
	case a >= 1e3:
		return fmt.Sprintf("%.3f kHz", f/1e3)
	default:
		return fmt.Sprintf("%.0f Hz", f)
	}
}

// dataURI renders bytes as a base64 PNG data URI.
func dataURI(b []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}
