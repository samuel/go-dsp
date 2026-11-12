// Command bfsk demodulates binary FSK with an fsk.Demodulator and prints one
// character a symbol, for looking at a signal before decoding it. By default
// it prints the NRZI bits AX.25 carries, '#' for a 1 (no change of level) and
// ' ' for a 0; -levels prints the line levels instead, '#' for mark.
//
// The input is a WAV file, or a headerless file or standard input given with
// -format and -rate.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"

	"samuelks.com/dsp/ax25"
	"samuelks.com/dsp/fsk"
	"samuelks.com/dsp/pcm"
	"samuelks.com/dsp/sampleio"
)

var (
	flagVerbose    = flag.Bool("v", false, "Verbose output")
	flagSampleRate = flag.Float64("rate", 0, "Sample rate override; required for a headerless file")
	flagFormat     = flag.String("format", "i16le", "Sample format of a headerless file")
	flagBaud       = flag.Float64("baud", 1200, "Baud rate of the FSK signal")
	flagMark       = flag.Float64("mark", 1200, "Mark tone frequency in Hz")
	flagSpace      = flag.Float64("space", 2200, "Space tone frequency in Hz")
	flagLevels     = flag.Bool("levels", false, "Print line levels rather than NRZI bits")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	path := ""
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}
	format, err := pcm.ParseFormat(*flagFormat)
	if err != nil {
		return err
	}
	rd, err := sampleio.Open(path, sampleio.Options{Format: format, Rate: *flagSampleRate})
	if err != nil {
		return err
	}
	defer func() {
		if err := rd.Close(); err != nil {
			log.Print(err)
		}
	}()

	demod, err := fsk.NewDemodulator[float32](fsk.Config{
		Mark:  *flagMark,
		Space: *flagSpace,
		Baud:  *flagBaud,
		Rate:  rd.Info().Rate,
	})
	if err != nil {
		return err
	}
	var nrzi ax25.NRZI

	samples := make([]float32, 4096)
	var levels, bits []byte
	for {
		n, err := rd.ReadSamples(samples)
		levels = demod.Demodulate(levels[:0], samples[:n])
		symbols := levels
		if !*flagLevels {
			bits = nrzi.Decode(bits[:0], levels)
			symbols = bits
		}
		for _, s := range symbols {
			if s == 0 {
				fmt.Print(" ")
			} else {
				fmt.Print("#")
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	fmt.Println()
	if *flagVerbose {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), rd.Info())
	}
	return nil
}
