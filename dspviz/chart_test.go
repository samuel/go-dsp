package dspviz

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samuel/go-dsp/dsp"
	"github.com/samuel/go-dsp/firpm"
)

// must unwraps a constructor that returns an error, for the call sites here
// that build a filter from arguments known to be valid. It panics rather than
// taking a *testing.T, because Go does not allow a multi-value call to sit
// alongside other arguments.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

var update = flag.Bool("update", false, "rewrite the golden chart files")

// parseXML reports whether an SVG document is well-formed. Every test that
// renders a chart goes through it, because an escaping bug produces a document
// that still looks plausible in a diff.
func parseXML(b []byte) error {
	d := xml.NewDecoder(bytes.NewReader(b))
	for {
		_, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// golden compares an SVG against the file checked in for it. The charts are
// byte-for-byte reproducible by construction -- every coordinate is written at
// two decimal places and no font metrics enter the layout -- which is what
// makes a golden file reasonable here rather than something to regenerate
// reflexively.
func golden(t *testing.T, name string, c *Chart) {
	t.Helper()
	var buf bytes.Buffer
	if err := c.WriteSVG(&buf); err != nil {
		t.Fatal(err)
	}

	// Whatever else is true of the output, it has to parse. A golden file would
	// happily enshrine an escaping bug; this will not.
	if err := parseXML(buf.Bytes()); err != nil {
		t.Fatalf("%s is not well-formed XML: %v", name, err)
	}

	path := filepath.Join("testdata", "golden", name+".svg")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run: go test ./dspviz -update)", err)
	}
	if !bytes.Equal(want, buf.Bytes()) {
		t.Errorf("%s differs from its golden file; run: go test ./dspviz -update", name)
	}
}

