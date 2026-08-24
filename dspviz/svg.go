package dspviz

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"image/color"
	"io"
	"math"
	"slices"
	"strconv"
)

// Chart is a line plot. It is not a general plotting library: it draws the
// handful of charts this package needs, in enough varieties that a caller can
// retitle or restyle one before writing it.
type Chart struct {
	Title, Subtitle string
	Width, Height   int // zero selects 800 by 480
	X, Y            Axis
	Series          []Series

	// Notes are printed under the plot, one per line. Warnings from a Response
	// belong here: a plot that cannot show what it appears to should say so on
	// its face rather than in a return value someone dropped.
	Notes []string

	// Image, if set, is a PNG drawn to fill the plot area, under any series.
	// It is how a spectrogram gets axes: the standard library can draw the
	// pane or the text but not both, so the raster is embedded as a data URI
	// inside an otherwise ordinary chart.
	Image []byte

	// ColorBar, if set, is drawn down the right-hand side to say what the
	// colors in Image mean.
	ColorBar *ColorBar
}

// ColorBar is the key for a chart with an Image.
type ColorBar struct {
	Palette  Palette
	Min, Max float64
	Label    string
}

// Series is one line on a chart. A zero Color takes the next entry from
// SeriesColors, a zero Width selects 1.5 px, and Fill draws down to the axis,
// which is what an impulse plot wants.
type Series struct {
	Name   string
	X, Y   []float64
	Color  color.RGBA
	Width  float64
	Dashed bool
	Fill   bool
}

// SeriesColors returns the default color cycle, as a fresh slice so that no
// caller can reorder it for every other caller.
func SeriesColors() []color.RGBA {
	return []color.RGBA{
		{0x1f, 0x77, 0xb4, 0xff},
		{0xd6, 0x27, 0x28, 0xff},
		{0x2c, 0xa0, 0x2c, 0xff},
		{0xff, 0x7f, 0x0e, 0xff},
		{0x94, 0x67, 0xbd, 0xff},
		{0x8c, 0x56, 0x4b, 0xff},
	}
}

// Layout constants. The font is a monospace stack at a fixed size, so a label's
// width is its length times charWidth and no font metrics are needed -- which
// is the whole reason the output can be compared against a golden file on any
// machine.
const (
	colorBarWidth = 14.0

	fontSize  = 12.0
	charWidth = fontSize * 0.6
	padLeft   = 22.0
	padRight  = 18.0
	padTop    = 16.0
	padBottom = 46.0
)

