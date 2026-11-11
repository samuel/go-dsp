package viz

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"samuelks.com/dsp"
	"samuelks.com/dsp/analysis"
)

// TestSignalReportJSONCarriesARealAnalysis is the end-to-end half of the JSON
// tests. The other two build the structs by hand, which pins the encoder but
// not the wiring: a spectral section that a real run never attaches would pass
// them both. This runs the accumulators over a signal and requires the section
// and its notes to reach the document.
func TestSignalReportJSONCarriesARealAnalysis(t *testing.T) {
	const rate = 8000.0

	// A real signal handed over as I/Q, which is the mistake SpectralStats
	// exists to catch, so the spectral section comes back with a note in it.
	x := builderSignal(8192)
	z := make([]complex128, len(x))
	for i, v := range x {
		z[i] = complex(v, v)
	}

	stats, err := analysis.StatsOfComplex(z, analysis.StatsOptions{Rate: rate})
	if err != nil {
		t.Fatal(err)
	}
	psd, err := analysis.PSDOfComplex(z, rate, analysis.STFTOptions{Size: 512, Complex: true})
	if err != nil {
		t.Fatal(err)
	}
	stats.Spectral = analysis.SpectralStatsOf(psd)
	if len(stats.Spectral.Notes) == 0 {
		t.Fatal("a real signal read as I/Q produced no spectral note, so this fixture proves nothing")
	}

	pair, err := func() (*analysis.ChannelPairStats, error) {
		b := analysis.NewPairBuilder(0, 1)
		if err := b.Write(x, x); err != nil {
			return nil, err
		}
		return b.Close()
	}()
	if err != nil {
		t.Fatal(err)
	}
	if len(pair.Notes) == 0 {
		t.Fatal("two identical channels produced no pair note, so this fixture proves nothing")
	}

	rep := &SignalReport{Channels: []*analysis.SignalStats{stats}, Pairs: []*analysis.ChannelPairStats{pair}}
	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}

	channels, _ := got["channels"].([]any)
	if len(channels) != 1 {
		t.Fatalf("channels = %v, want one", got["channels"])
	}
	ch, _ := channels[0].(map[string]any)
	sp, ok := ch["spectral"].(map[string]any)
	if !ok {
		t.Fatalf("no spectral section: %v", ch["spectral"])
	}
	if n, _ := sp["notes"].([]any); len(n) != len(stats.Spectral.Notes) {
		t.Errorf("spectral notes = %v, want %d", sp["notes"], len(stats.Spectral.Notes))
	}
	if _, ok := ch["iq"].(map[string]any); !ok {
		t.Errorf("no iq section for a complex analysis: %v", ch["iq"])
	}
	pairs, _ := got["pairs"].([]any)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %v, want one", got["pairs"])
	}
	pj, _ := pairs[0].(map[string]any)
	if n, _ := pj["notes"].([]any); len(n) != len(pair.Notes) {
		t.Errorf("pair notes = %v, want %d", pj["notes"], len(pair.Notes))
	}

	// And the text summary, which is the other reader of the same document and
	// has its own copy of which sections exist.
	var text bytes.Buffer
	if err := rep.WriteSummary(&text); err != nil {
		t.Fatal(err)
	}
	for _, want := range append(append([]string{}, stats.Spectral.Notes...), pair.Notes...) {
		if !strings.Contains(text.String(), want) {
			t.Errorf("the summary omits %q:\n%s", want, text.String())
		}
	}
}

// TestKindByNameCoversTheCatalog checks the lookup against the catalog itself,
// so an entry added to one and not reachable through the other cannot pass.
func TestKindByNameCoversTheCatalog(t *testing.T) {
	for _, k := range Catalog() {
		got, ok := KindByName(k.Name)
		if !ok {
			t.Errorf("%s: KindByName does not find a catalog entry", k.Name)
			continue
		}
		if got.Label != k.Label || len(got.Params) != len(k.Params) {
			t.Errorf("%s: KindByName returned a different entry", k.Name)
		}
	}
	if _, ok := KindByName("nonesuch"); ok {
		t.Error("KindByName accepted a name that is not in the catalog")
	}
	if _, ok := KindByName(""); ok {
		t.Error("KindByName accepted an empty name")
	}
}

