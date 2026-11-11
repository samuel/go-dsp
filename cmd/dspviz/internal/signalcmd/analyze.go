package signalcmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"samuelks.com/dsp/analysis"
	"samuelks.com/dsp/internal/clifile"
	"samuelks.com/dsp/internal/cliflags"
	"samuelks.com/dsp/internal/viz"
	"samuelks.com/dsp/plot"
	"samuelks.com/dsp/sampleio"
)

// views is which of the analyses to run. All are single-pass accumulators, so
// asking for every one still costs one read of the file.
type views struct {
	spectrogram bool
	spectrum    bool
	waveform    bool
	level       bool
	histogram   bool
	tones       bool
	stats       bool
}

// charts is which of them produce a picture. Statistics are one document
// however many channels there are; only a picture is written per channel, and
// only a picture cannot go to standard output twice.
func (v views) charts() bool {
	return v.spectrogram || v.spectrum || v.waveform || v.level || v.histogram || v.tones
}

// needsSTFT reports the ones built on the frame accumulator, which need STFT
// options worked out for them.
func (v views) needsSTFT() bool {
	return v.spectrogram || v.spectrum || v.stats || v.tones
}

// toneFlags describe the two-tone decision a demodulator would be making.
type toneFlags struct {
	mark, space, baud, threshold float64
	block                        int
}

func (t *toneFlags) register(fs *flag.FlagSet) {
	fs.Float64Var(&t.mark, "mark", 0, "mark tone in Hz")
	fs.Float64Var(&t.space, "space", 0, "space tone in Hz")
	fs.Float64Var(&t.baud, "baud", 0, "symbol rate, which sets the block length")
	fs.IntVar(&t.block, "block", 0, "block length in samples, overriding -baud")
	fs.Float64Var(&t.threshold, "threshold", 0, "the decoder's threshold on the difference of the two powers")
}

// channelViews is one channel's worth of accumulators.
type channelViews struct {
	// label names the channel in an output file, empty when the run covers a
	// single channel, so a mono file keeps the names it always used.
	label string

	sg    *analysis.SpectrogramBuilder
	stats *analysis.StatsBuilder
	wave  *analysis.WaveformBuilder
	lev   *analysis.LevelBuilder
	hist  *analysis.HistogramBuilder
	tones *analysis.ToneTrackBuilder

	sinks  []analysis.Sink
	csinks []analysis.ComplexSink
}

func (a *channelViews) add(s any) {
	if r, ok := s.(analysis.Sink); ok {
		a.sinks = append(a.sinks, r)
	}
	if c, ok := s.(analysis.ComplexSink); ok {
		a.csinks = append(a.csinks, c)
	}
}

func (a *channelViews) write(x []float64) error {
	for _, s := range a.sinks {
		if err := s.Write(x); err != nil {
			return err
		}
	}
	return nil
}

func (a *channelViews) writeComplex(z []complex128) error {
	for _, s := range a.csinks {
		if err := s.WriteComplex(z); err != nil {
			return err
		}
	}
	return nil
}

// newChannelViews builds the accumulators the wanted views need for one channel.
// sopt is computed once in run so its notes print once, not once per channel.
func newChannelViews(label string, info sampleio.Info, out *outputFlags, tn *toneFlags, v views, sopt analysis.STFTOptions) (*channelViews, error) {
	a := &channelViews{label: label}

	if v.needsSTFT() {
		var err error
		sopt.Complex = info.Complex
		if !v.spectrogram {
			// Nothing is going to draw the frames, so keep the fewest the
			// reducer allows and let the averaged spectrum do the work.
			sopt.Columns = 2
		}
		if a.sg, err = analysis.NewSpectrogramBuilder(info.Rate, sopt); err != nil {
			return nil, err
		}
		a.add(a.sg)
	}
	if v.stats {
		bits := info.Bits
		if info.ValidBits > 0 && info.ValidBits < bits {
			bits = info.ValidBits
		}
		s, err := analysis.NewStatsBuilder(analysis.StatsOptions{
			Rate: info.Rate, Bits: bits, Complex: info.Complex,
		})
		if err != nil {
			return nil, err
		}
		a.stats = s
		a.add(s)
	}
	if v.waveform {
		w, err := analysis.NewWaveformBuilder(info.Rate, analysis.WaveformOptions{Columns: 2 * paneWidth(out)})
		if err != nil {
			return nil, err
		}
		a.wave = w
		a.add(w)
	}
	if v.level {
		l, err := analysis.NewLevelBuilder(info.Rate, analysis.LevelOptions{Columns: 2 * paneWidth(out)})
		if err != nil {
			return nil, err
		}
		a.lev = l
		a.add(l)
	}
	if v.histogram {
		h, err := analysis.NewHistogramBuilder(analysis.HistogramOptions{Complex: info.Complex})
		if err != nil {
			return nil, err
		}
		a.hist = h
		a.add(h)
	}
	if v.tones {
		if info.Complex {
			return nil, errors.New("the tone view reads a real signal; drop -iq")
		}
		if tn.mark <= 0 || tn.space <= 0 {
			return nil, errors.New("the tone view needs -mark and -space in Hz")
		}
		t, err := analysis.NewToneTrackBuilder(info.Rate, analysis.ToneTrackOptions{
			Mark: tn.mark, Space: tn.space, Baud: tn.baud, Block: tn.block,
			Threshold: tn.threshold, Columns: 2 * paneWidth(out),
		})
		if err != nil {
			return nil, err
		}
		a.tones = t
		a.add(t)
	}
	return a, nil
}

