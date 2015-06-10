package dsp

import (
	"math/rand"
	"testing"
)

var demodBenchSamples []complex64

func init() {
	rand.Seed(0)
	demodBenchSamples = make([]complex64, benchSize)
	for i := 0; i < benchSize; i++ {
		demodBenchSamples[i] = complex(rand.Float32(), rand.Float32())
	}
}

func TestFMDemodulation(t *testing.T) {
	filter := &FMDemodFilter{}
	input := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}
	output := make([]float32, len(input))
	filter.pre = 0.0
	if n := fmDemodulateAsm(filter, input, output); n != len(input) {
		t.Fatalf("Expected n %d instead of %d", len(input), n)
	}
	expected := make([]float32, len(input))
	filter.pre = 0.0
	if n := fmDemodulate(filter, input, expected); n != len(input) {
		t.Fatalf("Expected n %d instead of %d", len(input), n)
	}
	if len(output) != len(expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
	}
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}
}

// func TestPolarDiscriminator32(t *testing.T) {
// 	for i := 0; i < 1000; i++ {
// 		x := complex(rand.Float32()-0.5, rand.Float32()-0.5)
// 		y := complex(rand.Float32()-0.5, rand.Float32()-0.5)
// 		expected := polarDiscriminator32(x, y)
// 		output := PolarDiscriminator32(x, y)
// 		if expected != output {
// 			t.Fatalf("Output differs: %f != %f", output, expected)
// 		}
// 	}
// }

func BenchmarkPolarDiscriminator32(b *testing.B) {
	x := complex(float32(1), float32(2))
	y := complex(float32(-3), float32(9))
	for i := 0; i < b.N; i++ {
		_ = PolarDiscriminator32(x, y)
	}
}

func BenchmarkFMDemodulation(b *testing.B) {
	filter := &FMDemodFilter{}
	output := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fmDemodulateAsm(filter, demodBenchSamples, output)
	}
}

func BenchmarkFMDemodulation_Go(b *testing.B) {
	filter := &FMDemodFilter{}
	output := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fmDemodulate(filter, demodBenchSamples, output)
	}
}