// TestReportWriteSummary covers the text the command line prints, which is the
// only reader of most of Metrics. It is written line by line from optional
// fields, so a report missing a section has to print the rest rather than a
// row of zeros.
func TestReportWriteSummary(t *testing.T) {
	const rate = 48000.0
	p := must(dsp.NewBiquadLowPass[float64](rate, 1000, 0.7071))
	rep, err := Analyze(p, rate, ReportOptions{
		Points: 128, Frame: 4096, ToneHz: 997, SweepSec: -1,
		Passband: [2]float64{20, 500}, Stopband: [2]float64{4000, 20000},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := rep.WriteSummary(&buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"rate          48000 Hz in, 48000 Hz out",
		"method        ",
		"cutoff        ",
		"passband      20-500 Hz",
		"stopband      4000-20000 Hz",
		"group delay   ",
		// The tone is snapped to an odd bin, so it is near 997 rather than at
		// it; that is CoherentFreq's doing and the summary reports where the
		// measurement was actually made.
		"SFDR ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary omits %q:\n%s", want, got)
		}
	}
	// Every line has to carry a number rather than a NaN or an infinity, which
	// is what an empty band or a silent tone would leave behind.
	for _, bad := range []string{"NaN", "Inf"} {
		if strings.Contains(got, bad) {
			t.Errorf("the summary holds %s:\n%s", bad, got)
		}
	}

	// A report with no tone measurement and no bands prints the rest.
	bare := &Report{Rates: [2]float64{rate, rate}, Response: rep.Response, Warnings: []string{"a note"}}
	buf.Reset()
	if err := bare.WriteSummary(&buf); err != nil {
		t.Fatal(err)
	}
	got = buf.String()
	// The column spacing is part of the match: "passband" also appears in the
	// group delay line, which every report has.
	if strings.Contains(got, "passband  ") || strings.Contains(got, "SFDR") {
		t.Errorf("a bare report printed a section it has no numbers for:\n%s", got)
	}
	if !strings.Contains(got, "note          a note") {
		t.Errorf("the warning did not print:\n%s", got)
	}
}

