package dsp

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestPLLLocksToACarrier(t *testing.T) {
	const f = 0.01
	p, err := NewPLL[complex128](0.002, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	// An AM signal: a carrier at f with a slow tone on its amplitude, at a phase offset.
	src := make([]complex128, 20000)
	for i := range src {
		amp := 1 + 0.5*math.Cos(2*math.Pi*0.001*float64(i))
		src[i] = cmplx.Rect(amp, 2*math.Pi*f*float64(i)+1)
	}
	dst := make([]complex128, len(src))
	p.Process(dst, src)
	if got := p.Frequency(); math.Abs(got-f) > 1e-6 {
		t.Errorf("frequency = %v, want %v", got, f)
	}
	// Locked, the derotated signal is real: the carrier plus the modulation.
	for i := len(dst) - 1000; i < len(dst); i++ {
		want := 1 + 0.5*math.Cos(2*math.Pi*0.001*float64(i))
		if math.Abs(real(dst[i])-want) > 1e-3 || math.Abs(imag(dst[i])) > 1e-3 {
			t.Fatalf("dst[%d] = %v, want %v", i, dst[i], want)
		}
	}

	// A carrier outside the range pins the frequency at its edge.
	p, err = NewPLL[complex128](0.002, 0.005)
	if err != nil {
		t.Fatal(err)
	}
	p.Process(dst, src)
	if got := p.Frequency(); math.Abs(got-0.005) > 1e-9 {
		t.Errorf("frequency with a limited range = %v, want 0.005", got)
	}
	for _, bad := range [][2]float64{{0, 0.1}, {0.5, 0.1}, {0.01, -1}, {0.01, 0.6}} {
		if _, err := NewPLL[complex64](bad[0], bad[1]); err == nil {
			t.Errorf("NewPLL%v accepted", bad)
		}
	}
}

// toneRMS returns the RMS of the second half of x.
func toneRMS(x []float64) float64 {
	tail := x[len(x)/2:]
	var p float64
	for _, v := range tail {
		p += v * v
	}
	return math.Sqrt(p / float64(len(tail)))
}

func TestSidebandDemod(t *testing.T) {
	const n = 8000
	tone := func(f float64) []complex128 {
		s := make([]complex128, n)
		for i := range s {
			s[i] = cmplx.Rect(1, 2*math.Pi*f*float64(i))
		}
		return s
	}
	demod := func(low, high, shift float64, src []complex128) []float64 {
		d, err := NewSidebandDemod[complex128, float64](low, high, 0.01, shift)
		if err != nil {
			t.Fatal(err)
		}
		dst := make([]float64, len(src))
		d.Process(dst, src)
		return dst
	}
	// Upper sideband passes a tone above the carrier as a real cosine and rejects its image.
	usb := demod(0.01, 0.1, 0, tone(0.05))
	if r := toneRMS(usb); math.Abs(r-math.Sqrt2/2) > 0.01 {
		t.Errorf("USB tone RMS = %v, want %v", r, math.Sqrt2/2)
	}
	for i := n - 100; i < n; i++ {
		if want := math.Cos(2 * math.Pi * 0.05 * float64(i-demodDelay(0.01))); math.Abs(usb[i]-want) > 0.01 {
			t.Fatalf("usb[%d] = %v, want %v", i, usb[i], want)
		}
	}
	if r := toneRMS(demod(0.01, 0.1, 0, tone(-0.05))); r > 1e-3 {
		t.Errorf("USB passed the lower sideband at RMS %v", r)
	}
	// Lower sideband is the mirror.
	if r := toneRMS(demod(-0.1, -0.01, 0, tone(-0.05))); math.Abs(r-math.Sqrt2/2) > 0.01 {
		t.Errorf("LSB tone RMS = %v", r)
	}
	// CW: a carrier at 0 Hz becomes a tone at the shift.
	cw := demod(-0.005, 0.005, 0.03, tone(0))
	if r := toneRMS(cw); math.Abs(r-math.Sqrt2/2) > 0.01 {
		t.Errorf("CW tone RMS = %v", r)
	}
	for _, bad := range [][3]float64{{0.1, 0.05, 0.01}, {-0.6, 0.1, 0.01}, {0, 0.1, 0}} {
		if _, err := NewSidebandDemod[complex64, float32](bad[0], bad[1], bad[2], 0); err == nil {
			t.Errorf("NewSidebandDemod%v accepted", bad)
		}
	}
}

// demodDelay returns the filter delay of a SidebandDemod with the given transition.
func demodDelay(transition float64) int {
	n := int(math.Ceil((sidebandAttenuation-7.95)/(14.36*transition))) + 1 | 1
	return n / 2
}
