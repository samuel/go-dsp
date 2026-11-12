// Command bfsk demodulates binary FSK by comparing the power a Goertzel filter
// measures at the mark tone with the power at the space tone, recovers the bit
// clock from the transitions, and prints the bits as '#' for a mark and ' '
// for a space. It is the bit slicer of the ax25 example on its own, for
// looking at a signal before decoding it.
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
	"math"

	"samuelks.com/dsp"
	"samuelks.com/dsp/pcm"
	"samuelks.com/dsp/sampleio"
)

var (
	flagVerbose    = flag.Bool("v", false, "Verbose output")
	flagSampleRate = flag.Float64("rate", 0, "Sample rate override; required for a headerless file")
	flagFormat     = flag.String("format", "i16le", "Sample format of a headerless file")
	flagBaud       = flag.Int("baud", 1200, "Baud rate of the FSK signal")
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
	rd, err := sampleio.Open(path, sampleio.Options{Format: format, Rate: *flagSampleRate})
	if err != nil {
		return err
	}
	defer func() {
		if err := rd.Close(); err != nil {
			log.Print(err)
		}
	}()

	sampleRate := rd.Info().Rate
	baud := *flagBaud
	if baud <= 0 {
		// blockSize below divides by it, and the bit clock is its reciprocal.
		return fmt.Errorf("-baud must be positive, not %d", baud)
	}
	window := 4
	blockSize := int(sampleRate) / baud

	// The type parameter is the sample type; the recursion and the results are
	// float64 either way.
	goer, err := dsp.NewGoertzel[float32]([]float64{*flagMark, *flagSpace}, sampleRate, blockSize)
	if err != nil {
		return err
	}

	threshold := 0.0

	samples := make([]float32, blockSize)
	buf := make([]float32, window)

	currentTime := float64(0.0)
	bitClock := 1.0 / float64(baud)
	windowTime := float64(window) / sampleRate
	timeDelta := 0.0
	prevBit := 0
	transition := false

	for {
		if err := readFull(rd, buf); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}

		copy(samples, samples[window:])
		copy(samples[len(samples)-window:], buf)

		goer.Reset()
		goer.Feed(samples)
		mags := goer.Power()
		diff := mags[0] - mags[1]

		if math.Abs(diff) > threshold {
			b := 1
			if diff < 0 {
				b = 0
			}
			if prevBit != b {
				transition = true
				prevBit = b
				// Align transitions to middle of clock tick
				timeDelta = bitClock/2.0 - currentTime
			}
		}

		currentTime += windowTime
		for currentTime >= bitClock {
			currentTime -= bitClock
			b := 1
			if transition {
				b = 0
				currentTime += timeDelta
				timeDelta = 0.0
			}
			if b == 0 {
				fmt.Print(" ")
			} else {
				fmt.Print("#")
			}
			transition = false
		}
	}
	fmt.Println()
	if *flagVerbose {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), rd.Info())
	}
	return nil
}

// readFull fills buf, reporting io.EOF for a short read at the end: a partial
// window is not a window.
func readFull(rd *sampleio.Reader, buf []float32) error {
	total := 0
	for total < len(buf) {
		n, err := rd.ReadSamples(buf[total:])
		total += n
		if err != nil {
			return err
		}
		if n == 0 {
			return io.EOF
		}
	}
	return nil
}
