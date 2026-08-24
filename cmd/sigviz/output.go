package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"

	"github.com/samuel/go-dsp/dsp"
	"github.com/samuel/go-dsp/dspviz"
	"github.com/samuel/go-dsp/internal/clifile"
	"github.com/samuel/go-dsp/internal/cliflags"
)

// outputFlags are the flags that say what to draw and where to put it.
//
// If you know sox spectrogram: -x is -width, -X is -pps, -y and -Y are -height,
// -z is -range, -Z is -ceiling, -w is -window, -d is -duration, -S is -start,
// -m is -channel mix, -r is -bare, -t is -title and -o is -o. Names here are
// words rather than sox's case-sensitive letters: -z and -Z would invert the
// color scale silently.
type outputFlags struct {
	// The flags dspviz shares: width, height, floor, palette, title and quiet,
	// with one help text between the two commands.
	cliflags.Output

	out string
	dir string

	pps float64
	fft int
	hop int
	win string

	ceiling float64
	rangeDB float64
	center  float64

	bare bool
	json bool

	// notes is whatever stftOptions had to say about the numbers it was given,
	// so a clamp is reported rather than applied silently.
	notes []string
}

func (o *outputFlags) register(fs *flag.FlagSet) {
	o.Register(fs, "viridis")
	fs.StringVar(&o.out, "o", "", "file to write, or - for standard output")
	fs.StringVar(&o.dir, "out", "", "directory to write a whole set into")
	fs.Float64Var(&o.pps, "pps", 0, "spectrogram columns per second")
	fs.IntVar(&o.fft, "fft", 0, "transform size in samples (0 chooses one from the rate)")
	fs.IntVar(&o.hop, "hop", 0, "samples between frames (0 chooses one)")
	fs.StringVar(&o.win, "window", "kaiser", "analysis window: "+strings.Join(windowNames(), ", "))
	fs.Float64Var(&o.ceiling, "ceiling", 0, "top of the spectrogram color scale in dBFS")
	fs.Float64Var(&o.rangeDB, "range", 120, "decibels of color scale below the ceiling, when -floor does not say")
	fs.Float64Var(&o.center, "center", 0, "tuner frequency in Hz, for labeling a two-sided spectrogram")
	fs.BoolVar(&o.bare, "bare", false, "write the spectrogram as a bare raster, with no axes or color bar")
	fs.BoolVar(&o.json, "json", false, "print the statistics as JSON")
}

func (o *outputFlags) check() error {
	if o.out != "" && o.dir != "" {
		return errors.New("-o writes one file and -out writes a directory of them; pass one or the other")
	}
	return nil
}

func (o *outputFlags) chartOptions() dspviz.ChartOptions {
	// Linear frequency, unlike a filter measurement: a recording's spectrum reads
	// against the tones in it rather than across decades.
	opt := o.ChartOptions()
	opt.Linear = true
	return opt
}

// spectrogramOptions builds the color scale.
//
// A ceiling of zero means full scale, the opposite of dspviz's automatic
// ceiling for a filter: a filter with gain needs the head room, while two
// recordings rescaled to their own loudest moments would come out looking alike
// and neither picture would say which was louder.
func (o *outputFlags) spectrogramOptions(title string) dspviz.SpectrogramOptions {
	opt := dspviz.SpectrogramOptions{
		Title:  title,
		Width:  o.Width,
		Height: o.Height,
	}
	if o.Title != "" {
		opt.Title = o.Title
	}
	rng := o.rangeDB
	if rng <= 0 {
		rng = 120
	}
	if o.ceiling == 0 {
		opt.FullScale = true
		opt.FloorDB = -rng
	} else {
		opt.CeilDB = o.ceiling
		opt.FloorDB = o.ceiling - rng
	}
	// An explicit floor wins over the one derived from -range.
	if o.FloorDB != 0 {
		opt.FloorDB = o.FloorDB
	}
	return opt
}

// stftOptions sizes the frames.
//
// The default is rate-relative, which no constant number of points can be: 2048
// points is 43 ms at 48 kHz and 186 ms at 11 kHz, and the second averages
// 300 baud keying into two steady lines. An explicit -height matches bins to
// pixels, and -fft wins over both.
func (o *outputFlags) stftOptions(rate float64) (dspviz.STFTOptions, error) {
	if o.fft == 1 || o.fft < 0 {
		return dspviz.STFTOptions{}, fmt.Errorf("-fft %d: the frame size must be at least 2, or 0 for the default", o.fft)
	}
	size := dspviz.FrameSize(rate, 0.025)
	switch {
	case o.fft > 0:
		size = o.fft
	case o.Height > 0:
		size = 256
		for size/2+1 < o.Height && size < 1<<16 {
			size <<= 1
		}
	}

	hop := o.hop
	if hop <= 0 && o.pps > 0 {
		hop = int(math.Round(rate / o.pps))
	}
	if hop <= 0 {
		hop = size / 4
	}
	// A hop wider than half a frame skips samples no frame ever covers, which is
	// how sox's spectrogram examines two per cent of a long file and misses a
	// hundred millisecond burst. Clamp it; compress time with the column
	// reducer, which looks at everything.
	if hop > size/2 {
		o.notes = append(o.notes, fmt.Sprintf("a hop of %d samples would skip past whole frames; using %d", hop, size/2))
		hop = size / 2
	}

	w, err := windowFunc(o.win)
	if err != nil {
		return dspviz.STFTOptions{}, err
	}

	width := o.Width
	if width <= 0 {
		width = 900
	}
	opt := dspviz.STFTOptions{
		Size:   size,
		Hop:    hop,
		Window: w,
		Center: o.center,
		// Twice the pane, so a stream of unknown length reduces to between one and
		// two columns per pixel and no peak is averaged away.
		Columns: 2 * width,
	}
	return opt, nil
}

