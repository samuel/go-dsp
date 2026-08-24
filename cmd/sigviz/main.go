// Command sigviz says what is in a recording.
//
// Where dspviz measures a filter, sigviz measures a file: point it at a WAV or
// a raw capture and it reports the numbers and draws the pictures. It shares
// dspviz's drawing layer, so there is one chart style here and not two.
//
//	sigviz dtmf.wav                                     # the report, no flags at all
//	sigviz spectrogram -o /tmp/wf.png capture.wav
//	sigviz spectrum -o - capture.wav                    # an SVG on standard output
//	sigviz analyze -out /tmp/look capture.wav           # everything, in one pass
//	sigviz tones -rate 11025 -format i16le -mark 1600 -space 1800 -baud 300 packet.raw
//
// A WAV carries its rate and its encoding, so it needs no flags. A headerless
// capture carries neither, so -format and -rate are required rather than
// guessed: an unstated rate makes every frequency label on the output a lie.
//
// The analyses are all single-pass accumulators driven from one read of the
// file, so a capture larger than memory is looked at as thoroughly as one that
// fits.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/samuel/go-dsp/sampleio"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "help", "-h", "--help":
		usage()
		return
	case "formats":
		formats()
		return
	case "spectrogram", "spec":
		err = draw(cmd, args, views{spectrogram: true})
	case "spectrum":
		err = draw(cmd, args, views{spectrum: true})
	case "waveform":
		err = draw(cmd, args, views{waveform: true})
	case "level":
		err = draw(cmd, args, views{level: true})
	case "histogram":
		err = draw(cmd, args, views{histogram: true})
	case "tones":
		err = draw(cmd, args, views{tones: true})
	case "stats":
		err = draw(cmd, args, views{stats: true})
	case "analyze":
		err = draw(cmd, args, views{
			spectrogram: true, spectrum: true, waveform: true,
			level: true, histogram: true, stats: true,
		})
	default:
		// A bare path is the report, which is the whole of what a WAV needs.
		err = draw("stats", os.Args[1:], views{stats: true})
	}
	if err != nil {
		if _, ok := errors.AsType[reportedError](err); !ok {
			fmt.Fprintln(os.Stderr, "sigviz:", err)
		}
		os.Exit(1)
	}
}

// reportedError is an error whose message is already printed, so main exits
// without printing it again. The original is kept for a test to match on.
type reportedError struct{ error }

func (e reportedError) Unwrap() error { return e.error }

// draw parses one subcommand's flags and runs it.
//
// ContinueOnError, not ExitOnError as in cmd/dspviz, so the tests can exercise
// the bad-flag paths without the test binary calling os.Exit.
func draw(name string, args []string, v views) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var in inputFlags
	var out outputFlags
	var tn toneFlags
	in.register(fs)
	out.register(fs)
	if v.tones {
		tn.register(fs)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			// -h is a request granted; the flag package already printed usage.
			return nil
		}
		// The flag package already printed the message and usage.
		return reportedError{err}
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("one file at a time, not %d", fs.NArg())
	}
	path := fs.Arg(0)

	// analyze writes a whole set, so it gets a directory; the rest default to
	// standard output.
	if name == "analyze" && out.dir == "" && out.out == "" {
		out.dir = "."
	}
	return run(path, &in, &out, &tn, v)
}

func usage() {
	w := os.Stderr
	_, _ = fmt.Fprintln(w, "usage: sigviz <command> [options] <file|->")
	_, _ = fmt.Fprintln(w, "\ncommands:")
	for _, c := range [][2]string{
		{"spectrogram", "frequency against time, as a labeled raster (alias: spec)"},
		{"spectrum", "the averaged spectrum, with peak hold and min hold"},
		{"waveform", "a min/max envelope against time"},
		{"level", "RMS, peak and crest against time, in decibels"},
		{"histogram", "the amplitude distribution"},
		{"tones", "a two-tone demodulator's decision variable against its threshold"},
		{"stats", "the numbers, as text or -json; writes no picture"},
		{"analyze", "all of the above, from one pass, into -out <dir>"},
		{"formats", "the sample formats a headerless file can be read as"},
	} {
		_, _ = fmt.Fprintf(w, "  %-12s %s\n", c[0], c[1])
	}
	_, _ = fmt.Fprintln(w, "\nA WAV needs no flags. A headerless capture needs -format and -rate.")
	_, _ = fmt.Fprintln(w, "Run 'sigviz <command> -h' for the options.")
}

func formats() {
	fmt.Println("sample formats, for -format on a headerless file:")
	for _, f := range sampleio.Formats() {
		fmt.Printf("  %-6s %s\n", f, f.Desc())
	}
	fmt.Println("\nA leading c makes the samples interleaved I/Q, so cu8 is rtl-sdr's format")
	fmt.Println("and cs16le a 16-bit I/Q recording. The ffmpeg spelling of a signed integer")
	fmt.Println("is accepted too: " + strings.Join([]string{"s16le", "i16le"}, " and ") + " are the same thing.")
}
