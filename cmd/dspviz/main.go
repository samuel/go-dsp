// Command dspviz measures and draws filters and recordings.
//
// "dspviz filter" draws what a filter does to a signal: the standard chart set
// for any filter kind in its catalog, or an interactive page that recomputes as
// the parameters move. "dspviz signal" says what is in a recording: point it at
// a WAV or a raw capture and it reports the numbers and draws the pictures. The
// two share one chart style.
//
//	dspviz filter biquad -type lowpass -freq 1000 -q 0.7071 -out /tmp/lp
//	dspviz filter firpm -taps 63 -passEnd 6000 -stopStart 9000 -out /tmp/fir
//	dspviz filter serve -http 127.0.0.1:8080
//	dspviz signal dtmf.wav                             # the report, no flags at all
//	dspviz signal spectrogram -o /tmp/wf.png capture.wav
//	dspviz signal analyze -out /tmp/look capture.wav   # everything, in one pass
//	dspviz signal tones -rate 11025 -format i16le -mark 1600 -space 1800 -baud 300 packet.raw
package main

import (
	"fmt"
	"log/slog"
	"os"

	"samuelks.com/dsp/cmd/dspviz/internal/filtercmd"
	"samuelks.com/dsp/cmd/dspviz/internal/signalcmd"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "filter":
		os.Exit(filtercmd.Run(os.Args[2:]))
	case "signal":
		os.Exit(signalcmd.Run(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "dspviz: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dspviz <filter|signal> <command> [options]")
	fmt.Fprintln(os.Stderr, "\n  filter     measure and draw a filter from the catalog, or serve an interactive page")
	fmt.Fprintln(os.Stderr, "  signal     measure and draw a recording")
	fmt.Fprintln(os.Stderr, "\nRun 'dspviz filter -h' or 'dspviz signal -h' for the commands of each.")
}
