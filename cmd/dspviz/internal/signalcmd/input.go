package signalcmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"samuelks.com/dsp"
	"samuelks.com/dsp/pcm"
	"samuelks.com/dsp/sampleio"
)

// blockFrames is how much of the file is buffered at once. Everything
// downstream is a single-pass accumulator, so this is the whole of the memory a
// capture costs, whatever its length.
const blockFrames = 1 << 16

// channelMode says which of a file's channels an analysis covers.
type channelMode int

const (
	// chanAll gives every channel its own accumulators and compares them. It
	// is the default, so a fault in one channel is not silently ignored.
	chanAll channelMode = iota

	// chanOne is the single channel, or I/Q pair, that -channel names.
	chanOne

	// chanMix is the mean of every channel.
	chanMix
)

// inputFlags are the flags that say what the file holds. Every subcommand
// registers the same set, so a command line that works for one works for all.
type inputFlags struct {
	format   string
	rate     float64
	channels int
	channel  string
	iq       bool
	start    float64
	duration float64
	raw      bool
}

func (f *inputFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.format, "format", "", "sample format of a headerless file: "+
		strings.Join(pcm.FormatNames(), ", ")+", each optionally with a leading c for interleaved I/Q")
	fs.Float64Var(&f.rate, "rate", 0, "sample rate in Hz; required for a headerless file, an override for a WAV")
	fs.IntVar(&f.channels, "channels", 0, "interleaved channels in a headerless file")
	fs.StringVar(&f.channel, "channel", "all",
		"which channel to analyze: a number, all for every channel separately, or mix for the mean of them")
	fs.BoolVar(&f.iq, "iq", false, "read channel pairs as interleaved I/Q")
	fs.Float64Var(&f.start, "start", 0, "seconds to skip before the first sample")
	fs.Float64Var(&f.duration, "duration", 0, "seconds to read, or 0 for all of it")
	fs.BoolVar(&f.raw, "raw", false, "read the file as headerless data even if it has a header")
}

// layout turns the flags into what sampleio needs to open the file, which
// channels to analyze and, for chanOne, which one.
func (f *inputFlags) layout() (l sampleio.Layout, mode channelMode, pick int, err error) {
	l.Rate = f.rate
	l.Channels = f.channels
	if f.raw {
		l.Container = sampleio.Raw
	}
	l.IQ = f.iq

	if f.format != "" {
		format, iq, err := sampleio.ParseFormatSpec(f.format)
		if err != nil {
			return l, mode, 0, err
		}
		l.Format = format
		l.IQ = l.IQ || iq
	}

	switch strings.ToLower(f.channel) {
	case "all", "a":
		mode = chanAll
	case "mix", "m":
		mode = chanMix
	default:
		pick, err = strconv.Atoi(f.channel)
		if err != nil {
			return l, mode, 0, fmt.Errorf("-channel takes a number, all, or mix, not %q", f.channel)
		}
		mode = chanOne
	}

	if l.IQ {
		switch mode {
		case chanMix:
			return l, mode, 0, errors.New("-channel mix does not apply to an I/Q stream; I and Q are one signal")
		case chanAll:
			// Each I/Q pair is one signal, so there are no channels to
			// compare. Read the first pair.
			mode, pick = chanOne, 0
		case chanOne:
		}
	}

	if f.start < 0 || f.duration < 0 {
		return l, mode, 0, errors.New("-start and -duration cannot be negative")
	}
	return l, mode, pick, nil
}

// source is an open recording, positioned at the section that was asked for.
type source struct {
	r    *sampleio.Reader
	info sampleio.Info
	mode channelMode
	pick int   // the channel or I/Q pair chanOne reads
	left int64 // frames still to read, or -1 for all of them
}

// channels is how many sets of accumulators the run needs: one per channel
// when reading them separately, and one otherwise.
func (s *source) channels() int {
	if s.mode == chanAll {
		return s.info.Channels
	}
	return 1
}

// planes is how many slices a planar read of the file takes: one per channel,
// or one per I/Q pair.
func (s *source) planes() int {
	if s.info.IQ {
		return s.info.Channels / 2
	}
	return s.info.Channels
}

// open opens path, which may be "-" for standard input, and skips to -start.
func (f *inputFlags) open(path string) (*source, error) {
	l, mode, pick, err := f.layout()
	if err != nil {
		return nil, err
	}
	r, err := sampleio.Open(path, l)
	if err != nil {
		return nil, err
	}
	s := &source{r: r, info: r.Info(), mode: mode, pick: pick, left: -1}
	if err := s.position(f); err != nil {
		_ = r.Close() // the error from position is the one to report
		return nil, err
	}
	return s, nil
}