// chartPath is where a named chart goes: into the directory when there is one,
// and otherwise to -o, which may be standard output.
//
// label names the channel a multi-channel run is writing, empty for one
// covering a single channel.
func (o *outputFlags) chartPath(name, label, ext string) (string, error) {
	if label != "" {
		name += "-" + label
	}
	if o.dir != "" {
		return filepath.Join(o.dir, name+"."+ext), nil
	}
	return labelPath(o.out, label)
}

// labelPath puts a channel label into a path given to -o, so out.svg becomes
// out-ch0.svg and out-ch1.svg for a stereo file.
//
// Standard output cannot take two documents, so a label there is an error
// rather than a silent choice of the first channel.
func labelPath(path, label string) (string, error) {
	if label == "" {
		return path, nil
	}
	if path == "" || path == "-" {
		return "", errors.New("standard output takes one chart, and this file has several channels; " +
			"pick one with -channel N, fold them with -channel mix, or write a set with -out <dir>")
	}
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "-" + label + ext, nil
}

// writeChart writes a line chart to wherever this run's flags put it.
func (o *outputFlags) writeChart(name, label string, c *dspviz.Chart) error {
	path, err := o.chartPath(name, label, "svg")
	if err != nil {
		return err
	}
	if o.bare {
		c.Title, c.Subtitle, c.Notes = "", "", nil
	}
	return cliflags.Write(path, c)
}

// writeSpectrogram writes the raster, with axes unless -bare asked for the pane
// on its own.
func (o *outputFlags) writeSpectrogram(name, label string, sg *dspviz.Spectrogram, opt dspviz.SpectrogramOptions) error {
	pal, err := o.Palette()
	if err != nil {
		return err
	}
	if o.dir != "" {
		svg, err := o.chartPath(name, label, "svg")
		if err != nil {
			return err
		}
		png, err := o.chartPath(name, label, "png")
		if err != nil {
			return err
		}
		if err := clifile.Write(svg, func(w io.Writer) error {
			return sg.WriteSVG(w, pal, opt)
		}); err != nil {
			return err
		}
		return clifile.Write(png, func(w io.Writer) error {
			return sg.WritePNG(w, pal, opt)
		})
	}
	dst, err := labelPath(o.out, label)
	if err != nil {
		return err
	}
	if err := clifile.CheckExt(dst, "svg", "png"); err != nil {
		return err
	}
	// -bare is a bare raster, not a document; writing PNG bytes into a path named
	// .svg would be worse than saying so.
	if o.bare && clifile.Ext(dst) == "svg" {
		return errors.New("-bare writes a bare raster; give -o a .png path, or leave it off for standard output")
	}
	raster := o.bare || clifile.Ext(dst) == "png"
	return clifile.Write(dst, func(w io.Writer) error {
		if raster {
			return sg.WritePNG(w, pal, opt)
		}
		return sg.WriteSVG(w, pal, opt)
	})
}

// windowFunc returns the analysis window a name selects. The empty name and
// "kaiser" give nil, dspviz's own window: a Kaiser quiet enough that the plot's
// floor is the arithmetic rather than the window.
func windowFunc(name string) (func([]float64), error) {
	switch strings.ToLower(name) {
	case "", "kaiser":
		return nil, nil
	case "rectangular", "none":
		return func(w []float64) {
			for i := range w {
				w[i] = 1
			}
		}, nil
	case "triangle":
		return func(w []float64) { dsp.TriangleWindow(w) }, nil
	case "hann", "hanning":
		return func(w []float64) { dsp.HannWindow(w) }, nil
	case "hamming":
		return func(w []float64) { dsp.HammingWindow(w) }, nil
	case "blackman":
		return func(w []float64) { dsp.BlackmanWindow(w) }, nil
	case "blackman-harris":
		return func(w []float64) { dsp.BlackmanHarrisWindow(w) }, nil
	case "nuttall":
		return func(w []float64) { dsp.NuttallWindow(w) }, nil
	case "flattop":
		return func(w []float64) { dsp.FlatTopWindow(w) }, nil
	}
	return nil, fmt.Errorf("unknown window %q, want one of %s", name, strings.Join(windowNames(), ", "))
}

func windowNames() []string {
	return []string{"kaiser", "rectangular", "triangle", "hann", "hamming",
		"blackman", "blackman-harris", "nuttall", "flattop"}
}
