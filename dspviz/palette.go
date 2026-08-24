package dspviz

import (
	"image/color"
	"math"
)

// Palette maps a value in [0, 1] to a color. Anything outside is clamped.
type Palette func(v float64) color.RGBA

// The stop tables are unexported arrays behind functions that build a closure
// over a copy, the same discipline the window coefficients in dsp follow: an
// exported table is one any importer can reorder for every other importer in
// the process.
var (
	viridisStops = [...]color.RGBA{
		{0x44, 0x01, 0x54, 0xff}, {0x48, 0x18, 0x6a, 0xff}, {0x47, 0x2d, 0x7b, 0xff},
		{0x43, 0x40, 0x87, 0xff}, {0x3b, 0x52, 0x8b, 0xff}, {0x33, 0x63, 0x8d, 0xff},
		{0x2c, 0x72, 0x8e, 0xff}, {0x26, 0x82, 0x8e, 0xff}, {0x21, 0x91, 0x8c, 0xff},
		{0x1f, 0xa0, 0x88, 0xff}, {0x28, 0xae, 0x80, 0xff}, {0x3f, 0xbc, 0x73, 0xff},
		{0x5e, 0xc9, 0x62, 0xff}, {0x84, 0xd4, 0x4b, 0xff}, {0xad, 0xdc, 0x30, 0xff},
		{0xd8, 0xe2, 0x19, 0xff}, {0xfd, 0xe7, 0x25, 0xff},
	}

	// heatStops is the waterfall gradient used elsewhere: black
	// through blue and yellow to a deep red.
	heatStops = [...]color.RGBA{
		{0x00, 0x00, 0x00, 0xff}, {0x00, 0x00, 0x20, 0xff}, {0x00, 0x00, 0x30, 0xff},
		{0x00, 0x00, 0x50, 0xff}, {0x00, 0x00, 0x91, 0xff}, {0x1e, 0x90, 0xff, 0xff},
		{0xff, 0xff, 0x00, 0xff}, {0xfe, 0x6d, 0x16, 0xff}, {0xff, 0x00, 0x00, 0xff},
		{0xc6, 0x00, 0x00, 0xff}, {0x9f, 0x00, 0x00, 0xff}, {0x75, 0x00, 0x00, 0xff},
		{0x4a, 0x00, 0x00, 0xff},
	}
)

// ViridisPalette returns the perceptually uniform default. It is the default
// rather than the heat map because a gradient whose lightness does not rise
// steadily invents features: an eye reads the boundary between two similarly
// bright hues as an edge, so a heat map draws contours across a smooth slope.
// Viridis has none.
func ViridisPalette() Palette { return lerpPalette(viridisStops[:]) }

// HeatPalette returns the black-blue-yellow-red waterfall gradient. It is
// louder than viridis and better at making a single faint line jump out of a
// black background, which is what a sweep plot is mostly for.
func HeatPalette() Palette { return lerpPalette(heatStops[:]) }

// GrayPalette returns a plain black to white ramp.
func GrayPalette() Palette {
	return func(v float64) color.RGBA {
		g := uint8(math.Round(clamp01(v) * 255))
		return color.RGBA{g, g, g, 0xff}
	}
}

// PaletteByName returns the palette that name selects.
//
// The second result is false for a name that is not one of them, so a caller
// that can report a typo does, rather than drawing a different picture than the
// one that was asked for. A caller that cannot -- a server reading a query
// parameter -- picks its own default instead.
func PaletteByName(name string) (Palette, bool) {
	switch name {
	case "viridis":
		return ViridisPalette(), true
	case "heat":
		return HeatPalette(), true
	case "gray", "grey":
		return GrayPalette(), true
	}
	return nil, false
}

// PaletteNames returns the names PaletteByName accepts, one per palette, in a
// fresh slice. The alternative spelling of gray is not among them.
func PaletteNames() []string {
	return []string{"viridis", "heat", "gray"}
}

// lerpPalette interpolates between stops spread evenly over [0, 1].
//
// The last stop is reached at v == 1 rather than being squeezed into the final
// fraction of the range: scaling by the number of stops plus one and clamping
// compresses the top two stops into the last few percent.
func lerpPalette(stops []color.RGBA) Palette {
	s := append([]color.RGBA(nil), stops...)
	return func(v float64) color.RGBA {
		v = clamp01(v)
		if len(s) == 1 {
			return s[0]
		}
		x := v * float64(len(s)-1)
		i := int(x)
		if i >= len(s)-1 {
			return s[len(s)-1]
		}
		t := x - float64(i)
		a, b := s[i], s[i+1]
		return color.RGBA{
			R: lerp8(a.R, b.R, t),
			G: lerp8(a.G, b.G, t),
			B: lerp8(a.B, b.B, t),
			A: 0xff,
		}
	}
}

func lerp8(a, b uint8, t float64) uint8 {
	return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t))
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(1, v))
}
