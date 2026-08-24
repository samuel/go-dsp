package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/samuel/go-dsp/sampleio"
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

	// chanOne is the single channel Options.Channel names, chanMix the mean.
	chanOne
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
		strings.Join(sampleio.FormatNames(), ", ")+", each optionally with a leading c for interleaved I/Q")
	fs.Float64Var(&f.rate, "rate", 0, "sample rate in Hz; required for a headerless file, an override for a WAV")
	fs.IntVar(&f.channels, "channels", 0, "interleaved channels in a headerless file")
	fs.StringVar(&f.channel, "channel", "all",
		"which channel to analyze: a number, all for every channel separately, or mix for the mean of them")
	fs.BoolVar(&f.iq, "iq", false, "read channel pairs as interleaved I/Q")
	fs.Float64Var(&f.start, "start", 0, "seconds to skip before the first sample")
	fs.Float64Var(&f.duration, "duration", 0, "seconds to read, or 0 for all of it")
	fs.BoolVar(&f.raw, "raw", false, "read the file as headerless data even if it has a header")
}

func (f *inputFlags) options() (sampleio.Options, channelMode, error) {
	var opt sampleio.Options
	mode := chanAll
	opt.Rate = f.rate
	opt.Channels = f.channels
	opt.Raw = f.raw
	opt.IQ = f.iq

	if f.format != "" {
		format, iq, err := sampleio.ParseFormatSpec(f.format)
		if err != nil {
			return opt, mode, err
		}
		opt.Format = format
		opt.IQ = opt.IQ || iq
	}

	switch strings.ToLower(f.channel) {
	case "all", "a":
		mode = chanAll
	case "mix", "m":
		mode = chanMix
		opt.Channel = sampleio.MixChannels
	default:
		n, err := strconv.Atoi(f.channel)
		if err != nil {
			return opt, mode, fmt.Errorf("-channel takes a number, all, or mix, not %q", f.channel)
		}
		mode = chanOne
		opt.Channel = sampleio.Channel(n)
	}

	// An I/Q stream's channels are a pair rather than alternatives, so a planar
	// read has nothing to split: each pair is one signal.
	if opt.IQ && mode == chanAll {
		mode = chanOne
		opt.Channel = 0
	}

	if f.start < 0 || f.duration < 0 {
		return opt, mode, errors.New("-start and -duration cannot be negative")
	}
	return opt, mode, nil
}

// source is an open recording, plus the section of it that was asked for.
type source struct {
	r    *sampleio.Reader
	info sampleio.Info
	mode channelMode
	skip int64 // frames to drop
	left int64 // frames still to read, or -1 for all of them

	startSec float64 // what -start asked for, for the error when it is past the end
}

// channels is how many sets of accumulators the run needs: every channel in the
// file when reading them separately, and one otherwise.
func (s *source) channels() int {
	if s.mode == chanAll {
		return s.info.Channels
	}
	return 1
}

// open opens path, which may be "-" for standard input.
//
// The section is read past rather than sought to, even for a file: sampleio's
// Skip and Limit are in frames, and the rate is not known until the header has
// been read, so seeking would mean opening the file twice.
func (f *inputFlags) open(path string) (*source, error) {
	opt, mode, err := f.options()
	if err != nil {
		return nil, err
	}
	r, err := sampleio.Open(path, opt)
	if err != nil {
		return nil, err
	}
	s := &source{r: r, info: r.Info(), mode: mode, left: -1}
	if opt.IQ && mode == chanOne && s.info.Channels > 2 {
		fmt.Fprintf(os.Stderr,
			"sigviz: this capture holds %d I/Q pairs; reading pair 0, and -channel selects another\n",
			s.info.Channels/2)
	}
	if f.start > 0 {
		s.skip = int64(f.start * s.info.Rate)
		s.startSec = f.start
		// A length is only believed once checked against the size on disk, so
		// a pipe says -1 and the skip is checked at EOF instead.
		if s.info.Frames >= 0 && s.skip >= s.info.Frames {
			return nil, fmt.Errorf("-start %g is at or past the end of the file, which is %.6g seconds long",
				f.start, s.info.Duration())
		}
	}
	if f.duration > 0 {
		s.left = int64(f.duration * s.info.Rate)
	}
	return s, nil
}

func (s *source) Close() error { return s.r.Close() }

// exhausted is what the reading loops return at EOF. A skip that never ran out
// means -start asked for a point past the last sample, and every view would
// otherwise report an empty analysis without saying why.
func (s *source) exhausted() error {
	if s.skip > 0 {
		return fmt.Errorf("-start %g is past the end of the stream", s.startSec)
	}
	return nil
}

// eachPlanar reads the section in blocks and hands fn one slice per channel.
//
// It is one pass over the file however many channels it has: the reader decodes
// the whole interleaved block once and scatters it.
func (s *source) eachPlanar(fn func([][]float64) error) error {
	if s.info.Complex {
		return errors.New("this analysis needs a real stream; drop -iq")
	}
	n := s.info.Channels
	planes := make([][]float64, n)
	for c := range planes {
		planes[c] = make([]float64, blockFrames)
	}
	// The section is applied once per block and the views handed sub-slices of
	// the planes, so every channel is trimmed identically.
	view := make([][]float64, n)
	for {
		got, err := s.r.ReadFloat64Planar(planes)
		if got > 0 {
			lo, hi, done := s.section(got)
			if hi > lo {
				for c := range view {
					view[c] = planes[c][lo:hi]
				}
				if err := fn(view); err != nil {
					return err
				}
			}
			if done {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return s.exhausted()
			}
			return err
		}
	}
}

// each reads the section in blocks and hands each to fn. Exactly one of the two
// callbacks is used, whichever matches the stream.
func (s *source) each(realFn func([]float64) error, cplx func([]complex128) error) error {
	if s.info.Complex {
		return s.eachComplex(cplx)
	}
	if realFn == nil {
		return errors.New("this analysis needs an I/Q stream; add -iq or a c-prefixed format")
	}
	buf := make([]float64, blockFrames)
	for {
		n, err := s.r.ReadFloat64(buf)
		if n > 0 {
			lo, hi, done := s.section(n)
			if hi > lo {
				if err := realFn(buf[lo:hi]); err != nil {
					return err
				}
			}
			if done {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return s.exhausted()
			}
			return err
		}
	}
}

func (s *source) eachComplex(fn func([]complex128) error) error {
	if fn == nil {
		return errors.New("this analysis needs a real stream; drop -iq")
	}
	buf := make([]complex128, blockFrames)
	for {
		n, err := s.r.ReadComplex128(buf)
		if n > 0 {
			lo, hi, done := s.section(n)
			if hi > lo {
				if err := fn(buf[lo:hi]); err != nil {
					return err
				}
			}
			if done {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return s.exhausted()
			}
			return err
		}
	}
}

// section applies -start and -duration to a block that has just been read,
// returning the half-open range of it to use and whether the section has ended.
// A block straddling the start is split, keeping its tail, which is why the
// result is a range rather than a count.
func (s *source) section(n int) (lo, hi int, done bool) {
	lo, hi = 0, n
	if s.skip > 0 {
		d := min(s.skip, int64(n))
		s.skip -= d
		lo = int(d)
		if lo == hi {
			return lo, hi, false
		}
	}
	if s.left >= 0 {
		if int64(hi-lo) >= s.left {
			hi = lo + int(s.left)
			s.left = 0
			return lo, hi, true
		}
		s.left -= int64(hi - lo)
	}
	return lo, hi, false
}
