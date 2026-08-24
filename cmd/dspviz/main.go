// Command dspviz draws what a filter does to a signal.
//
// It writes the standard chart set -- magnitude, passband, transition, stopband,
// phase, group delay, impulse, step, tone spectrum and sweep spectrogram -- as
// SVG files, or serves them as an interactive page that recomputes as the
// parameters move.
//
// Every filter kind in the dspviz catalog is a subcommand whose parameters are
// its flags, so adding a filter there adds it here without this file changing.
//
//	dspviz biquad -type lowpass -freq 1000 -q 0.7071 -out /tmp/lp
//	dspviz firpm -taps 63 -passEnd 6000 -stopStart 9000 -out /tmp/fir
//	dspviz decimate -factor 4 -out /tmp/dec
//	dspviz serve -http 127.0.0.1:8080
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/samuel/go-dsp/dspviz"
	"github.com/samuel/go-dsp/internal/clifile"
	"github.com/samuel/go-dsp/internal/cliflags"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = draw(cmd, args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dspviz:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dspviz <filter|serve> [options]")
	fmt.Fprintln(os.Stderr, "\nfilters:")
	for _, k := range dspviz.Catalog() {
		fmt.Fprintf(os.Stderr, "  %-10s %s\n", k.Name, k.Label)
	}
	fmt.Fprintln(os.Stderr, "\n  serve      an interactive page that recomputes as the parameters move")
	fmt.Fprintln(os.Stderr, "\nRun 'dspviz <filter> -h' for the parameters of one.")
}

// draw builds a filter from the catalog and writes its charts.
func draw(kind string, args []string) error {
	k, ok := dspviz.KindByName(kind)
	if !ok {
		usage()
		return fmt.Errorf("unknown filter %q", kind)
	}

	fs := flag.NewFlagSet(kind, flag.ExitOnError)
	params, choices := k.Defaults()

	// The flags are the catalog entry, so nothing knows a biquad from a firpm.
	nums := make(map[string]*float64, len(k.Params))
	for _, p := range k.Params {
		unit := p.Unit
		if unit != "" {
			unit = " (" + unit + ")"
		}
		nums[p.Name] = fs.Float64(p.Name, p.Default,
			fmt.Sprintf("%s%s, %g to %g", p.Label, unit, p.Min, p.Max))
	}
	strs := make(map[string]*string, len(k.Choices))
	for _, c := range k.Choices {
		strs[c.Name] = fs.String(c.Name, c.Default,
			fmt.Sprintf("%s: %s", c.Label, strings.Join(c.Options, ", ")))
	}

	out := fs.String("out", ".", "directory to write the charts into")
	charts := fs.String("charts", "all", "comma-separated charts to write, or all")
	points := fs.Int("points", 2048, "frequency grid points")
	frame := fs.Int("frame", 0, "measurement frame in samples (0 chooses one)")
	taps := fs.Int("taps-shown", 128, "samples of impulse and step response to draw")
	tone := fs.Float64("tone", 997, "tone frequency for the spectrum, in Hz")
	sweepSec := fs.Float64("sweep-seconds", 8, "sweep length; negative skips the sweep")
	stftSize := fs.Int("stft", 2048, "spectrogram transform size")
	linear := fs.Bool("linear", false, "draw frequency on a linear axis")
	var of cliflags.Output
	of.Register(fs, "heat")
	if err := fs.Parse(args); err != nil {
		return err
	}

	for name, v := range nums {
		params[name] = *v
	}
	for name, v := range strs {
		choices[name] = *v
	}

	p, err := dspviz.Build(kind, params, choices)
	if err != nil {
		return err
	}

	ropt := dspviz.ReportOptions{
		Points: *points, Frame: *frame, Taps: *taps, ToneHz: *tone,
		SweepSec: *sweepSec, STFTSize: *stftSize,
	}
	// Where the bands are part of the spec, say so rather than letting Analyze
	// rediscover them from the response.
	if pass, stop, ok := dspviz.Bands(kind, params); ok {
		ropt.Passband, ropt.Stopband = pass, stop
	}
	rep, err := dspviz.Analyze(p, ropt)
	if err != nil {
		return err
	}

	if err := cliflags.MakeDir(*out); err != nil {
		return err
	}

	opt := of.ChartOptions()
	opt.Linear = *linear
	all := *charts == "all"

	// Names are trimmed and checked before anything is written, so "-charts
	// magnitude, phase" works and a misspelling errors instead of reporting
	// success.
	available := slices.Sorted(maps.Keys(rep.Charts(opt)))
	if rep.Sweep != nil {
		available = append(available, "sweep")
		slices.Sort(available)
	}
	var want []string
	if !all {
		for name := range strings.SplitSeq(*charts, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if !slices.Contains(available, name) {
				return fmt.Errorf("unknown chart %q, want some of %s, or all",
					name, strings.Join(available, ", "))
			}
			want = append(want, name)
		}
		if len(want) == 0 {
			return errors.New("-charts named none; pass some of " + strings.Join(available, ", ") + ", or all")
		}
	}

	// Resolve the palette before measuring or writing, so a typo reports early.
	pal, err := of.Palette()
	if err != nil {
		return err
	}

	var written []string
	for name, c := range rep.Charts(opt) {
		if !all && !slices.Contains(want, name) {
			continue
		}
		if err := cliflags.Write(filepath.Join(*out, name+".svg"), c); err != nil {
			return err
		}
		written = append(written, name)
	}

	if rep.Sweep != nil && (all || slices.Contains(want, "sweep")) {
		sopt := dspviz.SpectrogramOptions{
			Title: k.Label + ": sweep to Nyquist",
			// Always linear: the sweep and its aliases only read as straight
			// lines on a linear axis.
			LogFreq: false,
			FloorDB: of.FloorDB,
		}
		if err := clifile.Write(filepath.Join(*out, "sweep.svg"), func(w io.Writer) error {
			return rep.Sweep.WriteSVG(w, pal, sopt)
		}); err != nil {
			return err
		}
		if err := clifile.Write(filepath.Join(*out, "sweep.png"), func(w io.Writer) error {
			return rep.Sweep.WritePNG(w, pal, sopt)
		}); err != nil {
			return err
		}
		written = append(written, "sweep")
	}

	slices.Sort(written)
	if !of.Quiet {
		if err := rep.WriteSummary(os.Stdout); err != nil {
			return err
		}
		fmt.Printf("\nwrote %s to %s\n", strings.Join(written, ", "), *out)
	}
	return nil
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("http", "127.0.0.1:8080", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	srv, err := dspviz.NewServer(dspviz.ServerOptions{})
	if err != nil {
		return err
	}
	slog.Info("listening", "addr", "http://"+*addr)
	// http.ListenAndServe sets no read-header timeout, leaving a connection that
	// sends nothing open forever.
	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return hs.ListenAndServe()
}