// channelPairs is the comparison set: every distinct pair up to 8 channels,
// and each channel against the first beyond that, keeping the table readable.
// Two channels, which is what this is for, is one pair either way.
func channelPairs(n int) [][2]int {
	var out [][2]int
	if n > 8 {
		for c := 1; c < n; c++ {
			out = append(out, [2]int{0, c})
		}
		return out
	}
	for a := range n {
		for b := a + 1; b < n; b++ {
			out = append(out, [2]int{a, b})
		}
	}
	return out
}

// run opens the file, builds the sinks for every covered channel, reads the
// whole section once through them, and writes the results.
func run(path string, in *inputFlags, out *outputFlags, tn *toneFlags, v views) error {
	if err := out.check(); err != nil {
		return err
	}
	src, err := in.open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := src.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "dspviz signal: closing the input:", err)
		}
	}()

	info := src.info
	if info.Rate <= 0 {
		return errors.New("the file does not say what rate it was recorded at; pass -rate")
	}

	n := src.channels()

	// Report a run that cannot place its output before reading the file, not at
	// the point of writing after minutes of analysis.
	if n > 1 && v.charts() && out.dir == "" && (out.out == "" || out.out == "-") {
		return errors.New("standard output takes one chart, and this file has several channels; " +
			"pick one with -channel N, fold them with -channel mix, or write a set with -out <dir>")
	}

	// Once for the run, not once per channel: see newChannelViews.
	var sopt analysis.STFTOptions
	if v.needsSTFT() {
		if sopt, err = out.stftOptions(info.Rate); err != nil {
			return err
		}
	}

	each := make([]*channelViews, n)
	for c := range each {
		// Only a run covering several channels needs to say which is which.
		label := ""
		if n > 1 {
			label = fmt.Sprintf("ch%d", c)
		}
		if each[c], err = newChannelViews(label, info, out, tn, v, sopt); err != nil {
			return err
		}
	}

	// Channels are compared only when the statistics are wanted, since nothing
	// else reads the answer.
	var pairs []*analysis.PairBuilder
	var pairAt [][2]int
	if v.stats && n > 1 {
		pairAt = channelPairs(n)
		for _, p := range pairAt {
			pairs = append(pairs, analysis.NewPairBuilder(p[0], p[1]))
		}
	}

	if n > 1 {
		err = src.eachPlanar(func(planes [][]float64) error {
			for c, a := range each {
				if err := a.write(planes[c]); err != nil {
					return err
				}
			}
			for i, p := range pairs {
				if err := p.Write(planes[pairAt[i][0]], planes[pairAt[i][1]]); err != nil {
					return err
				}
			}
			return nil
		})
	} else {
		a := each[0]
		err = src.each(a.write, a.writeComplex)
	}
	if err != nil {
		return err
	}

	for _, note := range append(slices.Clone(info.Notes), out.notes...) {
		fmt.Fprintln(os.Stderr, "dspviz signal:", note)
	}

	if err := cliflags.MakeDir(out.dir); err != nil {
		return err
	}
	return writeViews(path, info, each, pairs, out, tn, v)
}

