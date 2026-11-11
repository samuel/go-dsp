package viz

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"samuelks.com/dsp/analysis"
)

// WriteToneSummary prints what the track says, and what the spectrum it was measured
// alongside says the tones really are.
func WriteToneSummary(w io.Writer, t *analysis.ToneTrack, peaks []analysis.TonePeak) error {
	var b strings.Builder
	fmt.Fprintf(&b, "tones         mark %.1f Hz, space %.1f Hz, shift %.1f Hz\n",
		t.Mark, t.Space, math.Abs(t.Mark-t.Space))
	fmt.Fprintf(&b, "block         %d samples (%.2f ms), hop %d\n",
		t.Block, float64(t.Block)/t.Rate*1000, t.Hop)
	fmt.Fprintf(&b, "decisions     %d blocks, %d over the threshold of %g, %d sign changes\n",
		t.Decisions, t.Crossings, t.Threshold, t.Transitions)
	fmt.Fprintf(&b, "difference    largest %.4g, a threshold of %.4g would be crossed three times in four\n",
		t.MaxAbsDiff, t.Suggested)

	if len(peaks) > 0 {
		for i, pk := range peaks {
			fmt.Fprintf(&b, "peak %d        %.1f Hz at %.1f dBFS, %.1f dB over the noise\n",
				i+1, pk.Hz, pk.DB, pk.SNRDB)
		}
		if len(peaks) >= 2 {
			lo, hi := peaks[0].Hz, peaks[1].Hz
			if lo > hi {
				lo, hi = hi, lo
			}
			fmt.Fprintf(&b, "measured      shift %.1f Hz; try -mark %.0f -space %.0f\n", hi-lo, lo, hi)
		}
	}
	for _, w := range t.Warnings {
		fmt.Fprintf(&b, "note          %s\n", w)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// WriteStatsJSON writes the numbers as JSON, from SignalStats' own tags; see
// json.go for why they are sanitized on the way out.
func WriteStatsJSON(w io.Writer, s *analysis.SignalStats) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sanitize(s))
}

// SignalReport is every channel of a recording measured, plus how the channels
// relate.
//
// A recording with one channel is a report with one entry and no pairs, so a
// caller has one report to handle rather than two.
type SignalReport struct {
	Channels []*analysis.SignalStats      `json:"channels"`
	Pairs    []*analysis.ChannelPairStats `json:"pairs,omitempty"`
	Notes    []string                     `json:"notes,omitempty"`
}

// WriteSummary prints every channel as a short table, with a heading per channel
// once there is more than one to tell apart.
func (r *SignalReport) WriteSummary(w io.Writer) error {
	for i, s := range r.Channels {
		if len(r.Channels) > 1 {
			if i > 0 {
				if _, err := io.WriteString(w, "\n"); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "channel %d\n", i); err != nil {
				return err
			}
		}
		if err := WriteStatsSummary(w, s); err != nil {
			return err
		}
	}
	if len(r.Pairs) > 0 {
		var b strings.Builder
		b.WriteString("\nchannels\n")
		for _, p := range r.Pairs {
			fmt.Fprintf(&b, "%d against %d   correlation %.4f, balance %+.2f dB%s\n",
				p.A, p.B, p.Correlation, p.BalanceDB, pairVerdict(p))
			for _, n := range p.Notes {
				fmt.Fprintf(&b, "note          %s\n", n)
			}
		}
		if _, err := io.WriteString(w, b.String()); err != nil {
			return err
		}
	}
	for _, n := range r.Notes {
		if _, err := fmt.Fprintf(w, "note          %s\n", n); err != nil {
			return err
		}
	}
	return nil
}

func pairVerdict(p *analysis.ChannelPairStats) string {
	switch {
	case p.DualMono:
		return ", dual mono"
	case p.Inverted:
		return ", inverted"
	}
	return ""
}

// WriteJSON writes the whole report as one document, from the tagged structs;
// see json.go for why the values are sanitized on the way out.
//
// The document is the same whether the file has one channel or eight: a consumer
// reads channels[0] either way.
func (r *SignalReport) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sanitize(r))
}

func dualMonoNote(q *analysis.IQStats) string {
	switch {
	case q.DualMono:
		return ", which is dual mono"
	case math.Abs(math.Abs(q.QuadratureErrorDeg)-90) < 1:
		return ", a quarter cycle apart: these are an I/Q pair on two channels"
	}
	return ""
}
