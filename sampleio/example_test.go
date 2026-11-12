package sampleio_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math"

	"samuelks.com/dsp/pcm"
	"samuelks.com/dsp/sampleio"
)

// An rtl-sdr capture has no header: it is interleaved unsigned bytes, I then
// Q. The Layout gives its format and rate, and "cu8" parses to the format with
// IQ set. Samples are scaled so the most negative code is exactly -1.
func ExampleNewReader() {
	format, iq, err := sampleio.ParseFormatSpec("cu8")
	if err != nil {
		log.Fatal(err)
	}
	capture := []byte{255, 128, 128, 0, 0, 64}
	rd, err := sampleio.NewReader(bytes.NewReader(capture), sampleio.Layout{
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

// Open reads a WAV file's rate and encoding from its header, so the zero
// Layout is enough. Read in blocks until io.EOF, which is returned by the call
// after the one that read the last samples.
func ExampleOpen() {
	rd, err := sampleio.Open("capture.wav", sampleio.Layout{})
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

// A stereo file is read one slice per channel. A nil slice skips its channel,
// so this reads only the right one, starting 2 frames in.
func ExampleReader_ReadPlanar() {
	// Two channels of 8-bit samples, left then right.
	stereo := []byte{128, 0, 128, 64, 128, 192, 128, 255}
	rd, err := sampleio.NewReader(bytes.NewReader(stereo), sampleio.Layout{
		Format:   pcm.U8,
		Rate:     8000,
		Channels: 2,
	})
	if err != nil {
		log.Fatal(err)
	}
	if _, err := rd.Skip(2); err != nil {
		log.Fatal(err)
	}

	right := make([]float32, 16)
	n, err := rd.ReadPlanar([][]float32{nil, right})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(n, right[:n])
	// Output:
	// 2 [0.5 0.9921875]
}

// A Writer encodes full-scale samples and writes the container. A stereo pair
// is written as one slice per channel. Close fills in the sizes in the WAV
// header, since a file can seek.
func ExampleWriter() {
	w, err := sampleio.Create("tone.wav", sampleio.Layout{
		Format:   pcm.I16LE,
		Rate:     48000,
		Channels: 2,
	})
	if err != nil {
		log.Fatal(err)
	}
	left := make([]float32, 48000)
	right := make([]float32, 48000)
	for i := range left {
		left[i] = 0.5 * float32(math.Sin(2*math.Pi*440*float64(i)/48000))
		right[i] = -left[i]
	}
	if _, err := w.WritePlanar([][]float32{left, right}); err != nil {
		log.Fatal(err)
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Println(w.Info().Frames, "frames")
}