// writeViews turns every channel's accumulators into files.
func writeViews(path string, info sampleio.Info, each []*channelViews,
	pairs []*analysis.PairBuilder, out *outputFlags, tn *toneFlags, v views) error {
	copt := out.chartOptions()
	written := map[string]bool{}
	report := &viz.SignalReport{Notes: info.Notes}

	for _, a := range each {
		var psd *analysis.PSD
		if a.sg != nil {
			psd = a.sg.PSD()
		}

		if v.spectrogram {
			sg, err := a.sg.Close()
			if err != nil {
				return err
			}
			title := fmt.Sprintf("%s: %s", filepath.Base(displayName(path)), describe(info))
			if a.label != "" {
				title = fmt.Sprintf("%s: channel %s, %s",
					filepath.Base(displayName(path)), strings.TrimPrefix(a.label, "ch"), describe(info))
			}
			if err := out.writeSpectrogram("spectrogram", a.label, sg, out.spectrogramOptions(title)); err != nil {
				return err
			}
			written["spectrogram"] = true
		}
		if v.spectrum {
			if psd == nil || psd.Frames == 0 {
				return errors.New("the section is shorter than one frame")
			}
			if err := out.writeChart("spectrum", a.label, plot.PSDChart(psd, copt)); err != nil {
				return err
			}
			written["spectrum"] = true
		}
		if v.waveform {
			w, err := a.wave.Close()
			if err != nil {
				return err
			}
			if err := out.writeChart("waveform", a.label, plot.WaveformChart(w, copt)); err != nil {
				return err
			}
			written["waveform"] = true
		}
		if v.level {
			l, err := a.lev.Close()
			if err != nil {
				return err
			}
			if err := out.writeChart("level", a.label, plot.LevelChart(l, copt)); err != nil {
				return err
			}
			written["level"] = true
		}
		if v.histogram {
			h, err := a.hist.Close()
			if err != nil {
				return err
			}
			if err := out.writeChart("histogram", a.label, plot.HistogramChart(h, copt)); err != nil {
				return err
			}
			written["histogram"] = true
		}
		if v.tones {
			track, err := a.tones.Close()
			if err != nil {
				return err
			}
			if err := out.writeChart("tones", a.label, plot.ToneTrackChart(track, copt)); err != nil {
				return err
			}
			written["tones"] = true
			if !out.Quiet {
				if a.label != "" {
					fmt.Printf("channel %s\n", strings.TrimPrefix(a.label, "ch"))
				}
				var peaks []analysis.TonePeak
				if psd != nil {
					peaks = analysis.TonePeaks(psd, 2, math.Max(50, math.Abs(tn.mark-tn.space)/2))
				}
				if err := viz.WriteToneSummary(os.Stdout, track, peaks); err != nil {
					return err
				}
			}
		}
		if v.stats {
			s, err := a.stats.Close()
			if err != nil {
				return err
			}
			if psd != nil && psd.Frames > 0 {
				s.Spectral = analysis.SpectralStatsOf(psd)
			} else {
				s.Notes = append(s.Notes,
					"the section is shorter than one frame, so there is no spectrum; try -fft with fewer points")
			}
			report.Channels = append(report.Channels, s)
		}
	}

	if v.stats {
		for _, p := range pairs {
			cp, err := p.Close()
			if err != nil {
				return err
			}
			report.Pairs = append(report.Pairs, cp)
		}
		if err := writeReport(path, info, report, out); err != nil {
			return err
		}
		written["stats"] = true
	}

	if out.dir != "" && !out.Quiet {
		names := slices.Sorted(maps.Keys(written))
		fmt.Printf("wrote %s to %s\n", strings.Join(names, ", "), out.dir)
	}
	return nil
}

// writeReport writes the statistics: one document however many channels, with
// the channels as sections and cross-channel comparisons as pairs, which belong
// to none of them individually.
func writeReport(path string, info sampleio.Info, report *viz.SignalReport, out *outputFlags) error {
	dst := out.out
	if out.dir != "" {
		dst = filepath.Join(out.dir, "stats.txt")
		if out.json {
			dst = filepath.Join(out.dir, "stats.json")
		}
	}
	return clifile.Write(dst, func(w io.Writer) error {
		if out.json {
			return report.WriteJSON(w)
		}
		if err := writeHeader(w, displayName(path), info); err != nil {
			return err
		}
		return report.WriteSummary(w)
	})
}

// paneWidth is the width the column reducers are sized against, so no column
// straddles two pixels by more than half of one.
func paneWidth(out *outputFlags) int {
	if out.Width > 0 {
		return out.Width
	}
	return 900
}

func displayName(path string) string {
	if path == "" || path == "-" {
		return "standard input"
	}
	return path
}

// describe is the one-line statement of what the file holds, carried by every
// plot so a picture cannot be read as being of something it is not.
func describe(info sampleio.Info) string {
	ch := fmt.Sprintf("%d channels", info.Channels)
	if info.Channels == 1 {
		ch = "mono"
	}
	if info.Complex {
		ch += ", I/Q"
	}
	s := fmt.Sprintf("%s %s, %.0f Hz, %s", info.Container, info.Format, info.Rate, ch)
	if d := info.Duration(); d > 0 {
		s += fmt.Sprintf(", %.3f s", d)
	}
	return s
}

func writeHeader(w io.Writer, name string, info sampleio.Info) error {
	_, err := fmt.Fprintf(w, "file          %s\nformat        %s (%s)\n",
		name, info.Format, info.Format.Desc())
	return err
}