// TestSignalReportOneChannelReadsAsOne keeps the common case unadorned: a mono
// recording's summary is its channel's summary, with no heading inviting the
// reader to look for a second one.
func TestSignalReportOneChannelReadsAsOne(t *testing.T) {
	b, err := analysis.NewStatsBuilder(analysis.StatsOptions{Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Write([]float64{0.5, -0.5, 0.25, -0.25}); err != nil {
		t.Fatal(err)
	}
	s := closedStats(t, b)

	var one, rep bytes.Buffer
	if err := WriteStatsSummary(&one, s); err != nil {
		t.Fatal(err)
	}
	if err := (&SignalReport{Channels: []*analysis.SignalStats{s}}).WriteSummary(&rep); err != nil {
		t.Fatal(err)
	}
	if one.String() != rep.String() {
		t.Errorf("a one-channel report differs from the channel's own summary:\n%s\n---\n%s", one.String(), rep.String())
	}
}

// TestSignalReportJSONSurvivesSilence is the infinity trap one level further
// down than SignalStats meets it: a channel list is a slice of objects, and a
// sanitizer that walks only maps leaves every one of them untouched.
func TestSignalReportJSONSurvivesSilence(t *testing.T) {
	b, err := analysis.NewStatsBuilder(analysis.StatsOptions{Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Write(make([]float64, 200)); err != nil {
		t.Fatal(err)
	}
	silent := closedStats(t, b)
	if !math.IsInf(silent.PeakDB, -1) {
		t.Fatal("a silent channel should peak at -Inf dBFS")
	}

	rep := &SignalReport{
		Channels: []*analysis.SignalStats{silent, silent},
		Pairs:    []*analysis.ChannelPairStats{{A: 0, B: 1, BalanceDB: math.Inf(1)}},
	}
	var buf bytes.Buffer
	if err := rep.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var doc struct {
		Channels []map[string]any `json:"channels"`
		Pairs    []map[string]any `json:"pairs"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if len(doc.Channels) != 2 || len(doc.Pairs) != 1 {
		t.Fatalf("got %d channels and %d pairs, want 2 and 1", len(doc.Channels), len(doc.Pairs))
	}
	for i, c := range doc.Channels {
		if c["peakDB"].(float64) >= 0 {
			t.Errorf("channel %d peakDB came out as %v, want it pinned to the bottom", i, c["peakDB"])
		}
	}
	if doc.Pairs[0]["balanceDB"].(float64) <= 0 {
		t.Errorf("balanceDB came out as %v, want it pinned to the top", doc.Pairs[0]["balanceDB"])
	}
}

// TestStatsClipping is the trap a naive counter falls into: a peak-normalized
// file has a sample on the rail by construction, and reporting that as clipping
// makes the number useless.
func TestStatsClipping(t *testing.T) {
	const bits = 16
	full := math.Ldexp(1, bits-1)
	rail := (full - 1) / full

	// Seven samples on the rail in three runs, of four, two and one.
	x := make([]float64, 200)
	for i := range x {
		x[i] = 0.25
	}
	for _, r := range [][2]int{{10, 4}, {50, 2}, {90, 1}} {
		for i := range r[1] {
			x[r[0]+i] = rail
		}
	}
	s := statsOf(t, x, analysis.StatsOptions{Rate: 8000, Bits: bits})
	if s.Clipped != 7 {
		t.Errorf("%d samples on the rail, want 7", s.Clipped)
	}
	if s.ClippedRuns != 3 {
		t.Errorf("%d runs, want 3", s.ClippedRuns)
	}
	if s.SustainedRuns != 1 {
		t.Errorf("%d sustained runs, want 1 (only the run of four is longer than %d)", s.SustainedRuns, analysis.MinClipRun)
	}

	// A peak-normalized file: one sample at the rail and nothing else. It is not
	// clipped, and the summary must not say it is.
	y := make([]float64, 200)
	for i := range y {
		y[i] = 0.5 * math.Sin(float64(i))
	}
	y[7] = rail
	p := statsOf(t, y, analysis.StatsOptions{Rate: 8000, Bits: bits})
	if p.SustainedRuns != 0 {
		t.Errorf("a peak-normalized file reports %d sustained runs, want 0", p.SustainedRuns)
	}
	var buf bytes.Buffer
	if err := WriteStatsSummary(&buf, p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "clipping      none") {
		t.Errorf("the summary does not say the file is unclipped:\n%s", buf.String())
	}
}

// TestStatsJSONSurvivesSilence checks that WriteJSON can serialize a
// stats document for an all-zero signal, whose -Inf PeakDB that
// encoding/json refuses to emit.
func TestStatsJSONSurvivesSilence(t *testing.T) {
	s := statsOf(t, make([]float64, 1000), analysis.StatsOptions{Rate: 8000})
	if !math.IsInf(s.PeakDB, -1) {
		t.Fatalf("silence peaks at %v dBFS, want -Inf", s.PeakDB)
	}
	if _, err := json.Marshal(s); err == nil {
		t.Fatal("encoding/json accepted an infinity; this test no longer proves anything")
	}
	var buf bytes.Buffer
	if err := WriteStatsJSON(&buf, s); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if m["peakDB"].(float64) >= 0 {
		t.Errorf("peakDB came out as %v, want it pinned to the bottom of the range", m["peakDB"])
	}
}

// TestStatsJSONCoversEveryField checks that every field SignalStats declares
// reaches the JSON.
//
// It walks the tags and looks for each one in the encoded output, so a field
// the struct carries but the document skips cannot pass.
func TestStatsJSONCoversEveryField(t *testing.T) {
	s := &analysis.SignalStats{
		IQ:       &analysis.IQStats{},
		Spectral: &analysis.SpectralStats{Notes: []string{"a spectral note"}},
		Notes:    []string{"a note"},
		BitsNote: "a bits note",
	}
	var buf bytes.Buffer
	if err := WriteStatsJSON(&buf, s); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}

	rt := reflect.TypeOf(*s)
	for f := range rt.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		if name == "complex" {
			continue // omitempty, and this fixture is a real signal
		}
		if _, ok := got[name]; !ok {
			t.Errorf("the JSON is missing %q, which SignalStats declares", name)
		}
	}

	sp, ok := got["spectral"].(map[string]any)
	if !ok {
		t.Fatalf("no spectral section: %v", got["spectral"])
	}
	if _, ok := sp["noiseFloorDB"]; !ok {
		t.Errorf("the spectral section is missing its own fields: %v", sp)
	}
	if n, _ := sp["notes"].([]any); len(n) != 1 {
		t.Errorf("spectral notes = %v, want one", sp["notes"])
	}
	if n, _ := got["notes"].([]any); len(n) != 1 {
		t.Errorf("notes = %v, want one", got["notes"])
	}
}

// TestReportJSONCarriesPairNotes checks the other document, whose pair entries
// were hand-built the same way and dropped their notes.
func TestReportJSONCarriesPairNotes(t *testing.T) {
	r := &SignalReport{
		Channels: []*analysis.SignalStats{{Rate: 8000}},
		Pairs:    []*analysis.ChannelPairStats{{A: 0, B: 1, Notes: []string{"a pair note"}}},
		Notes:    []string{"a report note"},
	}
	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	pairs, _ := got["pairs"].([]any)
	if len(pairs) != 1 {
		t.Fatalf("pairs = %v, want one", got["pairs"])
	}
	p, _ := pairs[0].(map[string]any)
	if n, _ := p["notes"].([]any); len(n) != 1 {
		t.Errorf("pair notes = %v, want one", p["notes"])
	}
}

// TestToneTrackReportsAThresholdOnTheWrongScale is the failure the view exists
// to name. Nothing about a decoder that emits no bits says whether the tones are
// wrong, the clock never locks, or the threshold is simply on a different scale
// than the signal -- and this is the last of those, stated in one line.
func TestToneTrackReportsAThresholdOnTheWrongScale(t *testing.T) {
	const rate = 11025.0
	x := fsk(int(rate)/2, rate, 1600, 1800, 300)
	tr, err := analysis.ToneTrackOf(x, rate, analysis.ToneTrackOptions{
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
	if err := WriteToneSummary(&buf, tr, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "0 over the threshold") {
		t.Errorf("the summary hides the crossing count:\n%s", buf.String())
	}
}
