package dspviz

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// ChannelPairStats says how two channels of a recording relate to one another.
//
// It is what a per-channel report cannot say however many channels it covers:
// two channels can each be unremarkable on their own and still be a duplicate
// pair, or the same signal with one of them inverted, which sums to silence the
// moment anything folds them down to mono.
type ChannelPairStats struct {
	A int `json:"a"`
	B int `json:"b"`

	// Correlation is the centered Pearson correlation: 1 for identical
	// channels, 0 for unrelated ones, -1 for one that is the other inverted.
	Correlation float64 `json:"correlation"`

	// BalanceDB is A's level relative to B's, in decibels of RMS.
	BalanceDB float64 `json:"balanceDB"`

	// DualMono is one signal written twice, and Inverted is one signal written
	// twice with the sign of the second flipped. Both are faults rather than
	// findings: the first wastes half the file, and the second is silent in
	// mono.
	DualMono bool `json:"dualMono"`
	Inverted bool `json:"inverted"`

	Frames int64    `json:"frames"`
	Notes  []string `json:"notes,omitempty"`
}

// PairBuilder accumulates the relationship between two channels a block at a
// time.
//
// It is not a Sink: a Sink takes one signal and this takes two. The five
// sums behind it cost five multiply-adds per frame and nothing that grows
// with the length of the input.
type PairBuilder struct {
	a, b  int
	cross crossSums
}

// NewPairBuilder starts a comparison of the two channels with these indices,
// which it carries only so that the result can name them.
func NewPairBuilder(a, b int) *PairBuilder {
	return &PairBuilder{a: a, b: b}
}

// Write adds a block of each channel. It uses as many frames as the shorter of
// the two holds, the way the conversions in dsp do.
func (p *PairBuilder) Write(x, y []float64) error {
	n := min(len(x), len(y))
	for i := range n {
		p.cross.add(x[i], y[i])
	}
	return nil
}

// Reset clears the accumulated state.
func (p *PairBuilder) Reset() { p.cross = crossSums{} }

// Close returns the comparison. A pair with no frames is an error rather than a
// comparison of nothing, as it is for every other builder here.
func (p *PairBuilder) Close() (*ChannelPairStats, error) {
	if p.cross.n == 0 {
		return nil, errors.New("dspviz: no frames")
	}
	s := &ChannelPairStats{A: p.a, B: p.b, Frames: p.cross.n}
	_, _, pA, pB, _ := p.cross.moments()
	s.Correlation = p.cross.correlation()
	switch {
	case pA > 0 && pB > 0:
		s.BalanceDB = dbPower(pA / pB)
	case pA > 0:
		s.BalanceDB = math.Inf(1)
	case pB > 0:
		s.BalanceDB = math.Inf(-1)
	}

	// A duplicate or an inversion has to match in level as well as in waveform:
	// the same signal at half the level correlates just as well and is neither.
	level := math.Abs(s.BalanceDB) < 0.01
	s.DualMono = level && s.Correlation > 0.9999
	s.Inverted = level && s.Correlation < -0.9999

	switch {
	case s.DualMono:
		s.Notes = append(s.Notes, fmt.Sprintf(
			"channels %d and %d hold the same signal; the file is mono written twice", p.a, p.b))
	case s.Inverted:
		s.Notes = append(s.Notes, fmt.Sprintf(
			"channel %d is channel %d inverted; folded to mono the two cancel to silence", p.b, p.a))
	case s.Correlation < -0.2:
		// Short of an exact inversion this is a caution rather than a fault:
		// plenty of wide stereo correlates negatively and is meant to.
		s.Notes = append(s.Notes, fmt.Sprintf(
			"channels %d and %d correlate at %.3f, so a mono fold loses level rather than gaining it",
			p.a, p.b, s.Correlation))
	}
	if math.Abs(s.BalanceDB) > 3 && !math.IsInf(s.BalanceDB, 0) {
		s.Notes = append(s.Notes, fmt.Sprintf(
			"channel %d sits %.1f dB %s channel %d",
			p.a, math.Abs(s.BalanceDB), louder(s.BalanceDB), p.b))
	}
	return s, nil
}

func louder(db float64) string {
	if db > 0 {
		return "above"
	}
	return "below"
}

// SignalReport is every channel of a recording measured, plus how the channels
// relate.
//
// A recording with one channel is a report with one entry and no pairs, so a
// caller has one report to handle rather than two.
type SignalReport struct {
	Channels []*SignalStats      `json:"channels"`
	Pairs    []*ChannelPairStats `json:"pairs,omitempty"`
	Notes    []string            `json:"notes,omitempty"`
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
		if err := s.WriteSummary(w); err != nil {
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

func pairVerdict(p *ChannelPairStats) string {
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
