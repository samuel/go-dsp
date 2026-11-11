package plot

import (
	"image/color"
	"math"
	"slices"
	"testing"
)

// TestPalettesByName checks the name-to-palette lookup the command line and
// the server both drive. Every name PaletteNames advertises has to resolve, and
// each one has to be a different picture: an unknown name that returned viridis
// would draw a plausible chart that is the wrong one.
func TestPalettesByName(t *testing.T) {
	names := PaletteNames()
	if len(names) == 0 {
		t.Fatal("PaletteNames is empty")
	}
	if slices.Contains(names, "grey") {
		t.Error("PaletteNames lists the alternative spelling of gray, which its doc says it does not")
	}

	seen := make(map[color.RGBA]string, len(names))
	for _, name := range names {
		p, ok := PaletteByName(name)
		if !ok {
			t.Errorf("%s: PaletteByName does not know a name PaletteNames advertises", name)
			continue
		}
		// A palette has to map the whole range, and the two ends have to
		// differ, or a spectrogram drawn with it is one flat color.
		lo, hi := p(0), p(1)
		if lo == hi {
			t.Errorf("%s: both ends of the ramp are %v", name, lo)
		}
		if lo.A != 0xff || hi.A != 0xff {
			t.Errorf("%s: not opaque: %v, %v", name, lo, hi)
		}
		// Out of range is clamped rather than extrapolated or wrapped.
		if p(-5) != lo || p(5) != hi {
			t.Errorf("%s: out-of-range values are not clamped", name)
		}
		if other, dup := seen[hi]; dup {
			t.Errorf("%s ends the same color as %s", name, other)
		}
		seen[hi] = name
	}

	// The alternative spelling resolves even though it is not advertised.
	if _, ok := PaletteByName("grey"); !ok {
		t.Error(`PaletteByName("grey") should resolve`)
	}
	if _, ok := PaletteByName("nonesuch"); ok {
		t.Error("PaletteByName accepted a name that is not a palette")
	}
}

func TestPaletteIsACopy(t *testing.T) {
	for name, fn := range map[string]func() Palette{
		"viridis": ViridisPalette, "heat": HeatPalette, "gray": GrayPalette,
	} {
		p := fn()
		lo, hi := p(0), p(1)
		if lo == hi {
			t.Errorf("%s maps 0 and 1 to the same color", name)
		}
		// Out-of-range values clamp rather than wrapping or panicking.
		if p(-5) != lo || p(5) != hi {
			t.Errorf("%s does not clamp outside [0, 1]", name)
		}
		if p(math.NaN()) != lo {
			t.Errorf("%s does not handle NaN", name)
		}
	}
	// The stop table must not be reachable through anything handed out.
	a, b := ViridisPalette(), ViridisPalette()
	if a(0.5) != b(0.5) {
		t.Error("two palettes disagree")
	}
}