// WriteSVG renders the chart. Every coordinate is written with two decimal
// places, far below a pixel at these sizes, so the same chart renders
// byte-identically on any machine and a golden file is a reasonable test.
func (c *Chart) WriteSVG(w io.Writer) error {
	width, height := c.Width, c.Height
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 480
	}

	x, y := c.X, c.Y
	xs := make([][]float64, 0, len(c.Series))
	ys := make([][]float64, 0, len(c.Series))
	for _, s := range c.Series {
		xs = append(xs, s.X)
		ys = append(ys, s.Y)
	}
	x.autoRange(xs)
	y.autoRange(ys)

	xt, yt := x.ticks(), y.ticks()

	// The left inset has to hold the widest y label plus the axis title.
	var widest int
	for _, t := range yt {
		if l := len(t.label()); l > widest {
			widest = l
		}
	}
	left := padLeft + float64(widest)*charWidth + 8
	if y.Label != "" {
		left += fontSize + 4
	}
	top := padTop + fontSize + 6
	if c.Title != "" {
		top += fontSize + 8
	}
	if c.Subtitle != "" {
		top += fontSize + 4
	}
	bottom := float64(height) - padBottom - float64(len(c.Notes))*(fontSize+2)
	right := float64(width) - padRight
	if c.ColorBar != nil {
		right -= colorBarWidth + 8 + float64(widestLabel(c.ColorBar))*charWidth + 10
	}
	if bottom <= top {
		bottom = top + 1
	}

	b := bufio.NewWriter(w)
	p := &printer{w: b}

	p.printf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" font-size="%g">`,
		width, height, width, height, fontSize)
	p.printf(`<rect width="%d" height="%d" fill="var(--bg, #ffffff)"/>`, width, height)
	p.printf(`<style>.ax{stroke:#444;fill:none}.gr{stroke:#e2e2e2;fill:none}.gm{stroke:#f0f0f0;fill:none}text{fill:#222}</style>`)

	if c.Title != "" {
		p.printf(`<text x="%s" y="%s" font-size="%g" font-weight="bold">`, num(left), num(padTop+fontSize+2), fontSize+3)
		p.text(c.Title)
		p.printf(`</text>`)
	}
	if c.Subtitle != "" {
		ty := padTop + fontSize + 2
		if c.Title != "" {
			ty += fontSize + 8
		}
		p.printf(`<text x="%s" y="%s" fill="#666">`, num(left), num(ty))
		p.text(c.Subtitle)
		p.printf(`</text>`)
	}

	p.printf(`<clipPath id="pane"><rect x="%s" y="%s" width="%s" height="%s"/></clipPath>`,
		num(left), num(top), num(right-left), num(bottom-top))

	if len(c.Image) > 0 {
		p.printf(`<image x="%s" y="%s" width="%s" height="%s" preserveAspectRatio="none" image-rendering="pixelated" href="%s"/>`,
			num(left), num(top), num(right-left), num(bottom-top), dataURI(c.Image))
	}

	// Grid and ticks.
	for _, t := range xt {
		px := left + x.pos(t.Value)*(right-left)
		if math.IsNaN(px) || px < left-0.01 || px > right+0.01 {
			continue
		}
		cls := "gr"
		if t.Minor {
			cls = "gm"
		}
		if len(c.Image) > 0 {
			// Over a raster a full-height rule hides the data. Tick outward
			// from the frame instead.
			p.printf(`<path class="ax" d="M%s %sv5"/>`, num(px), num(bottom))
		} else {
			p.printf(`<path class="%s" d="M%s %sV%s"/>`, cls, num(px), num(top), num(bottom))
		}
		if t.Minor {
			continue
		}
		p.printf(`<text x="%s" y="%s" text-anchor="middle">`, num(px), num(bottom+fontSize+6))
		p.text(t.label())
		p.printf(`</text>`)
	}
	for _, t := range yt {
		py := bottom - y.pos(t.Value)*(bottom-top)
		if math.IsNaN(py) || py < top-0.01 || py > bottom+0.01 {
			continue
		}
		cls := "gr"
		if t.Minor {
			cls = "gm"
		}
		if len(c.Image) > 0 {
			p.printf(`<path class="ax" d="M%s %sh-5"/>`, num(left), num(py))
		} else {
			p.printf(`<path class="%s" d="M%s %sH%s"/>`, cls, num(left), num(py), num(right))
		}
		if t.Minor {
			continue
		}
		p.printf(`<text x="%s" y="%s" text-anchor="end">`, num(left-6), num(py+fontSize*0.35))
		p.text(t.label())
		p.printf(`</text>`)
	}

	// Series, clipped to the pane so a line leaving the range keeps its slope
	// rather than being bent back along the frame.
	colors := SeriesColors()
	p.printf(`<g clip-path="url(#pane)">`)
	for i, s := range c.Series {
		col := s.Color
		if col.A == 0 {
			col = colors[i%len(colors)]
		}
		width := s.Width
		if width == 0 {
			width = 1.5
		}
		d := c.path(&x, &y, s, left, right, top, bottom)
		if d == "" {
			continue
		}
		dash := ""
		if s.Dashed {
			dash = ` stroke-dasharray="5 3"`
		}
		if s.Fill {
			p.printf(`<path d="%s" fill="%s" fill-opacity="0.25" stroke="none"/>`, d+closeToAxis(&y, left, right, bottom, top), rgb(col))
		}
		p.printf(`<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linejoin="round"%s/>`,
			d, rgb(col), num(width), dash)
	}
	p.printf(`</g>`)

	p.printf(`<rect class="ax" x="%s" y="%s" width="%s" height="%s" fill="none"/>`,
		num(left), num(top), num(right-left), num(bottom-top))

	if cb := c.ColorBar; cb != nil {
		bx := right + 14
		pal := cb.Palette
		if pal == nil {
			pal = ViridisPalette()
		}
		// One rectangle per two pixels of height: enough to read as a smooth
		// ramp, few enough to keep the document small.
		const stepPx = 2.0
		for y := top; y < bottom; y += stepPx {
			v := 1 - (y-top)/(bottom-top)
			p.printf(`<rect x="%s" y="%s" width="%s" height="%s" fill="%s"/>`,
				num(bx), num(y), num(colorBarWidth), num(stepPx+0.5), rgb(pal(v)))
		}
		p.printf(`<rect class="ax" x="%s" y="%s" width="%s" height="%s" fill="none"/>`,
			num(bx), num(top), num(colorBarWidth), num(bottom-top))
		// A degenerate range makes every tick position a 0/0. It happens for a
		// spectrogram of pure silence, where the floor and the ceiling meet, and
		// "NaN" in a coordinate makes the whole document fail to render rather
		// than losing one label.
		var ticks []Tick
		if cb.Max > cb.Min {
			ticks = NiceTicks(cb.Min, cb.Max, 6)
		}
		for _, t := range ticks {
			ty := bottom - (t.Value-cb.Min)/(cb.Max-cb.Min)*(bottom-top)
			if ty < top-0.01 || ty > bottom+0.01 {
				continue
			}
			p.printf(`<text x="%s" y="%s">`, num(bx+colorBarWidth+5), num(ty+fontSize*0.35))
			p.text(t.label())
			p.printf(`</text>`)
		}
		if cb.Label != "" {
			p.printf(`<text x="%s" y="%s" text-anchor="middle" fill="#444">`, num(bx+colorBarWidth/2), num(top-6))
			p.text(cb.Label)
			p.printf(`</text>`)
		}
	}

	if x.Label != "" {
		p.printf(`<text x="%s" y="%s" text-anchor="middle" fill="#444">`, num((left+right)/2), num(bottom+fontSize*2+10))
		p.text(x.Label)
		p.printf(`</text>`)
	}
	if y.Label != "" {
		ly := (top + bottom) / 2
		p.printf(`<text x="%s" y="%s" text-anchor="middle" fill="#444" transform="rotate(-90 %s %s)">`,
			num(padLeft-6), num(ly), num(padLeft-6), num(ly))
		p.text(y.Label)
		p.printf(`</text>`)
	}

	// Legend, only when there is something to tell apart.
	if len(c.Series) > 1 {
		lx, ly := right, top+fontSize
		for i, s := range slices.Backward(c.Series) {
			if s.Name == "" {
				continue
			}
			col := s.Color
			if col.A == 0 {
				col = colors[i%len(colors)]
			}
			w := float64(len(s.Name))*charWidth + 26
			lx -= w
			p.printf(`<path d="M%s %sh16" stroke="%s" stroke-width="2.5"/>`, num(lx), num(ly-4), rgb(col))
			p.printf(`<text x="%s" y="%s">`, num(lx+20), num(ly))
			p.text(s.Name)
			p.printf(`</text>`)
		}
	}

	// Notes sit below the x-axis label, which is why the pane was shortened by
	// one line per note above. A note longer than the pane is truncated rather
	// than being allowed to run off the edge of the document.
	fit := int((right - left) / (charWidth * (fontSize - 1) / fontSize))
	for i, n := range c.Notes {
		if fit > 1 && len(n) > fit {
			n = n[:fit-1] + "\u2026"
		}
		p.printf(`<text x="%s" y="%s" fill="#777" font-size="%g">`,
			num(left), num(bottom+fontSize*3+8+float64(i)*(fontSize+2)), fontSize-1)
		p.text(n)
		p.printf(`</text>`)
	}

	p.printf(`</svg>`)
	if p.err != nil {
		return p.err
	}
	return b.Flush()
}

