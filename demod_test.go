package dsp

import (
	"math"
	"math/rand"
	"testing"
)

var demodBenchSamples []complex64

func init() {
	r := rand.New(rand.NewSource(0))
	demodBenchSamples = make([]complex64, benchSize)
	for i := range benchSize {
		demodBenchSamples[i] = complex(r.Float32(), r.Float32())
	}
}

func TestFMDemodulation(t *testing.T) {
	filter := &FMDemod[complex64, float32]{}
	src := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}
	dst := make([]float32, len(src))
	filter.pre = 0.0
	fmDemodulateAsm(filter, dst, src)
	expected := make([]float32, len(src))
	filter.pre = 0.0
	fmDemodulate(filter, expected, src)
	if len(dst) != len(expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
	}
	for i := range dst {
		if dst[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
		}
	}
}

func BenchmarkFastPolarDiscriminator(b *testing.B) {
	x := complex(float32(1), float32(2))
	y := complex(float32(-3), float32(9))
	for range b.N {
		_ = FastPolarDiscriminator(x, y)
	}
}

func BenchmarkFMDemodulation(b *testing.B) {
	filter := &FMDemod[complex64, float32]{}
	dst := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		fmDemodulateAsm(filter, dst, demodBenchSamples)
	}
}

func BenchmarkFMDemodulation_Go(b *testing.B) {
	filter := &FMDemod[complex64, float32]{}
	dst := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		fmDemodulate(filter, dst, demodBenchSamples)
	}
}

// TestFMDemodulateShortOutput checks that a short output truncates rather than
// overruns. Demodulate used to write len(src) samples whatever the output
// length was.
func TestFMDemodulateShortOutput(t *testing.T) {
	src := []complex64{complex(0, 2), complex(1, 2), complex(-3, 7), complex(4, -9), complex(1, 1)}
	for n := range len(src) + 2 {
		// One extra element to catch a write past the end.
		dst := make([]float32, n+1)
		fmDemodulateAsm(&FMDemod[complex64, float32]{}, dst[:n], src)
		if dst[n] != 0 {
			t.Fatalf("n=%d: wrote past the end: %v", n, dst)
		}

		ref := make([]float32, n)
		fmDemodulate(&FMDemod[complex64, float32]{}, ref, src)
		for i := range ref {
			if dst[i] != ref[i] {
				t.Fatalf("n=%d [%d]: asm %v, go %v", n, i, dst[i], ref[i])
			}
		}
	}
}

// TestFMDemodReset checks that a reset demodulator starts a stream the
// same way a fresh one does, which is the only way to reuse one.
func TestFMDemodReset(t *testing.T) {
	src := []complex64{complex(0, 2), complex(1, 2), complex(-3, 7), complex(4, -9)}
	f := NewFMDemod[complex64, float32]()
	first := make([]float32, len(src))
	f.Demodulate(first, src)
	f.Reset()
	again := make([]float32, len(src))
	f.Demodulate(again, src)
	for i := range first {
		if again[i] != first[i] {
			t.Fatalf("sample %d after Reset: %v, want %v", i, again[i], first[i])
		}
	}
}

// TestFMDemodWidths checks the paths that do not run the assembly: complex128
// is the exact discriminator, and a mismatched output width is the input
// width's answer converted.
func TestFMDemodWidths(t *testing.T) {
	src128 := make([]complex128, 300)
	src64 := make([]complex64, len(src128))
	for i := range src128 {
		s, c := math.Sincos(0.3*float64(i) + 0.002*float64(i*i))
		src128[i] = complex(c, s)
		src64[i] = complex64(src128[i])
	}

	got := make([]float64, len(src128))
	NewFMDemod[complex128, float64]().Demodulate(got, src128)
	var pre complex128
	for i, v := range src128 {
		if want := PolarDiscriminator(v, pre); got[i] != want {
			t.Fatalf("complex128 [%d] = %v, want %v", i, got[i], want)
		}
		pre = v
	}

	single := make([]float32, len(src64))
	NewFMDemod[complex64, float32]().Demodulate(single, src64)
	widened := make([]float64, len(src64))
	NewFMDemod[complex64, float64]().Demodulate(widened, src64)
	for i := range single {
		if widened[i] != float64(single[i]) {
			t.Fatalf("complex64 to float64 [%d] = %v, want %v", i, widened[i], single[i])
		}
	}
}
