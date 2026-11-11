package dsp_test

import (
	"fmt"
	"log"
	"math"

	"samuelks.com/dsp"
)

// gainDB returns the steady-state gain of a filter for a tone, from the peak of
// the second half of its output.
func gainDB(filter func(dst, src []float32) int, rate, freq float64) float64 {
	in := make([]float32, int(rate)/10)
	for i := range in {
		in[i] = float32(math.Sin(2 * math.Pi * freq * float64(i) / rate))
	}
	out := make([]float32, len(in))
	filter(out, in)
	var peak float64
	for _, v := range out[len(out)/2:] {
		peak = max(peak, math.Abs(float64(v)))
	}
	// Adding zero turns a rounded -0 into +0, so a unity gain prints as 0.0.
	return math.Round(200*math.Log10(peak))/10 + 0
}

// A 1 kHz Butterworth low-pass at 48 kHz passes a 100 Hz tone, is 3 dB down at
// its cutoff, and is more than 40 dB down a decade above it, as a
// second-order filter should be.
func ExampleNewBiquadLowPass() {
	const rate = 48000
	for _, freq := range []float64{100, 1000, 10000} {
		lp, err := dsp.NewBiquadLowPass[float32](rate, 1000, math.Sqrt2/2)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%.0f Hz: %.1f dB\n", freq, gainDB(lp.Process, rate, freq))
	}
	// Output:
	// 100 Hz: 0.0 dB
	// 1000 Hz: -3.0 dB
	// 10000 Hz: -42.8 dB
}

// FM demodulation turns each sample's phase step into an output, so a carrier
// offset by a constant frequency demodulates to a constant: 2*pi times the
// offset in cycles per sample.
//
// FMDemod[complex64, float32] is built on FastAtan2, whose
// error of up to about 0.01 radians is invisible under audio but not in a
// printed number; complex128 uses the exact discriminator.
func ExampleFMDemod() {
	const rate, offset = 240000.0, 12000.0
	iq := make([]complex128, 64)
	for i := range iq {
		s, c := math.Sincos(2 * math.Pi * offset * float64(i) / rate)
		iq[i] = complex(c, s)
	}
	fmd := dsp.NewFMDemod[complex128, float64]()
	out := make([]float64, len(iq))
	fmd.Process(out, iq)

	// The first output compares against the silence before the stream.
	fmt.Printf("%.4f rad/sample = %.0f Hz\n", out[10], out[10]*rate/(2*math.Pi))
	// Output:
	// 0.3142 rad/sample = 12000 Hz
}

// Doubling the sample rate: the low-pass cuts at the old Nyquist, a quarter of
// the new rate, and is scaled to a DC gain of 2 to make up for the zeros the
// interpolator stuffs between samples. Once the filter fills, a DC input comes
// out at its own level.
func ExampleFIRInterpolator() {
	taps, err := dsp.LowPassFIR[float32](31, 0.25, dsp.KaiserBeta(60))
	if err != nil {
		log.Fatal(err)
	}
	if err := dsp.NormalizeTaps(taps, 2); err != nil {
		log.Fatal(err)
	}
	it, err := dsp.NewFIRInterpolator(taps, 2)
	if err != nil {
		log.Fatal(err)
	}

	src := []float32{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	dst := make([]float32, it.OutputLen(len(src)))
	n := it.Process(dst, src)
	fmt.Printf("%d inputs, %d outputs, last four %.3f\n", len(src), n, dst[n-4:n])
	// Output:
	// 20 inputs, 40 outputs, last four [1.000 1.000 1.000 1.000]
}

// SincInterpolator.At reads a signal between its samples. Here a slow sine is
// read halfway between samples 20 and 21, and lands within the passband
// ripple of 16 windowed taps of the true value. The same table reads a complex
// signal, the matching complex exponential, part by part.
func ExampleSincInterpolator_At() {
	const freq = 0.03 // cycles per sample
	src := make([]float64, 64)
	iq := make([]complex64, 64)
	for i := range src {
		s, c := math.Sincos(2 * math.Pi * freq * float64(i))
		src[i] = s
		iq[i] = complex(float32(c), float32(s))
	}
	s, err := dsp.NewSincInterpolator[float64](16, 256, 0.45, 6)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("At(20.5) = %.4f, sine = %.4f\n", s.At(src, 20.5), math.Sin(2*math.Pi*freq*20.5))
	fmt.Printf("complex At(20.5) = %.4f\n", s.At(iq, 20.5))
	// Output:
	// At(20.5) = -0.6611, sine = -0.6613
	// complex At(20.5) = (-0.7498-0.6611i)
}
