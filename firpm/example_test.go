package firpm_test

import (
	"fmt"
	"log"
	"math"

	"github.com/samuel/go-dsp/firpm"
)

// Design a length 32 bandpass filter with stopbands 0 to 0.1 and 0.425 to 0.5,
// a passband from 0.2 to 0.35, and ten times the weight on the stopbands. This
// is the filter in figure 10 of the original Parks-McClellan paper.
func ExampleDesign() {
	h, deviation, err := firpm.Design(firpm.Spec{
		NumTaps: 32,
		Type:    firpm.BandPass,
		Bands: []firpm.Band{
			{Lower: 0.0, Upper: 0.1, Response: 0.0, Weight: 10.0},
			{Lower: 0.2, Upper: 0.35, Response: 1.0, Weight: 1.0},
			{Lower: 0.425, Upper: 0.5, Response: 0.0, Weight: 10.0},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("passband ripple  %.6f\n", deviation)
	fmt.Printf("stopband ripple  %.6f\n", deviation/10)
	fmt.Printf("h[0] %.6f  h[15] %.6f\n", h[0], h[15])
	// Output:
	// passband ripple  0.015131
	// stopband ripple  0.001513
	// h[0] -0.005753  h[15] 0.304109
}

// Band edges can be given in Hz instead, against a sample rate.
func ExampleDesign_sampleRate() {
	h, deviation, err := firpm.Design(firpm.Spec{
		NumTaps:    21,
		SampleRate: 48000,
		Bands: []firpm.Band{
			{Lower: 0, Upper: 4000, Response: 1.0, Weight: 1.0},
			{Lower: 8000, Upper: 24000, Response: 0.0, Weight: 1.0},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d taps, stopband attenuation %.1f dB\n", len(h), -20*math.Log10(deviation))
	// Output:
	// 21 taps, stopband attenuation 35.7 dB
}
