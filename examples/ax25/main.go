// Command ax25 decodes AX.25 packet radio frames from a recording of AFSK:
// 1200 baud Bell 202 by default, the modulation APRS uses on VHF. An
// fsk.Demodulator turns the audio into line levels, ax25.NRZI turns those into
// bits, and ax25.Decoder assembles the HDLC frames, printing each one's
// addresses and payload.
//
// The input is a WAV file, or a headerless file or standard input given with
// -format and -rate. -baud, -mark and -space select other AFSK variants, such
// as 300 baud HF packet:
//
//	ax25 -rate 11025 -baud 300 -mark 1600 -space 1800 capture.raw
package main

import (
	"encoding/hex"
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
	flagBaud       = flag.Float64("baud", 1200, "Baud rate of the AFSK signal")
	flagMark       = flag.Float64("mark", 1200, "Mark tone frequency in Hz")
	flagSpace      = flag.Float64("space", 2200, "Space tone frequency in Hz")
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
	rd, err := sampleio.Open(path, sampleio.Layout{Format: format, Rate: *flagSampleRate})
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
	dec := ax25.NewDecoder()

	samples := make([]float32, 4096)
	var levels, bits []byte
	for {
		n, err := rd.ReadSamples(samples)
		levels = demod.Demodulate(levels[:0], samples[:n])
		bits = nrzi.Decode(bits[:0], levels)
		for _, b := range bits {
			if frame := dec.Feed(b); frame != nil {
				printFrame(frame)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func printFrame(frame *ax25.Frame) {
	if *flagVerbose {
		fmt.Printf("%+v\n", frame)
	} else {
		fmt.Printf("%s to %s", frame.Source, frame.Destination)
		if len(frame.Repeaters) != 0 {
			fmt.Print(" via ")
			for i, r := range frame.Repeaters {
				if i != 0 {
					fmt.Print(",")
				}
				fmt.Print(r.String())
			}
		}
		fmt.Println()
	}
	fmt.Print(hex.Dump(frame.Info))
}
