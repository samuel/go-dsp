package dspviz

import (
	"fmt"
	"io"
	"math"
	"strings"
)

// The text form of a signal report. The JSON form is the tagged structs
// themselves; see json.go.

// WriteSummary prints the numbers as a short table, in the same form as Report's.
func (s *SignalStats) WriteSummary(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "rate          %.0f Hz\n", s.Rate)
	fmt.Fprintf(&b, "length        %d samples, %.3f s\n", s.Samples, s.Duration)
	fmt.Fprintf(&b, "peak          %.6f (%.2f dBFS)\n", s.Peak, s.PeakDB)
	fmt.Fprintf(&b, "rms           %.6f (%.2f dBFS)\n", s.RMS, s.RMSDB)
	fmt.Fprintf(&b, "crest         %.2f (%.2f dB)\n", s.Crest, DB(s.Crest))
	if !s.Complex {
		fmt.Fprintf(&b, "dc offset     %.6f (%.2f dBFS)\n", s.DC, DB(math.Abs(s.DC)))
	}
	if s.SustainedRuns > 0 {
		fmt.Fprintf(&b, "clipping      %d samples on the rail in %d runs, %d of them sustained\n",
			s.Clipped, s.ClippedRuns, s.SustainedRuns)
	} else {
		fmt.Fprintf(&b, "clipping      none (%d samples touch the rail, longest run under %d)\n",
			s.Clipped, minClipRun)
	}
	if s.EffectiveBits > 0 {
		fmt.Fprintf(&b, "bit depth     %d bits used\n", s.EffectiveBits)
	}
	if s.BitsNote != "" {
		fmt.Fprintf(&b, "bit depth     %s\n", s.BitsNote)
	}
	if !s.Complex {
		fmt.Fprintf(&b, "zero cross    %.1f per second\n", s.ZeroCrossRateHz)
	}
	fmt.Fprintf(&b, "silence       %.1f%% of the file, longest run %.3f s\n",
		s.SilentFraction*100, s.LongestSilence)

	if q := s.IQ; q != nil {
		fmt.Fprintf(&b, "iq dc         I %.6f, Q %.6f (%.2f dBFS)\n", q.DCI, q.DCQ, q.DCDB)
		fmt.Fprintf(&b, "iq imbalance  %.3f dB of gain, %.3f degrees of quadrature error\n",
			q.GainImbalanceDB, q.QuadratureErrorDeg)
		fmt.Fprintf(&b, "iq channels   correlation %.4f%s\n", q.Correlation, dualMonoNote(q))
	}
	if sp := s.Spectral; sp != nil {
		fmt.Fprintf(&b, "spectrum      %d frames, %.3f Hz of noise bandwidth per bin\n", sp.Frames, sp.NoiseBW)
		fmt.Fprintf(&b, "noise floor   %.1f dBFS per bin, flatness %.4f\n", sp.NoiseFloorDB, sp.Flatness)
		fmt.Fprintf(&b, "strongest     %.1f Hz at %.1f dBFS, %.1f dB over the floor\n", sp.PeakHz, sp.PeakDB, sp.SNRDB)
		fmt.Fprintf(&b, "occupied      %.0f to %.0f Hz holds 99%% of the power\n", sp.OccupiedLoHz, sp.OccupiedHiHz)
		fmt.Fprintf(&b, "widths        %.1f Hz at -3 dB, %.1f at -20, %.1f at -60\n",
			sp.Width3DB, sp.Width20DB, sp.Width60DB)
		if sp.ImageRejectionDB != 0 {
			fmt.Fprintf(&b, "image         %.1f dB below the tone\n", sp.ImageRejectionDB)
		}
		for _, n := range sp.Notes {
			fmt.Fprintf(&b, "note          %s\n", n)
		}
	}
	for _, n := range s.Notes {
		fmt.Fprintf(&b, "note          %s\n", n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