// path builds the polyline for one series. A point that cannot be placed --
// NaN, or a non-positive value on a logarithmic axis -- breaks the line into a
// new subpath rather than being clamped, so a gap in the data reads as a gap.
// An infinity, which is what a true zero magnitude becomes in dB, is pinned
// just outside the pane so the clip removes it and the neighboring segments
// still slope towards it.
func (c *Chart) path(x, y *Axis, s Series, left, right, top, bottom float64) string {
	var buf []byte
	n := min(len(s.X), len(s.Y))
	pen := false
	for i := range n {
		px, py := x.pos(s.X[i]), y.pos(s.Y[i])
		if math.IsInf(py, -1) {
			py = -0.5
		} else if math.IsInf(py, 1) {
			py = 1.5
		}
		if math.IsNaN(px) || math.IsNaN(py) {
			pen = false
			continue
		}
		cx := left + px*(right-left)
		cy := bottom - py*(bottom-top)
		if pen {
			buf = append(buf, 'L')
		} else {
			buf = append(buf, 'M')
			pen = true
		}
		buf = strconv.AppendFloat(buf, cx, 'f', 2, 64)
		buf = append(buf, ' ')
		buf = strconv.AppendFloat(buf, cy, 'f', 2, 64)
	}
	return string(buf)
}

// closeToAxis extends a path down to the zero line, or to the bottom of the
// pane when zero is off it, so a filled series reads as area under the curve.
func closeToAxis(y *Axis, left, right, bottom, top float64) string {
	base := bottom
	if p := y.pos(0); !math.IsNaN(p) && p >= 0 && p <= 1 {
		base = bottom - p*(bottom-top)
	}
	return "L" + num(right) + " " + num(base) + "L" + num(left) + " " + num(base) + "Z"
}

func (t Tick) label() string {
	if t.Minor {
		return ""
	}
	if t.Label != "" {
		return t.Label
	}
	return strconv.FormatFloat(t.Value, 'g', 4, 64)
}

// widestLabel returns the longest tick label a color bar will need, so the
// layout can reserve room for it.
func widestLabel(cb *ColorBar) int {
	var w int
	for _, t := range NiceTicks(cb.Min, cb.Max, 6) {
		if l := len(t.label()); l > w {
			w = l
		}
	}
	return w
}

func num(v float64) string {
	return string(strconv.AppendFloat(nil, v, 'f', 2, 64))
}

func rgb(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// printer holds the first write error rather than checking every call, so the
// rendering above reads as a description of the picture.
type printer struct {
	w   *bufio.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

// text writes escaped character data. Everything user-supplied goes through
// here: a title with an ampersand in it must not be able to produce a document
// that does not parse.
func (p *printer) text(s string) {
	if p.err != nil {
		return
	}
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		p.err = err
		return
	}
	_, p.err = p.w.Write(buf.Bytes())
}
