// Package cliflags holds the flags cmd/dspviz and cmd/sigviz share: chart size,
// decibel floor, palette, title and quiet.
//
// The two commands measure different things, a filter and a file, so most of
// their flags differ. What they share is what happens to a chart once it
// exists, one set of help text and one palette lookup instead of two.
package cliflags

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/samuel/go-dsp/dspviz"
	"github.com/samuel/go-dsp/internal/clifile"
)

// Output is the set of chart flags both commands take.
type Output struct {
	Width, Height int
	FloorDB       float64
	PaletteName   string
	Title         string
	Quiet         bool
}

// Register adds the flags to fs. defaultPalette differs by command: heat for
// dspviz, viridis for sigviz.
func (o *Output) Register(fs *flag.FlagSet, defaultPalette string) {
	fs.IntVar(&o.Width, "width", 0, "chart width in pixels")
	fs.IntVar(&o.Height, "height", 0, "chart height in pixels")
	fs.Float64Var(&o.FloorDB, "floor", 0, "decibel floor of a chart, and of a spectrogram's color scale (0 chooses one)")
	fs.StringVar(&o.PaletteName, "palette", defaultPalette,
		"spectrogram palette: "+strings.Join(dspviz.PaletteNames(), ", "))
	fs.StringVar(&o.Title, "title", "", "chart title, overriding the one each chart picks")
	fs.BoolVar(&o.Quiet, "quiet", false, "do not print the summary")
}

// Palette resolves the name, naming the alternatives when it is not one of
// them. Call it before any work so a typo reports early.
func (o *Output) Palette() (dspviz.Palette, error) {
	p, ok := dspviz.PaletteByName(o.PaletteName)
	if !ok {
		return nil, fmt.Errorf("unknown palette %q, want one of %s",
			o.PaletteName, strings.Join(dspviz.PaletteNames(), ", "))
	}
	return p, nil
}

// ChartOptions is the subset of these flags a line chart takes.
func (o *Output) ChartOptions() dspviz.ChartOptions {
	return dspviz.ChartOptions{
		Title:   o.Title,
		Width:   o.Width,
		Height:  o.Height,
		FloorDB: o.FloorDB,
	}
}

// MakeDir creates the directory a set of charts is written into.
func MakeDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// Write writes one line chart to path, which must name an SVG or be standard
// output. A PNG has no text, so it is rejected.
func Write(path string, c *dspviz.Chart) error {
	if err := clifile.CheckExt(path, "svg"); err != nil {
		return err
	}
	return clifile.Write(path, c.WriteSVG)
}
