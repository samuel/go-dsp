package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"

	"github.com/samuel/go-dsp/dsp"
	"github.com/samuel/go-dsp/dsp/ax25"
	"github.com/samuel/go-dsp/sampleio"
)

var (
	flagVerbose    = flag.Bool("v", false, "Verbose output")
	flagSampleRate = flag.Float64("rate", 0, "Sample rate override; required for a headerless file")
	flagFormat     = flag.String("format", "i16le", "Sample format of a headerless file")
	flagBaud       = flag.Int("baud", 1200, "Baud rate of the AFSK signal")
	flagMark       = flag.Int("mark", 1200, "Mark tone frequency in Hz")
	flagSpace      = flag.Int("space", 2200, "Space tone frequency in Hz")
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
	format, err := sampleio.ParseFormat(*flagFormat)
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
	goer, err := dsp.NewGoertzel[float32]([]float64{float64(*flagMark), float64(*flagSpace)}, sampleRate, blockSize)
	if err != nil {
		return err
	}

	threshold := 50.0

	samples := make([]float32, blockSize)
	buf := make([]float32, window)

	currentTime := float64(0.0)
	bitClock := 1.0 / float64(baud)
	windowTime := float64(window) / sampleRate
	timeDelta := 0.0
	prevBit := 0
	transition := false

	ax := ax25.NewDecoder()
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
			b := byte(1)
			if transition {
				b = 0
				currentTime += timeDelta
				timeDelta = 0.0
			}
			frame := ax.Feed(b)
			if frame != nil {
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
			transition = false
		}
	}
	return nil
}

// readFull fills buf, reporting io.EOF for a short read at the end: a partial
// window is not a window.
func readFull(rd *sampleio.Reader, buf []float32) error {
	total := 0
	for total < len(buf) {
		n, err := rd.ReadFloat32(buf[total:])
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
