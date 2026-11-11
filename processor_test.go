package dsp

import (
	"slices"
	"strconv"
	"testing"
)

// inPlace processes x in place and returns the prefix holding the output.
func inPlace[T Sample](p Processor[T, T], x []T) []T {
	return x[:p.Process(x, x)]
}

// TestProcessorContract runs every block processor through the rules Processor
// documents: Process writes exactly OutputLen(len(src)) samples and nothing
// past them, panics on a shorter dst, gives the same stream whatever the block
// lengths are, works in place wherever the output is no longer than the input,
// and starts over after Reset.
func TestProcessorContract(t *testing.T) {
	ramp := rampAndTone(67)
	real32 := make([]float32, len(ramp))
	cplx64 := make([]complex64, len(ramp))
	cplx128 := make([]complex128, len(ramp))
	for i, v := range ramp {
		w := ramp[(i*7+3)%len(ramp)]
		real32[i] = float32(v)
		cplx64[i] = complex(float32(v), float32(w))
		cplx128[i] = complex(v, w)
	}
	taps := []float64{0.1, -0.2, 0.4, 0.3, 0.1}

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"IIRFilter/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewIIRFilter([]float64{0.2, 0.3, 0.2}, []float64{1, -0.4, 0.1}))
			}, ramp, -9999)
		}},
		{"IIRFilter/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewRealCoefIIRFilter[complex64]([]float32{0.2, 0.3, 0.2}, []float32{1, -0.4, 0.1}))
			}, cplx64, -9999)
		}},
		{"BiQuadFilter/float32", func(t *testing.T) {
			checkProcessor(t, func() Processor[float32, float32] {
				return must(NewLowPassBiQuad[float32](48000, 4000, 0.7071))
			}, real32, -9999)
		}},
		{"DCFilter/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewDCFilter[float64](0.95))
			}, ramp, -9999)
		}},
		{"FIRFilter/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewFIRFilter(taps))
			}, ramp, -9999)
		}},
		{"FIRFilter/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewRealCoefFIRFilter[complex128](taps))
			}, cplx128, -9999)
		}},
		{"FIRDecimator/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewFIRDecimator(taps, 3))
			}, ramp, -9999)
		}},
		{"FIRInterpolator/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewFIRInterpolator(taps, 3))
			}, ramp, -9999)
		}},
		{"BoxcarDecimator/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewBoxcarDecimator[complex64](4))
			}, cplx64, -9999)
		}},
		{"BoxcarDecimator/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewBoxcarDecimator[float64](3))
			}, ramp, -9999)
		}},
		{"RationalBoxcarDecimator/float32", func(t *testing.T) {
			checkProcessor(t, func() Processor[float32, float32] {
				return must(NewRationalBoxcarDecimator[float32](48, 11))
			}, real32, -9999)
		}},
		{"RationalBoxcarDecimator/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewRationalBoxcarDecimator[complex128](7, 3))
			}, cplx128, -9999)
		}},
		{"FMDemod/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, float32] {
				return NewFMDemod[complex64, float32]()
			}, cplx64, -9999)
		}},
		{"FMDemod/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, float64] {
				return NewFMDemod[complex128, float64]()
			}, cplx128, -9999)
		}},
		{"Rotator90/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return NewRotator90[complex64]()
			}, cplx64, -9999)
		}},
		{"Rotator90/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return NewRotator90[complex128]()
			}, cplx128, -9999)
		}},
		{"SDFT/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewSDFT[complex128](3, 16, HannFreqCoeff()))
			}, cplx128, -9999)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

// checkProcessor applies the contract to the processors build returns. guard
// is a value no output takes, used to catch a write past the end of dst.
func checkProcessor[In, Out Sample](t *testing.T, build func() Processor[In, Out], src []In, guard Out) {
	t.Helper()
	p := build()
	need := p.OutputLen(len(src))
	want := make([]Out, need+4)
	for i := range want {
		want[i] = guard
	}
	if n := p.Process(want[:need], src); n != need {
		t.Fatalf("Process wrote %d, OutputLen said %d", n, need)
	}
	for i, v := range want[need:] {
		if v != guard {
			t.Fatalf("wrote past OutputLen at %d", need+i)
		}
	}
	want = want[:need]

	for _, block := range []int{1, 2, 3, 7, 16} {
		t.Run("block"+strconv.Itoa(block), func(t *testing.T) {
			p := build()
			var got []Out
			for i := 0; i < len(src); i += block {
				in := src[i:min(i+block, len(src))]
				out := make([]Out, p.OutputLen(len(in)))
				if n := p.Process(out, in); n != len(out) {
					t.Fatalf("at %d: wrote %d, OutputLen said %d", i, n, len(out))
				}
				got = append(got, out...)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %v\nwant %v", got, want)
			}
		})
	}

	t.Run("reset", func(t *testing.T) {
		p := build()
		p.Process(make([]Out, p.OutputLen(5)), src[:5])
		p.Reset()
		got := make([]Out, p.OutputLen(len(src)))
		p.Process(got, src)
		if !slices.Equal(got, want) {
			t.Fatalf("got %v\nwant %v", got, want)
		}
	})

	if need > 0 {
		t.Run("short", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("dst one short of OutputLen: no panic")
				}
			}()
			build().Process(make([]Out, need-1), src)
		})
	}

	if buf, ok := any(slices.Clone(src)).([]Out); ok && need <= len(src) {
		t.Run("inplace", func(t *testing.T) {
			p := build()
			if got := buf[:p.Process(buf, any(buf).([]In))]; !slices.Equal(got, want) {
				t.Fatalf("got %v\nwant %v", got, want)
			}
		})
	}
}
