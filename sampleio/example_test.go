package sampleio_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"

	"samuelks.com/dsp/sampleio"
)

// An rtl-sdr capture is headerless interleaved unsigned bytes, I then Q, so
// it needs its format and rate given. "cu8" names the format and sets IQ. The
// samples come back scaled so the most negative code is exactly -1.
func ExampleNewReader() {
	format, iq, err := sampleio.ParseFormatSpec("cu8")
	if err != nil {
		log.Fatal(err)
	}
	capture := []byte{255, 128, 128, 0, 0, 64}
	rd, err := sampleio.NewReader(bytes.NewReader(capture), sampleio.Options{
		Format: format,
		IQ:     iq,
		Rate:   2_400_000,
	})
	if err != nil {
		log.Fatal(err)
	}

	samples := make([]complex64, 16)
	n, err := rd.ReadSamples(samples)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(n, samples[:n])
	// Output:
	// 3 [(0.9921875+0i) (0-1i) (-1-0.5i)]
}

// Open reads a WAV file's rate and encoding from its header. Read in blocks
// until io.EOF, which comes on the call after the last samples.
func ExampleOpen() {
	rd, err := sampleio.Open("capture.wav", sampleio.Options{})
	if err != nil {
		log.Fatal(err)
	}
	info := rd.Info()

	block := make([]float64, 4096)
	var frames int
	for {
		var n int
		n, err = rd.ReadSamples(block)
		if err != nil {
			break
		}
		frames += n // process block[:n] here
	}
	if closeErr := rd.Close(); closeErr != nil {
		log.Fatal(closeErr)
	}
	if !errors.Is(err, io.EOF) {
		log.Fatal(err)
	}
	fmt.Printf("%d frames, %.2f s at %g Hz\n", frames, float64(frames)/info.Rate, info.Rate)
}