// position checks -channel against the file and applies -start and -duration,
// which are in seconds and so need the rate the file was opened with.
func (s *source) position(f *inputFlags) error {
	if s.mode == chanOne {
		unit := "channels"
		if s.info.IQ {
			unit = "I/Q pairs"
		}
		if s.pick < 0 || s.pick >= s.planes() {
			return fmt.Errorf("-channel %d: the file has %d %s, numbered from 0", s.pick, s.planes(), unit)
		}
		if all := strings.ToLower(f.channel); s.info.IQ && s.planes() > 1 && (all == "all" || all == "a") {
			fmt.Fprintf(os.Stderr,
				"dspviz signal: this capture holds %d I/Q pairs; reading pair 0, and -channel selects another\n",
				s.planes())
		}
	}
	if f.start > 0 {
		skip := int64(f.start * s.info.Rate)
		if s.info.Frames >= 0 && skip >= s.info.Frames {
			return fmt.Errorf("-start %g is at or past the end of the file, which is %.6g seconds long",
				f.start, s.info.Duration())
		}
		// A pipe does not know its length, so the end is found by skipping.
		if _, err := s.r.Skip(skip); err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("-start %g is past the end of the stream", f.start)
			}
			return err
		}
	}
	if f.duration > 0 {
		s.left = int64(f.duration * s.info.Rate)
	}
	return nil
}

func (s *source) Close() error { return s.r.Close() }

// blocks reads the section a block at a time. read fills the buffers and
// returns how many frames it read; use is given how many of them are inside
// the section.
func (s *source) blocks(read func() (int, error), use func(n int) error) error {
	for {
		n, err := read()
		if s.left >= 0 && int64(n) >= s.left {
			n, err = int(s.left), io.EOF
		}
		if n > 0 {
			if s.left > 0 {
				s.left -= int64(n)
			}
			if err := use(n); err != nil {
				return err
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// eachPlanar reads the section in blocks and hands fn one slice per channel.
func (s *source) eachPlanar(fn func([][]float64) error) error {
	if s.info.IQ {
		return errors.New("this analysis needs a real stream; drop -iq")
	}
	planes := make([][]float64, s.info.Channels)
	for c := range planes {
		planes[c] = make([]float64, blockFrames)
	}
	view := make([][]float64, len(planes))
	return s.blocks(
		func() (int, error) { return s.r.ReadPlanar(planes) },
		func(n int) error {
			for c := range view {
				view[c] = planes[c][:n]
			}
			return fn(view)
		})
}

// each reads the section in blocks and hands each to a callback: realFn for a
// real stream, cplx for an I/Q one. It reads the one channel or pair -channel
// names, or the mean of every channel for -channel mix.
func (s *source) each(realFn func([]float64) error, cplx func([]complex128) error) error {
	if s.info.IQ {
		if cplx == nil {
			return errors.New("this analysis needs a real stream; drop -iq")
		}
		// Only the chosen pair gets a slice; a nil plane is not read.
		buf := make([]complex128, blockFrames)
		planes := make([][]complex128, s.planes())
		planes[s.pick] = buf
		return s.blocks(
			func() (int, error) { return s.r.ReadPlanar(planes) },
			func(n int) error { return cplx(buf[:n]) })
	}
	if realFn == nil {
		return errors.New("this analysis needs an I/Q stream; add -iq or a c-prefixed format")
	}
	if s.mode == chanMix {
		return s.eachMix(realFn)
	}
	buf := make([]float64, blockFrames)
	planes := make([][]float64, s.planes())
	planes[s.pick] = buf
	return s.blocks(
		func() (int, error) { return s.r.ReadPlanar(planes) },
		func(n int) error { return realFn(buf[:n]) })
}

// eachMix hands fn the mean of every channel.
func (s *source) eachMix(fn func([]float64) error) error {
	planes := make([][]float64, s.planes())
	for c := range planes {
		planes[c] = make([]float64, blockFrames)
	}
	mix := planes[0]
	scale := 1 / float64(len(planes))
	return s.blocks(
		func() (int, error) { return s.r.ReadPlanar(planes) },
		func(n int) error {
			for _, p := range planes[1:] {
				dsp.VAdd(mix[:n], mix[:n], p[:n])
			}
			dsp.VScale(mix[:n], mix[:n], scale)
			return fn(mix[:n])
		})
}