func testResponse(t *testing.T) *Response {
	t.Helper()
	const rate = 48000.0
	p, err := NewBiQuadProcessor(must(dsp.NewLowPassBiQuad[float64](rate, 1000, 0.7071)), rate)
	if err != nil {
		t.Fatal(err)
	}
	r, err := FreqResponse(p, LogGrid(20, 20000, 200), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestChartsGolden(t *testing.T) {
	r := testResponse(t)
	golden(t, "magnitude", MagnitudeChart(r, ChartOptions{}))
	golden(t, "passband", PassbandChart(r, [2]float64{20, 500}, ChartOptions{}))
	golden(t, "transition", TransitionChart(r, 1000, ChartOptions{}))
	golden(t, "stopband", StopbandChart(r, [2]float64{4000, 20000}, ChartOptions{}))
	golden(t, "phase", PhaseChart(r, false, ChartOptions{}))
	golden(t, "groupdelay", GroupDelayChart(r, ChartOptions{}))

	taps, _, err := firpm.Design(firpm.Spec{
		NumTaps: 41, SampleRate: 48000,
		Bands: []firpm.Band{{Lower: 0, Upper: 6000, Response: 1, Weight: 1},
			{Lower: 9000, Upper: 24000, Response: 0, Weight: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := NewFIRProcessor(taps, 48000)
	golden(t, "impulse", ImpulseChart(Impulse(p, 48), 48000, ChartOptions{}))
	golden(t, "step", StepChart(Step(p, 48), 48000, ChartOptions{}))

	// Residual phase over the passband, which is the only place it means
	// anything: a linear-phase filter is a pure delay there and should draw as
	// a flat line at zero.
	fr, err := FreqResponse(p, LinearGrid(100, 6000, 400), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "residual-phase", PhaseChart(fr, true, ChartOptions{Linear: true}))
	for i, v := range fr.ResidualPhase() {
		if math.Abs(v) > 1e-6 {
			t.Fatalf("residual phase at %.0f Hz is %v degrees; a linear-phase filter is flat", fr.Freq[i], v)
		}
	}

	// The spectrum chart is drawn through a deliberately nonlinear processor.
	// A linear filter distorts nothing, so its THD and SFDR are -Inf and the
	// subtitle -- which is most of what this golden pins -- would carry no
	// numbers at all.
	golden(t, "spectrum", SpectrumChart(distortedSpectrum(t), ChartOptions{}))
}

// distortedSpectrum measures a squarer followed by a 12-bit quantizer through
// the tone method. y = x + a*(2x^2-1) puts one harmonic at twice the
// fundamental, a decibels down; the quantizer then puts a floor under
// everything, which is what makes the three subtitle figures differ from one
// another and keeps the curve inside the pane. Both are arithmetic rather than
// random, so the golden does not depend on a generator's sequence.
func distortedSpectrum(t *testing.T) *Spectrum {
	t.Helper()
	const rate = 48000.0
	const a = 0.003
	const steps = 1 << 11 // 12 bits, so a floor near -74 dB
	p, err := NewFuncProcessor(func(dst, src []float64) []float64 {
		for _, x := range src {
			y := x + a*(2*x*x-1)
			dst = append(dst, math.Round(y*steps)/steps)
		}
		return dst
	}, nil, rate, rate)
	if err != nil {
		t.Fatal(err)
	}
	// A small frame deliberately: the chart draws one point per bin, and a
	// golden file has to stay readable.
	s, err := ToneSpectrum(p, 1000, SpectrumOptions{Frame: 512})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestSVGEscapes checks that nothing a caller puts in a title can produce a
// document that does not parse, or inject markup into one.
func TestSVGEscapes(t *testing.T) {
	c := &Chart{
		Title:    `a & b <script>alert("x")</script>`,
		Subtitle: "über <100 dB",
		X:        Axis{Label: "x & y"},
		Y:        Axis{Label: `"quoted"`},
		Series:   []Series{{Name: "<>&", X: []float64{0, 1}, Y: []float64{0, 1}}},
		Notes:    []string{"note & more"},
	}
	var buf bytes.Buffer
	if err := c.WriteSVG(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<script>") {
		t.Error("markup in a title reached the output unescaped")
	}
	d := xml.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		_, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed: %v", err)
		}
	}
}

// TestSVGHandlesInfinities checks the case every magnitude plot hits: a true
// zero is -Inf in dB, and it must neither reach the output as the text "-Inf"
// nor bend the neighboring segments.
func TestSVGHandlesInfinities(t *testing.T) {
	c := &Chart{
		X: Axis{Min: 0, Max: 4},
		Y: Axis{Min: -100, Max: 0},
		Series: []Series{{X: []float64{0, 1, 2, 3, 4},
			Y: []float64{-10, math.Inf(-1), -20, math.NaN(), -30}}},
	}
	var buf bytes.Buffer
	if err := c.WriteSVG(&buf); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, bad := range []string{"Inf", "NaN", "inf", "nan"} {
		if strings.Contains(s, bad) {
			t.Errorf("%q reached the output", bad)
		}
	}
	// The NaN has to break the line rather than being interpolated across.
	if n := strings.Count(s, "M"); n < 2 {
		t.Errorf("expected the NaN to start a new subpath, got %d move commands", n)
	}
}

func TestNiceTicks(t *testing.T) {
	for _, c := range []struct {
		lo, hi float64
		want   []string
	}{
		{0, 10, []string{"0", "2", "4", "6", "8", "10"}},
		{-1, 1, []string{"-1.0", "-0.5", "0.0", "0.5", "1.0"}},
		// The passband case: a hundredth of a decibel has to tick readably from
		// the same 1-2-5 family as everything else, and the labels have to carry
		// enough decimals to tell the ticks apart.
		{-0.02, 0.02, []string{"-0.02", "-0.01", "0.00", "0.01", "0.02"}},
		{-180, 0, []string{"-150", "-100", "-50", "0"}},
	} {
		var got []string
		for _, tk := range NiceTicks(c.lo, c.hi, 6) {
			got = append(got, tk.label())
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("NiceTicks(%v, %v) = %v, want %v", c.lo, c.hi, got, c.want)
		}
	}
	// An axis auto-ranged over floating-point noise must not ask for fourteen
	// decimal places; past six it goes to exponent form.
	for _, tk := range NiceTicks(-1e-13, 1e-13, 6) {
		if len(tk.label()) > 10 {
			t.Errorf("tick label %q is too wide; tiny ranges should use exponent form", tk.label())
		}
	}
	for _, tk := range NiceTicks(0, 1e9, 6) {
		if len(tk.label()) > 10 {
			t.Errorf("tick label %q is too wide; huge ranges should use exponent form", tk.label())
		}
	}

	// A degenerate range must still produce something rather than looping.
	if len(NiceTicks(5, 5, 6)) == 0 {
		t.Error("an empty range produced no ticks")
	}
	if len(NiceTicks(math.NaN(), 1, 6)) == 0 {
		t.Error("a NaN bound produced no ticks")
	}
}

func TestLogTicks(t *testing.T) {
	var majors []string
	for _, tk := range LogTicks(20, 20000) {
		if !tk.Minor {
			majors = append(majors, tk.label())
		}
	}
	want := "100,1k,10k"
	if got := strings.Join(majors, ","); got != want {
		t.Errorf("major log ticks = %v, want %v", got, want)
	}
	// Over a narrow span the subdivisions carry labels, because an axis that
	// spans one decade would otherwise have a single mark on it.
	var narrow []string
	for _, tk := range LogTicks(500, 2000) {
		if !tk.Minor {
			narrow = append(narrow, tk.label())
		}
	}
	if got, want := strings.Join(narrow, ","), "500,1k,2k"; got != want {
		t.Errorf("labeled ticks over half a decade either side of 1k = %v, want %v", got, want)
	}

	// Three to five decades draw them as an unlabelled reading aid, and beyond
	// that they are dropped rather than becoming a grey wash.
	mid, wide := 0, 0
	for _, tk := range LogTicks(20, 20000) {
		if tk.Minor {
			mid++
		}
	}
	for _, tk := range LogTicks(1e-3, 1e9) {
		if tk.Minor {
			wide++
		}
	}
	if mid == 0 {
		t.Error("no minor ticks over three decades")
	}
	if wide != 0 {
		t.Error("minor ticks over twelve decades would be unreadable")
	}
	if LogTicks(0, 10) != nil || LogTicks(10, 1) != nil {
		t.Error("a logarithmic axis through zero or reversed should give no ticks")
	}
}

// TestSeriesColorsAreCopies enforces the same rule the window coefficients in
// dsp follow: nothing exported is a table a caller can reorder for everyone
// else in the process.
func TestSeriesColorsAreCopies(t *testing.T) {
	a := SeriesColors()
	first := a[0]
	a[0] = a[1]
	if b := SeriesColors(); b[0] != first {
		t.Error("SeriesColors hands out the same slice every time")
	}
}

func TestChartAutoRange(t *testing.T) {
	var a Axis
	a.autoRange([][]float64{{1, 2, 3}})
	if a.Min > 1 || a.Max < 3 {
		t.Errorf("auto range %v..%v does not contain the data", a.Min, a.Max)
	}
	// All-infinite data must not produce an infinite axis.
	var b Axis
	b.autoRange([][]float64{{math.Inf(-1), math.NaN()}})
	if math.IsInf(b.Min, 0) || math.IsInf(b.Max, 0) || b.Min >= b.Max {
		t.Errorf("auto range over unusable data gave %v..%v", b.Min, b.Max)
	}
	var c Axis
	c.Log = true
	c.autoRange([][]float64{{-1, 0}})
	if c.Min <= 0 {
		t.Errorf("a logarithmic axis auto-ranged to %v", c.Min)
	}
}

// TestColorBarDegenerateRange checks that a color bar whose two ends meet does
// not put "NaN" in a coordinate.
//
// A spectrogram of pure silence has a floor equal to its ceiling, and every
// tick position is then a 0/0. One NaN makes the whole document fail to render
// rather than losing a label, which is the same trap the JSON path has with an
// infinity.
func TestColorBarDegenerateRange(t *testing.T) {
	c := &Chart{
		Title:    "silence",
		X:        Axis{Label: "time (s)", Min: 0, Max: 1},
		Y:        Axis{Label: "Hz", Min: 0, Max: 1},
		ColorBar: &ColorBar{Palette: ViridisPalette(), Min: -60, Max: -60, Label: "dBFS"},
	}
	var buf bytes.Buffer
	if err := c.WriteSVG(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "NaN") {
		t.Errorf("the document contains NaN:\n%s", buf.String())
	}
	if err := xml.Unmarshal(buf.Bytes(), new(any)); err != nil {
		t.Errorf("the document does not parse: %v", err)
	}
}
