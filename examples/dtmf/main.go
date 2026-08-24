package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/samuel/go-dsp/dsp/dtmf"
	"github.com/samuel/go-dsp/sampleio"
)

var (
	flagSampleRate = flag.Float64("rate", 0, "Sample rate override; required for a headerless file")
	flagFormat     = flag.String("format", "i16le", "Sample format of a headerless file")
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
	// A WAV carries its own rate and encoding, so these are overrides for it and
	// requirements for a headerless stream.
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
	// 205 samples is about 25ms at 8kHz, long enough to resolve the two tone
	// groups and short enough for the shortest digit anyone sends.
	blockSize := int(205 * sampleRate / 8000)
	window := blockSize / 4
	dt, err := dtmf.NewStandard(sampleRate, blockSize)
	if err != nil {
		return err
	}
	lastKey := dtmf.NoKey
	keyCount := 0
	samples := make([]float32, blockSize)
	buf := make([]float32, window)

	for {
		if _, err := readFull(rd, buf); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}

		copy(samples, samples[window:])
		copy(samples[len(samples)-window:], buf)

		// The same key on three consecutive blocks, which at a quarter-block
		// hop is one block's worth of agreement, before it counts as a digit.
		if k, _ := dt.Feed(samples); k != dtmf.NoKey && k == lastKey {
			keyCount++
			if keyCount == 3 {
				fmt.Printf("%c", dtmf.Key(k))
			}
		} else {
			lastKey = k
			keyCount = 0
		}
	}
	fmt.Println()
	return nil
}

// readFull fills buf, reporting io.EOF for a short read at the end: a partial
// block is not a block.
func readFull(rd *sampleio.Reader, buf []float32) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := rd.ReadFloat32(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.EOF
		}
	}
	return total, nil
}
