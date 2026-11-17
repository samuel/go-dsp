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
// documents: Process writes exactly OutputLen(len(src)) samples, or at most
// that many for a processor whose output follows its input, and nothing past
// them, panics on a shorter dst, gives the same stream whatever the block
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
		{"Biquad/float32", func(t *testing.T) {
			checkProcessor(t, func() Processor[float32, float32] {
				return must(NewBiquadLowPass[float32](48000, 4000, 0.7071))
			}, real32, -9999)
		}},
		{"DCFilter/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewDCFilter[float64](0.008))
			}, ramp, -9999)
		}},
		{"DCFilter/float32", func(t *testing.T) {
			checkProcessor(t, func() Processor[float32, float32] {
				return must(NewDCFilter[float32](0.008))
			}, real32, -9999)
		}},
		{"DCFilter/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewDCFilter[complex64](0.008))
			}, cplx64, -9999)
		}},
		{"DCFilter/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewDCFilter[complex128](0.008))
			}, cplx128, -9999)
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
		{"Mixer/float32/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float32, complex64] {
				return must(NewMixer[float32, complex64](0.11))
			}, real32, -9999)
		}},
		{"Mixer/complex64/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewMixer[complex64, complex64](-0.3))
			}, cplx64, -9999)
		}},
		{"Mixer/complex128/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewMixer[complex128, complex128](0.25))
			}, cplx128, -9999)
		}},
		{"SDFT/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewSDFT[complex128](3, 16, HannFreqCoeff()))
			}, cplx128, -9999)
		}},
		{"Resampler/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewResampler[float64](0.73))
			}, ramp, -9999)
		}},
		{"Resampler/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewResampler[complex64](1.6))
			}, cplx64, -9999)
		}},
		{"AGC/float32", func(t *testing.T) {
			checkProcessor(t, func() Processor[float32, float32] {
				return must(NewAGC[float32](AGCConfig{Target: 0.5, Attack: 4, Decay: 40, MaxGain: 10}))
			}, real32, -9999)
		}},
		{"AGC/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewAGC[complex128](AGCConfig{Target: 0.5, Attack: 4, Decay: 40, MaxGain: 10}))
			}, cplx128, -9999)
		}},
		{"PowerSquelch/float64", func(t *testing.T) {
			checkProcessor(t, func() Processor[float64, float64] {
				return must(NewPowerSquelch[float64](PowerSquelchConfig{Threshold: -6, Hysteresis: 1, Average: 5}))
			}, ramp, -9999)
		}},
		{"SidebandDemod/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, float32] {
				return must(NewSidebandDemod[complex64, float32](0.01, 0.1, 0.1, 0.05))
			}, cplx64, -9999)
		}},
		{"PLL/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewPLL[complex64](0.01, 0.1))
			}, cplx64, -9999)
		}},
		{"Costas/complex64", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex64, complex64] {
				return must(NewCostas[complex64](2, 0.01, 0.1))
			}, cplx64, -9999)
		}},
		{"Costas/complex128", func(t *testing.T) {
			checkProcessor(t, func() Processor[complex128, complex128] {
				return must(NewCostas[complex128](4, 0.01, 0.1))
			}, cplx128, -9999)
		}},
		{"SymbolSync/float32", func(t *testing.T) {
			checkVariableProcessor(t, func() Processor[float32, float32] {
				return must(NewSymbolSync[float32](SymbolSyncConfig{SamplesPerSymbol: 3.3, LoopBandwidth: 0.05, MaxDeviation: 0.1}))
			}, real32, -9999)
		}},
		{"SymbolSync/complex128", func(t *testing.T) {
			checkVariableProcessor(t, func() Processor[complex128, complex128] {
				return must(NewSymbolSync[complex128](SymbolSyncConfig{
					SamplesPerSymbol: 2.5, LoopBandwidth: 0.05, MaxDeviation: 0.1, Detector: MuellerMuller,
				}))
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
	checkProcessorCount(t, build, src, guard, true)
}

// checkVariableProcessor is checkProcessor for a processor whose OutputLen is
// an upper bound.
func checkVariableProcessor[In, Out Sample](t *testing.T, build func() Processor[In, Out], src []In, guard Out) {
	t.Helper()
	checkProcessorCount(t, build, src, guard, false)
}

func checkProcessorCount[In, Out Sample](t *testing.T, build func() Processor[In, Out], src []In, guard Out, exact bool) {
	t.Helper()
	// wrote checks a count Process returned against the OutputLen it was given.
	wrote := func(t *testing.T, n, bound int) {
		t.Helper()
		if n > bound || exact && n != bound {
			t.Fatalf("Process wrote %d, OutputLen said %d", n, bound)
		}
	}
	p := build()
	need := p.OutputLen(len(src))
	want := make([]Out, need+4)
	for i := range want {
		want[i] = guard
	}
	n := p.Process(want[:need], src)
	wrote(t, n, need)
	for i, v := range want[n:] {
		if v != guard {
			t.Fatalf("wrote past the count it returned at %d", n+i)
		}
	}
	want = want[:n]

	for _, block := range []int{1, 2, 3, 7, 16} {
		t.Run("block"+strconv.Itoa(block), func(t *testing.T) {
			p := build()
			var got []Out
			for i := 0; i < len(src); i += block {
				in := src[i:min(i+block, len(src))]
				out := make([]Out, p.OutputLen(len(in)))
				n := p.Process(out, in)
				wrote(t, n, len(out))
				got = append(got, out[:n]...)
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
		got = got[:p.Process(got, src)]
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
