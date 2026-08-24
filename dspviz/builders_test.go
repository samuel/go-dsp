package dspviz

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// builderSignal is a deterministic block of samples for the builder tests: a
// tone at bin 8 of 256 with a little offset, so every accumulator here has
// something to accumulate.
func builderSignal(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = 0.5*math.Cos(2*math.Pi*float64((8*i)%256)/256) + 0.01
	}
	return x
}

func builderSignalComplex(n int) []complex128 {
	z := make([]complex128, n)
	for i := range z {
		a := 2 * math.Pi * float64((8*i)%256) / 256
		z[i] = complex(0.5*math.Cos(a), 0.5*math.Sin(a))
	}
	return z
}

// TestBuildersReset requires that a reset builder reproduces its first result
// exactly, over every builder in the package.
//
// Reset is not tidiness: LevelBuilder and SpectrogramBuilder carry a partial
// window across calls and a fold whose weights say how many rows are behind each
// column, so a Reset that cleared the accumulators but left the fold alone
// would slide every later column towards whatever arrived first.
func TestBuildersReset(t *testing.T) {
	const rate = 8000.0
	x := builderSignal(4096)
	z := builderSignalComplex(4096)

	for _, c := range []struct {
		name string
		// run feeds the builder, closes it and returns the result. It is
		// called twice on the same builder with a Reset in between.
		run func(t *testing.T) (reset func(), run func() any)
	}{
		{"StatsBuilder", func(t *testing.T) (func(), func() any) {
			b, err := NewStatsBuilder(StatsOptions{Rate: rate, Bits: 16})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.Write(x); err != nil {
					t.Fatal(err)
				}
				s, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
		}},
		{"StatsBuilder/complex", func(t *testing.T) (func(), func() any) {
			b, err := NewStatsBuilder(StatsOptions{Rate: rate, Complex: true})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.WriteComplex(z); err != nil {
					t.Fatal(err)
				}
				s, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
		}},
		{"HistogramBuilder", func(t *testing.T) (func(), func() any) {
			b, err := NewHistogramBuilder(HistogramOptions{Buckets: 256})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.Write(x); err != nil {
					t.Fatal(err)
				}
				h, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return h
			}
		}},
		{"HistogramBuilder/complex", func(t *testing.T) (func(), func() any) {
			b, err := NewHistogramBuilder(HistogramOptions{Buckets: 256, Complex: true})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.WriteComplex(z); err != nil {
					t.Fatal(err)
				}
				h, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return h
			}
		}},
		{"LevelBuilder", func(t *testing.T) (func(), func() any) {
			b, err := NewLevelBuilder(rate, LevelOptions{WindowSec: 0.01, Columns: 32})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				// In uneven blocks, so a partial window really is carried.
				for _, blk := range unevenBlocks(x) {
					if err := b.Write(blk); err != nil {
						t.Fatal(err)
					}
				}
				l, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return l
			}
		}},
		{"LevelBuilder/complex", func(t *testing.T) (func(), func() any) {
			b, err := NewLevelBuilder(rate, LevelOptions{WindowSec: 0.01, Columns: 32})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.WriteComplex(z); err != nil {
					t.Fatal(err)
				}
				l, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return l
			}
		}},
		{"WaveformBuilder", func(t *testing.T) (func(), func() any) {
			b, err := NewWaveformBuilder(rate, WaveformOptions{Columns: 32})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				for _, blk := range unevenBlocks(x) {
					if err := b.Write(blk); err != nil {
						t.Fatal(err)
					}
				}
				w, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return w
			}
		}},
		{"WaveformBuilder/complex", func(t *testing.T) (func(), func() any) {
			b, err := NewWaveformBuilder(rate, WaveformOptions{Columns: 32})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.WriteComplex(z); err != nil {
					t.Fatal(err)
				}
				w, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return w
			}
		}},
		{"ToneTrackBuilder", func(t *testing.T) (func(), func() any) {
			b, err := NewToneTrackBuilder(rate, ToneTrackOptions{
				Mark: 250, Space: 1000, Baud: 300, Threshold: 1, Columns: 32,
			})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				for _, blk := range unevenBlocks(x) {
					if err := b.Write(blk); err != nil {
						t.Fatal(err)
					}
				}
				tr, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return tr
			}
		}},
		{"SpectrogramBuilder", func(t *testing.T) (func(), func() any) {
			b, err := NewSpectrogramBuilder(rate, STFTOptions{Size: 256, Columns: 16})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				for _, blk := range unevenBlocks(x) {
					if err := b.Write(blk); err != nil {
						t.Fatal(err)
					}
				}
				sg, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				// The PSD accumulated alongside it has to reset too.
				return []any{sg, b.PSD()}
			}
		}},
		{"SpectrogramBuilder/complex", func(t *testing.T) (func(), func() any) {
			b, err := NewSpectrogramBuilder(rate, STFTOptions{Size: 256, Columns: 16, Complex: true})
			if err != nil {
				t.Fatal(err)
			}
			return b.Reset, func() any {
				if err := b.WriteComplex(z); err != nil {
					t.Fatal(err)
				}
				sg, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return []any{sg, b.PSD()}
			}
		}},
		{"PairBuilder", func(t *testing.T) (func(), func() any) {
			b := NewPairBuilder(0, 1)
			return b.Reset, func() any {
				for _, blk := range unevenBlocks(x) {
					if err := b.Write(blk, blk); err != nil {
						t.Fatal(err)
					}
				}
				p, err := b.Close()
				if err != nil {
					t.Fatal(err)
				}
				return p
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			reset, run := c.run(t)
			first := run()
			reset()
			again := run()
			if !reflect.DeepEqual(first, again) {
				t.Errorf("after Reset the result differs\nfirst: %+v\nagain: %+v", first, again)
			}
		})
	}
}

// unevenBlocks splits x into blocks of assorted sizes, none of them a whole
// number of any builder's window or hop, so a builder that mishandles a partial
// window fails rather than happening to line up.
func unevenBlocks(x []float64) [][]float64 {
	var out [][]float64
	sizes := []int{1, 7, 63, 129, 1000}
	for i, s := 0, 0; i < len(x); s++ {
		n := min(sizes[s%len(sizes)], len(x)-i)
		out = append(out, x[i:i+n])
		i += n
	}
	return out
}

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

	stats, err := StatsOfComplex(z, StatsOptions{Rate: rate})
	if err != nil {
		t.Fatal(err)
	}
	psd, err := PSDOfComplex(z, rate, STFTOptions{Size: 512, Complex: true})
	if err != nil {
		t.Fatal(err)
	}
	stats.Spectral = SpectralStatsOf(psd)
	if len(stats.Spectral.Notes) == 0 {
		t.Fatal("a real signal read as I/Q produced no spectral note, so this fixture proves nothing")
	}

	pair, err := func() (*ChannelPairStats, error) {
		b := NewPairBuilder(0, 1)
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

	rep := &SignalReport{Channels: []*SignalStats{stats}, Pairs: []*ChannelPairStats{pair}}
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
