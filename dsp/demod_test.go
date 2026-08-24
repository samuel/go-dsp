package dsp

import (
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
	filter := &FMDemod{}
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
	filter := &FMDemod{}
	dst := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		fmDemodulateAsm(filter, dst, demodBenchSamples)
	}
}

func BenchmarkFMDemodulation_Go(b *testing.B) {
	filter := &FMDemod{}
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
		fmDemodulateAsm(&FMDemod{}, dst[:n], src)
		if dst[n] != 0 {
			t.Fatalf("n=%d: wrote past the end: %v", n, dst)
		}

		ref := make([]float32, n)
		fmDemodulate(&FMDemod{}, ref, src)
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
	f := NewFMDemod()
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
